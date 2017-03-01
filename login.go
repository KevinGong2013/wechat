package wechat

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UUIDProcessor scan this uuid
type UUIDProcessor interface {
	ProcessUUID(uuid string) error
	UUIDDidConfirm(err error)
}

type initRequest struct {
	BaseRequest *BaseRequest
}

type initResp struct {
	Response
	User    Contact
	Skey    string
	SyncKey map[string]interface{}
}

func (wechat *WeChat) reLogin() error {

	client, err := newClient()
	if err != nil {
		return err
	}

	wechat.Client = client

	err = wechat.beginLoginFlow()
	if err != nil {
		return err
	}

	return nil
}

// run is used to login to wechat server. Need end user scan orcode.
func (wechat *WeChat) beginLoginFlow() error {

	logger.Info(`wait a moment, prepare login parameters ... ...`)

	cached, err := wechat.cachedInfo()

	if err == nil {

		logger.Info(`will attempt recoverer sessoin`)

		wechat.BaseURL = cached[`baseURL`].(string)
		wechat.BaseRequest = cached[`baseRequest`].(*BaseRequest)
		cookies := cached[`cookies`].([]*http.Cookie)
		u, ue := url.Parse(wechat.BaseURL)
		if ue != nil {
			return ue
		}
		wechat.Client.Jar.SetCookies(u, cookies)

		err = wechat.init()
		if err != nil {
			deleteFile(wechat.conf.cookieCachePath())
		}

		return err
	}
	logger.Error(err)

	// 1.
	uuid, e := wechat.fetchUUID()

	if e != nil {
		return e
	}

	// 2.
	err = wechat.conf.Processor.ProcessUUID(uuid)

	if err != nil {
		return err
	}

	// 3.
	redirectURL, code, tip := ``, ``, 1

	for code != httpOK {
		redirectURL, code, tip, err = wechat.waitConfirmUUID(uuid, tip)
		if err != nil {
			wechat.conf.Processor.UUIDDidConfirm(err)
			return err
		}
	}

	wechat.conf.Processor.UUIDDidConfirm(nil)

	req, _ := http.NewRequest(`GET`, redirectURL, nil)

	// 4.
	if err = wechat.login(req); err != nil {
		return err
	}

	return wechat.init()
}

func (wechat *WeChat) cachedInfo() (map[string]interface{}, error) {

	cachedInfo := make(map[string]interface{}, 3)

	baseInfo, err := wechat.cachedBaseInfo()
	if err != nil {
		return nil, err
	}

	bq := new(BaseRequest)
	bqInfo := baseInfo[`baseRequest`].(map[string]interface{})

	if did, ok := bqInfo[`DeviceID`].(string); ok {
		bq.DeviceID = did
	}
	if pst, ok := baseInfo[`passTicket`].(string); ok {
		bq.PassTicket = pst
	}
	if skey, ok := bqInfo[`Skey`].(string); ok {
		bq.Skey = skey
	}
	if sid, ok := bqInfo[`Sid`].(string); ok {
		bq.Wxsid = sid
	}
	if uf, ok := bqInfo[`Uin`].(float64); ok {
		bq.Wxuin = int64(uf)
	}

	baseInfo[`baseRequest`] = bq

	if err != nil || len(baseInfo) != 3 {
		return nil, errors.New(`cached baseInfo is invalidate`)
	}

	cookies, err := wechat.cachedCookies()
	if err != nil || len(cookies) == 0 {
		return nil, errors.New(`cached cookies is invalidate`)
	}

	for k, v := range baseInfo {
		cachedInfo[k] = v
	}
	cachedInfo[`cookies`] = cookies

	return cachedInfo, nil
}

func (wechat *WeChat) fetchUUID() (string, error) {

	jsloginURL := "https://login.weixin.qq.com/jslogin"

	params := url.Values{}
	params.Set("appid", "wx782c26e4c19acffb")
	params.Set("fun", "new")
	params.Set("lang", "zh_CN")
	params.Set("_", strconv.FormatInt(time.Now().Unix(), 10))

	resp, err := wechat.Client.PostForm(jsloginURL, params)
	if err != nil {
		return ``, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ``, err
	}
	ds := string(data)

	code, err := search(ds, `window.QRLogin.code = `, `;`)

	if err != nil {
		return ``, err
	}

	if code != httpOK {
		err = fmt.Errorf("error code is unexpect:[%s], api result:[%s]", code, ds)
		return ``, err
	}

	uuid, err := search(ds, `window.QRLogin.uuid = "`, `";`)
	if err != nil {
		return ``, err
	}

	return uuid, nil
}

func (wechat *WeChat) waitConfirmUUID(uuid string, tip int) (redirectURI, code string, rt int, err error) {

	loginURL, rt := fmt.Sprintf("https://login.weixin.qq.com/cgi-bin/mmwebwx-bin/login?tip=%d&uuid=%s&_=%s", tip, uuid, strconv.FormatInt(time.Now().Unix(), 10)), tip
	resp, err := wechat.Client.Get(loginURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	ds := string(data)

	code, err = search(ds, `window.code=`, `;`)
	if err != nil {
		return
	}

	rt = 0
	switch code {
	case "201":
		logger.Debug(`scan successed, waitting wechat app send confirm request.`)
	case httpOK:
		redirectURI, err = search(ds, `window.redirect_uri="`, `";`)
		if err != nil {
			return
		}
		redirectURI += "&fun=new"
	default:
		err = fmt.Errorf("time out, will retry %v", err)
	}
	return
}

func (wechat *WeChat) login(req *http.Request) error {

	resp, err := wechat.Client.Do(req)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := resp.Body.(io.Reader)

	// full fill base request
	if err = xml.NewDecoder(reader).Decode(wechat.BaseRequest); err != nil {
		logger.Debug(err)
		return err
	}

	if wechat.BaseRequest.Ret != 0 { // 0 is success
		err = fmt.Errorf("login failed message:[%s]", wechat.BaseRequest.Message)
		return err
	}

	//5.
	urlStr := req.URL.String()
	index := strings.LastIndex(urlStr, "/")
	if index == -1 {
		index = len(urlStr)
	}

	wechat.BaseURL = urlStr[:index]
	wechat.refreshBaseInfo()
	wechat.refreshCookieCache(resp.Cookies())

	return nil
}

func (wechat *WeChat) init() error {

	data, err := json.Marshal(initRequest{
		BaseRequest: wechat.BaseRequest,
	})
	if err != nil {
		return err
	}

	apiURI := fmt.Sprintf("%s/webwxinit?%s&%s&r=%s", wechat.BaseURL, wechat.PassTicketKV(), wechat.SkeyKV(), now())

	req, err := http.NewRequest(`POST`, apiURI, bytes.NewReader(data))
	if err != nil {
		return err
	}

	var resp initResp
	err = wechat.ExcuteRequest(req, &resp)
	if err != nil {
		return err
	}

	wechat.BaseRequest.Skey = resp.Skey

	wechat.MySelf = resp.User
	wechat.syncKey = resp.SyncKey

	return nil
}

func (wechat *WeChat) keepAlive() {
	go func() {

		err := wechat.reLogin()

		if err != nil {
			logger.Errorf(`login failed: %v`, err)
			retryTimes := wechat.retryTimes
			triggerAfter := time.After(time.Minute * retryTimes)
			logger.Warnf(`will retry login after %d minute(s)`, retryTimes)
			<-triggerAfter
			wechat.retryTimes++
			wechat.keepAlive()
			return
		}

		logger.Info(`CONGRATULATION login successed`)

		logger.Info(`begin sync contact`)
		err = wechat.SyncContact()
		if err != nil {
			logger.Errorf(`sync contact error: %v`, err)
		}
		logger.Info(`sync contact successfully`)

		wechat.loginState <- 1
		err = wechat.beginSync()
		wechat.loginState <- -1

		logger.Errorf(`sync error: %v`, err)

		wechat.keepAlive() // if listen occured error will excute this cmd.

		return
	}()
}

func (wechat *WeChat) cachedCookies() ([]*http.Cookie, error) {

	bs, err := ioutil.ReadFile(wechat.conf.cookieCachePath())
	if err != nil {
		return nil, err
	}

	var cookieInterfaces []interface{}
	err = json.Unmarshal(bs, &cookieInterfaces)

	if err != nil {
		return nil, err
	}

	var cookies []*http.Cookie

	for _, c := range cookieInterfaces {
		data, _ := json.Marshal(c)
		var cookie *http.Cookie
		json.Unmarshal(data, &cookie)
		cookies = append(cookies, cookie)
	}

	return cookies, nil
}

func (wechat *WeChat) refreshCookieCache(cookies []*http.Cookie) {
	if len(cookies) == 0 {
		return
	}
	b, err := json.Marshal(cookies)
	if err != nil {
		logger.Warnf(`refresh cookie error: %v`, err)
	} else {
		createFile(wechat.conf.cookieCachePath(), b, false)
		logger.Info(`did refresh cookie cache`)
	}
}

func (wechat *WeChat) cachedBaseInfo() (map[string]interface{}, error) {

	ubs, err := ioutil.ReadFile(wechat.conf.baseInfoCachePath())
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(ubs, &result)
	return result, err
}

func (wechat *WeChat) refreshBaseInfo() {
	info := make(map[string]interface{}, 3)

	info[`baseURL`] = wechat.BaseURL
	info[`passTicket`] = wechat.BaseRequest.PassTicket
	info[`baseRequest`] = wechat.BaseRequest

	data, _ := json.Marshal(info)
	createFile(wechat.conf.baseInfoCachePath(), data, false)
}
