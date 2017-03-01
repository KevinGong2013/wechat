package wechat

import (
	"github.com/skratchdot/open-golang/open"
)

// implements UUIDProcessor
type defaultUUIDProcessor struct {
	path string
}

func (dp *defaultUUIDProcessor) ProcessUUID(uuid string) error {
	// 2.``
	path, err := fetchORCodeImage(uuid)

	if err != nil {
		return err
	}
	logger.Debugf(`qrcode image path: %s`, path)

	// 3.
	go func() {
		dp.path = path
		open.Start(path)
	}()
	logger.Info(`please scan ORCode by wechat mobile application`)

	return nil
}

func (dp *defaultUUIDProcessor) UUIDDidConfirm(err error) {
	if len(dp.path) > 0 {
		deleteFile(dp.path)
	}
}
