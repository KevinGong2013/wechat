package wechat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/emirpasic/gods/utils"
	"sync"
)

type cache contactCache

type contactCache struct {
	sync.Mutex
	contacts map[string]*Contact
}

// Contact is wx Account struct
type Contact struct {
	UserName        string
	NickName        string
	HeadImgURL      string `json:"HeadImgUrl"`
	HeadHash        string
	RemarkName      string
	DisplayName     string
	StarFriend      float64
	Sex             float64
	Signature       string
	VerifyFlag      float64
	ContactFlag     float64
	HeadImgFlag     float64
	Province        string
	City            string
	Alias           string
	EncryChatRoomID string `json:"EncryChatRoomId"`
	Type            int
	MemberList      []*Contact
}

const (
	// Offical 公众号 ...
	Offical = 0
	// Friend 好友 ...
	Friend = 1
	// Group 群组 ...
	Group = 2
	// Member 群组成员 ...
	Member = 3
	// FriendAndMember 即是好友也是群成员 ...
	FriendAndMember = 4
)

func newCache() *cache {
	return &cache{
		contacts: make(map[string]*Contact),
	}
}

func (c *cache) updateContact(v map[string]interface{}) {
	nc, err := newContact(v)
	if err != nil {
		logger.Errorf(`create contact failed error: %v`, err)
	} else {
		if len(nc.UserName) > 0 {
			oc := c.contacts[nc.NickName]
			if oc != nil {
				logger.Info(`old contact: %v will be replaced by %v`, oc, nc)
			}
			c.contacts[nc.UserName] = nc
		} else {
			logger.Warningf(`bad contact %v`, v)
		}
	}
}

func (c *cache) clearContactBy(username string) {
	delete(c.contacts, username)
}

func (c *cache) clear() {
	c.contacts = make(map[string]*Contact)
}

func (wechat *WeChat) syncContacts(cts []map[string]interface{}) {
	wechat.cache.Lock()
	defer wechat.cache.Unlock()

	logger.Debugf(`count of contact [contain group member]: [%d]`, len(cts))

	for _, v := range cts {
		wechat.cache.updateContact(v)
	}

	var buffer bytes.Buffer
	for _, c := range wechat.cache.contacts {
		des := "[群成员]"
		if c.Type == FriendAndMember {
			des = "[群友兼容好友]"
		} else if c.Type == Friend {
			des = "[好友]"
		} else if c.Type == Offical {
			des = "[公众号]"
		}
		buffer.WriteString(des)
		buffer.WriteString(fmt.Sprintf(" %v", utils.ReplaceEmoji(c.NickName)))
		buffer.WriteString(fmt.Sprintf(" [Sex: %v", c.Sex))
		buffer.WriteString(fmt.Sprintf(" City: %v]", c.City))
		buffer.WriteString("\n")
	}
	logger.Debug(buffer.String())
}

// 修改在这里处理
func (wechat *WeChat) appendContacts(cts []map[string]interface{}) {
	wechat.cache.Lock()
	defer wechat.cache.Unlock()

	for _, v := range cts {
		wechat.cache.updateContact(v)
	}
}

func (wechat *WeChat) removeContact(username string) {
	wechat.cache.Lock()
	wechat.cache.clearContactBy(username)
	wechat.cache.Unlock()
}

func (c *cache) contactByUserName(un string) (*Contact, error) {
	c.Lock()
	defer c.Unlock()
	if contact, found := c.contacts[un]; found {
		return contact, nil
	}
	return nil, errors.New(`not Found`)
}

func newContact(m map[string]interface{}) (*Contact, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var c *Contact
	err = json.Unmarshal(data, &c)
	return c, err
}
