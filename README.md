# wechat
Awaken your wechat bot.

## INSTALLATION
```go
go get github.com/KevinGong2013/ggbot/wechat
```

## Basic Usage

```go
import "github.com/KevinGong2013/wechat"

// awaken a bot
bot, _ := wechat.AwakenNewBot(nil)
bot.Go() // begin handle everything
```

## Login State
```go
bot.Handle(`/login`, func(arg2 wechat.Event) {
	isSuccess := arg2.Data.(int) == 1
	if isSuccess {
		fmt.Println(`login Success`)
	} else {
		fmt.Println(`login Failed`)
	}
})
```

## Contact
### Get
``` go
// all contacts

bot.AllContacts()

// get contact by `UserName`
contact, _ := bot.ContactByUserName(UserName)

// get contact by `NickName`
contacts, _ := bot.ContactsByNickName(NickName)
```
### Change
```go
// handle contact change event
bot.Handle(`/contact`, func(evt wechat.Event) {
	data := evt.Data.(wechat.EventContactData)
	fmt.Println(`contact change event` + data.GGID)
})
```

## Message
### Send
```go
to := `filehelper`
// text message
bot.SendTextMsg(`Text`, to)
// video message
bot.SendFile(`testResource/test.mov`, to)
// image message
bot.SendFile(`testResource/test.png`, to)
// emoticon message
bot.SendFile(`testResource/test.gif`, to)
// file message
bot.SendFile(`testResource/test.txt`, to)
bot.SendFile(`testResource/test.mp3`, to)
```
### Receive
```go
// all solo msg
bot.Handle(`/msg/solo`, func(evt wechat.Event) {
	data := evt.Data.(wechat.EventMsgData)
	fmt.Println(`/msg/solo/` + data.Content)
})

// all group msg
bot.Handle(`/msg/group`, func(evt wechat.Event) {
	data := evt.Data.(wechat.EventMsgData)
	fmt.Println(`/msg/group/` + data.Content)
})
```

## Convenice
```go
bot.AddTimer(5 * time.Second)
bot.Handle(`/timer/5s`, func(arg2 wechat.Event) {
	data := arg2.Data.(wechat.EventTimerData)
	if bot.IsLogin {
		bot.SendTextMsg(fmt.Sprintf(`%v times`, data.Count), `filehelper`)
	}
})

bot.AddTiming(`9:00`)
bot.Handle(`/timing/9:00`, func(arg2 wechat.Event) {
	// data := arg2.Data.(wechat.EventTimingtData)
	bot.SendTextMsg(`9:00`, `filehelper`)
})
```
