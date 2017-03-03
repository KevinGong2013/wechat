package wechat

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Event ...
type Event struct {
	Type string
	Path string
	From string
	To   string
	Data interface{}
	Time int64
}

const (
	// Delete 删除联系人
	Delete = 0
	// Modify 有人修改了自己的信息
	Modify = 1
)

// EventContactData 通讯录中删人 或者有人修改资料的时候
type EventContactData struct {
	ChagngeType int
	GGID        string
}

// EventMsgData 新消息
type EventMsgData struct {
	IsGroupMsg       bool
	IsMediaMsg       bool
	IsSendedByMySelf bool
	MsgType          int64
	AtMe             bool
	MediaURL         string
	Content          string
	FromUserName     string
	FromGGID         string
	SenderUserName   string
	SenderGGID       string
	OriginalMsg      map[string]interface{}
}

// EventTimerData ...
type EventTimerData struct {
	Duration time.Duration
	Count    uint64
}

// EventTimingtData ...
type EventTimingtData struct {
	Count uint64
}

type evtStream struct {
	sync.RWMutex
	srcMap      map[string]chan Event
	stream      chan Event
	wg          sync.WaitGroup
	sigStopLoop chan Event
	Handlers    map[string]func(Event)
	hook        func(Event)
	serverEvt   chan Event
}

func newEvtStream() *evtStream {
	return &evtStream{
		srcMap:      make(map[string]chan Event),
		stream:      make(chan Event),
		Handlers:    make(map[string]func(Event)),
		sigStopLoop: make(chan Event),
		serverEvt:   make(chan Event, 10),
	}
}

func (es *evtStream) init() {
	es.merge("internal", es.sigStopLoop)
	es.merge(`serverEvent`, es.serverEvt)

	go func() {
		es.wg.Wait()
		close(es.stream)
	}()
}

func (es *evtStream) merge(name string, ec chan Event) {
	es.Lock()
	defer es.Unlock()

	es.wg.Add(1)
	es.srcMap[name] = ec

	go func(a chan Event) {
		for n := range a {
			n.From = name
			es.stream <- n
		}
		es.wg.Done()
	}(ec)
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	return path.Clean(p)
}

func isPathMatch(pattern, path string) bool {
	if len(pattern) == 0 {
		return false
	}
	n := len(pattern)
	return len(path) >= n && path[0:n] == pattern
}

func findMatch(mux map[string]func(Event), path string) string {
	n := -1
	pattern := ""
	for m := range mux {
		if !isPathMatch(m, path) {
			continue
		}
		if len(m) > n {
			pattern = m
			n = len(m)
		}
	}
	return pattern
}

func (es *evtStream) match(path string) string {
	return findMatch(es.Handlers, path)
}

// Go 皮皮虾我们走
func (wechat *WeChat) Go() {
	es := wechat.evtStream

	logger.Debug(`------------all handlers------------`)
	logger.Debug(es.Handlers)

	for e := range es.stream {
		switch e.Path {
		case "/sig/stoploop":
			return
		}
		go func(a Event) {
			es.RLock()
			defer es.RUnlock()
			if pattern := es.match(a.Path); pattern != "" {
				es.Handlers[pattern](a)
			}
		}(e)
		if es.hook != nil {
			es.hook(e)
		}
	}
}

// Stop 皮皮虾快停下
func (wechat *WeChat) Stop() {
	es := wechat.evtStream
	go func() {
		e := Event{
			Path: "/sig/stoploop",
		}
		es.sigStopLoop <- e
	}()
}

// Handle 处理消息，联系人，登录态 等等 所有东西
func (wechat *WeChat) Handle(path string, handler func(Event)) {
	wechat.evtStream.Handlers[cleanPath(path)] = handler
}

// Hook modify event on fly
func (wechat *WeChat) Hook(f func(Event)) {
	es := wechat.evtStream
	es.hook = f
}

// ResetHandlers remove all regeisted handler
func (wechat *WeChat) ResetHandlers() {
	for Path := range wechat.evtStream.Handlers {
		delete(wechat.evtStream.Handlers, Path)
	}
	return
}

// NewTimerCh ...
func newTimerCh(du time.Duration) chan Event {
	t := make(chan Event)

	go func(a chan Event) {
		n := uint64(0)
		for {
			n++
			time.Sleep(du)
			e := Event{}
			e.Path = "/timer/" + du.String()
			e.Time = time.Now().Unix()
			e.Data = EventTimerData{
				Duration: du,
				Count:    n,
			}
			t <- e

		}
	}(t)
	return t
}

// AddTimer ..
func (wechat *WeChat) AddTimer(du time.Duration) {
	wechat.evtStream.merge(`timer`, newTimerCh(du))
}

// NewTimingCh ...
func newTimingCh(hm string) chan Event {

	infos := strings.Split(hm, `:`)
	if len(infos) != 2 {
		panic(`hm incorrect`)
	}
	hour, _ := strconv.Atoi(infos[0])
	minute, _ := strconv.Atoi(infos[1])

	t := make(chan Event)

	go func(a chan Event) {
		n := uint64(0)
		for {
			now := time.Now()
			nh, nm, _ := now.Clock()
			next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
			if n > 0 || hour > nh || (hour == nh && minute < nm) {
				next = next.Add(time.Hour * 24)
			}
			logger.Debugf(`next timing %v`, next)
			n++
			time.Sleep(next.Sub(now))
			e := Event{}
			e.Path = `/timing/` + hm
			e.Time = time.Now().Unix()
			e.Data = EventTimingtData{
				Count: n,
			}
			t <- e
		}
	}(t)
	return t
}

// AddTiming ...
func (wechat *WeChat) AddTiming(hm string) {
	wechat.evtStream.merge(`timing`, newTimingCh(hm))
}

func (es *evtStream) emitContactChangeEvent(ggid string, ct int) {
	data := EventContactData{
		ChagngeType: ct,
		GGID:        ggid,
	}
	path := `/del`
	if ct != Delete {
		path = `/mod`
	}
	event := Event{
		Type: `ContactChange`,
		From: `Server`,
		Path: `/contact` + path,
		To:   `End`,
		Time: time.Now().Unix(),
		Data: data,
	}
	es.serverEvt <- event
}

func (wechat *WeChat) emitNewMessageEvent(m map[string]interface{}) {

	fromUserName := m[`FromUserName`].(string)
	senderUserName := fromUserName
	toUserName := m[`ToUserName`].(string)
	content := m[`Content`].(string)
	isSendedByMySelf := fromUserName == wechat.MySelf.UserName
	var groupUserName string
	if strings.HasPrefix(fromUserName, `@@`) {
		groupUserName = fromUserName
	} else if strings.HasPrefix(toUserName, `@@`) {
		groupUserName = toUserName
	}
	isGroupMsg := false
	if len(groupUserName) > 0 {
		isGroupMsg = true
		wechat.UpateGroupIfNeeded(groupUserName)
	}
	msgType := m[`MsgType`].(float64)
	mid := m[`MsgId`].(string)

	isMediaMsg := false
	mediaURL := ``
	path := ``

	switch msgType {
	case 3:
		path = `webwxgetmsgimg`
	case 47:
		pid, _ := m[`HasProductId`].(float64)
		if pid == 0 {
			path = `webwxgetmsgimg`
		}
	case 34:
		path = `webwxgetvoice`
	case 43:
		path = `webwxgetvideo`
	}
	if len(path) > 0 {
		isMediaMsg = true
		mediaURL = fmt.Sprintf(`%v/%s?msgid=%v&%v`, wechat.BaseURL, path, mid, wechat.SkeyKV())
	}
	isAtMe := false
	if isGroupMsg && !isSendedByMySelf {
		atme := `@`
		if len(wechat.MySelf.DisplayName) > 0 {
			atme += wechat.MySelf.DisplayName
		} else {
			atme += wechat.MySelf.NickName
		}
		isAtMe = strings.Contains(content, atme)

		infos := strings.Split(content, `:<br/>`)
		if len(infos) != 2 {
			return
		}

		contact, err := wechat.ContactByUserName(infos[0])
		if err != nil {
			wechat.FourceUpdateGroup(groupUserName)
			logger.Errorf(`can't find contact info, so ignore this message %s`, m)
			return
		}

		senderUserName = contact.UserName
		content = infos[1]
	}

	data := EventMsgData{
		IsGroupMsg:       isGroupMsg,
		IsMediaMsg:       isMediaMsg,
		IsSendedByMySelf: isSendedByMySelf,
		MsgType:          int64(msgType),
		AtMe:             isAtMe,
		MediaURL:         mediaURL,
		Content:          content,
		FromUserName:     fromUserName,
		FromGGID:         wechat.cache.userGG[fromUserName], // TODO 不应该直接从字典里取
		SenderUserName:   senderUserName,
		SenderGGID:       wechat.cache.userGG[senderUserName],
		OriginalMsg:      m,
	}
	evtPath := `/solo`
	if isGroupMsg {
		evtPath = `/group`
	}
	event := Event{
		Type: `NewMessage`,
		From: `Server`,
		Path: `/msg` + evtPath,
		To:   `End`,
		Time: time.Now().Unix(),
		Data: data,
	}
	wechat.evtStream.serverEvt <- event
}

func (wechat *WeChat) handleServerEvent(resp *syncMessageResponse) {

	es := wechat.evtStream

	if resp.DelContactCount > 0 {
		logger.Debug(resp.DelContactList)
		for _, v := range resp.DelContactList {
			ggid := wechat.cache.userGG[v[`UserName`].(string)]
			go es.emitContactChangeEvent(ggid, Delete)
		}
	}
	if resp.ModContactCount > 0 {
		for _, v := range resp.ModContactList {
			logger.Debug(resp.ModContactList)
			ggid := wechat.cache.userGG[v[`UserName`].(string)]
			go es.emitContactChangeEvent(ggid, Modify)
		}
	}
	if resp.ModChatRoomMemberCount > 0 {
		logger.Error(`resp.ModChatRoomMemberCount TODO`)
	}
	if resp.AddMsgCount > 0 {
		logger.Debug(resp.AddMsgList)
		for _, v := range resp.AddMsgList {
			go wechat.emitNewMessageEvent(v)
		}
	}
}
