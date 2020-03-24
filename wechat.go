package wechat

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

var logger = log.WithFields(log.Fields{
	"module": "wechat",
})

const httpOK = `200`

// BaseRequest is a base for all wx api request.
type BaseRequest struct {
	XMLName xml.Name `xml:"error" json:"-"`

	Ret        int    `xml:"ret" json:"-"`
	Message    string `xml:"message" json:"-"`
	Wxsid      string `xml:"wxsid" json:"Sid"`
	Skey       string `xml:"skey"`
	DeviceID   string `xml:"-"`
	Wxuin      int64  `xml:"wxuin" json:"Uin"`
	PassTicket string `xml:"pass_ticket" json:"-"`
}

// Caller is a interface, All response need implement this.
type Caller interface {
	IsSuccess() bool
	Error() error
}

// Response is a wrapper.
type Response struct {
	BaseResponse *BaseResponse
}

// IsSuccess flag this request is success or failed.
func (response *Response) IsSuccess() bool {
	return response.BaseResponse.Ret == 0
}

// response's error msg.
func (response *Response) Error() error {
	return fmt.Errorf("error message:[%s]", response.BaseResponse.ErrMsg)
}

// BaseResponse for all api resp.
type BaseResponse struct {
	Ret    int
	ErrMsg string
}

// Configure ...
type Configure struct {
	Processor         UUIDProcessor
	Debug             bool
	CachePath         string
	UniqueGroupMember bool
	version           string
}

// DefaultConfigure create default configuration
func DefaultConfigure() *Configure {
	return &Configure{
		Processor:         new(defaultUUIDProcessor),
		Debug:             true,
		UniqueGroupMember: true,
		CachePath:         `.wechat/debug`,
		version:           `1.0.1-rc1`,
	}
}

func (c *Configure) contactCachePath() string {
	return filepath.Join(c.CachePath, `contact-cache.json`)
}
func (c *Configure) baseInfoCachePath() string {
	return filepath.Join(c.CachePath, `basic-info-cache.json`)
}
func (c *Configure) cookieCachePath() string {
	return filepath.Join(c.CachePath, `cookie-cache.json`)
}

func (c *Configure) httpDebugPath(url *url.URL) string {
	ps := strings.Split(url.Path, `/`)
	lastP := strings.Split(ps[len(ps)-1], `?`)[0][5:]
	return c.CachePath + `/` + lastP
}

// WeChat container a default http client and base request.
type WeChat struct {
	Client      *http.Client
	BaseURL     string
	BaseRequest *BaseRequest
	MySelf      Contact
	IsLogin     bool

	conf       *Configure
	evtStream  *evtStream
	cache      *cache
	syncKey    map[string]interface{}
	syncHost   string
	retryTimes time.Duration
	loginState chan int // -1 登录失败 1登录成功
}

// NewWeChat is designed for Create a new Wechat instance.
func newWeChat(conf *Configure) (*WeChat, error) {

	if _, err := os.Stat(conf.CachePath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(conf.CachePath, os.ModePerm)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	client, err := newClient()
	if err != nil {
		return nil, err
	}

	baseReq := new(BaseRequest)
	baseReq.Ret = 1
	baseReq.DeviceID = `e999471493880231`

	wechat := &WeChat{
		Client:      client,
		BaseRequest: baseReq,
		evtStream:   newEvtStream(),
		IsLogin:     false,
		retryTimes:  time.Duration(0),
		loginState:  make(chan int),
		conf:        conf,
		cache:       newCache(),
	}

	return wechat, nil
}

func newClient() (*http.Client, error) {

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	transport := http.Transport{
		Dial: (&net.Dialer{
			Timeout: 1 * time.Minute,
		}).Dial,
		TLSHandshakeTimeout: 1 * time.Minute,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	client := &http.Client{
		Transport: &transport,
		Jar:       jar,
		Timeout:   1 * time.Minute,
	}

	return client, nil
}

// NewBot is start point for wx bot.
func NewBot(conf *Configure) (*WeChat, error) {

	if conf == nil {
		conf = DefaultConfigure()
	}

	wechat, err := newWeChat(conf)

	if err != nil {
		return nil, err
	}

	wechat.evtStream.init()
	go func() {
		for {
			ls := <-wechat.loginState
			event := Event{
				Path: `/login`,
				From: `Wechat`,
				To:   `End`,
				Data: ls,
				Time: time.Now().Unix(),
			}
			wechat.evtStream.serverEvt <- event
		}
	}()

	wechat.keepAlive()

	if conf.Debug {
		log.SetLevel(log.DebugLevel)
	}

	return wechat, nil
}

// ExecuteRequest is designed for perform http request
func (wechat *WeChat) ExecuteRequest(req *http.Request, call Caller) error {

	filename := wechat.conf.httpDebugPath(req.URL)

	if wechat.conf.Debug {
		reqData, _ := httputil.DumpRequestOut(req, false)
		createFile(filename+`_req.json`, reqData, false)
		c, _ := json.Marshal(wechat.Client.Jar.Cookies(req.URL))
		createFile(filename+`_req.json`, c, true)
	}

	resp, err := wechat.Client.Do(req)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := resp.Body.(io.Reader)

	if wechat.conf.Debug {

		data, e := ioutil.ReadAll(reader)
		if e != nil {
			return e
		}

		createFile(filename+`_resp.json`, data, true)
		reader = bytes.NewReader(data)
	}

	if err = json.NewDecoder(reader).Decode(call); err != nil {
		return err
	}

	if !call.IsSuccess() {
		return call.Error()
	}

	wechat.refreshBaseInfo()
	wechat.refreshCookieCache(resp.Cookies())

	return nil
}

// Execute a http request by default http client.
func (wechat *WeChat) Execute(path string, body io.Reader, call Caller) error {
	method := "GET"
	if body != nil {
		method = "POST"
	}
	req, err := http.NewRequest(method, path, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set(`User-Agent`, `Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_2) AppleWebKit/602.3.12 (KHTML, like Gecko) Version/10.0.2 Safari/602.3.12`)

	return wechat.ExecuteRequest(req, call)
}

// PassTicketKV return a string like `pass_ticket=1234s`
func (wechat *WeChat) PassTicketKV() string {
	return fmt.Sprintf(`pass_ticket=%s`, wechat.BaseRequest.PassTicket)
}

// SkeyKV return a string like `skey=1234s`
func (wechat *WeChat) SkeyKV() string {
	return fmt.Sprintf(`skey=%s`, wechat.BaseRequest.Skey)
}
