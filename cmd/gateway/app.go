package main

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/bcmmacro/bridging-go/internal/config"
	"github.com/bcmmacro/bridging-go/library/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

type Gateway struct {
	bridge *websocket.Conn
	wss    map[int64]*websocket.Conn
}

func (gw *Gateway) Run(conf *config.Config) {
	bridge_netloc := conf.BridgeNetLoc
	bridge_token := conf.BridgeToken
	gw.connect(bridge_netloc, bridge_token)
}

func (gw *Gateway) connect(bridge_netloc string, bridge_token string) {
	bridgeURL, err := url.Parse(bridge_netloc + "/bridge")
	errors.Check(err)
	origin, _ := url.Parse("http://localhost/")
	wsConf := websocket.Config{
		Location: bridgeURL,
		Origin:   origin,
		Version:  websocket.ProtocolVersionHybi13,
		Header:   http.Header{"bridging-token": []string{bridge_token}},
	}
	wss, err := websocket.DialConfig(&wsConf)
	fmt.Print(wsConf, wss)
	errors.Check(err)
	logrus.Info("Connected to bridge")
	gw.bridge = wss
}
