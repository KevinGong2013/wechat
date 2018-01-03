package main

import (
	"fmt"
	"time"

	"github.com/KevinGong2013/wechat"
)

func main() {

	bot, err := wechat.NewBot(nil)
	if err != nil {
		panic(err)
	}

	bot.Handle(`/msg/solo`, func(evt wechat.Event) {
		data := evt.Data.(wechat.EventMsgData)
		fmt.Println(`/msg/solo/` + data.Content)
	})

	bot.Handle(`/msg/group`, func(evt wechat.Event) {
		data := evt.Data.(wechat.EventMsgData)
		fmt.Println(`/msg/group/` + data.Content)
	})

	bot.Handle(`/contact`, func(evt wechat.Event) {
		data := evt.Data.(wechat.EventContactData)
		fmt.Printf(`/contact/%v`, data.Contact.NickName)
	})

	bot.Handle(`/login`, func(arg2 wechat.Event) {
		isSuccess := arg2.Data.(int) == 1
		if isSuccess {
			fmt.Println(`login Success`)
			cs, err := bot.SearchContact(`Chris`, `朝阳区`, wechat.Any, wechat.Any)
			if err != nil {
				fmt.Errorf("%v", err)
			} else {
				fmt.Print(cs)
			}
		} else {
			fmt.Println(`login Failed`)
		}
	})

	// 60s 发一次消息
	bot.AddTimer(60 * time.Second)
	bot.Handle(`/timer/60s`, func(arg2 wechat.Event) {
		data := arg2.Data.(wechat.EventTimerData)
		if bot.IsLogin {
			bot.SendTextMsg(fmt.Sprintf(`第%v次`, data.Count), `filehelper`)
		}
	})

	// 9:00 每天9点发一条消息
	bot.AddTiming(`18:00`)
	bot.Handle(`/timing/9:00`, func(arg2 wechat.Event) {
		// data := arg2.Data.(wechat.EventTimingtData)
		bot.SendTextMsg(`9:00 了`, `filehelper`)
	})

	bot.Go()
}
