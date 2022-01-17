package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"

	"github.com/bcmmacro/bridging-go/internal/proto"
	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	http2 "github.com/bcmmacro/bridging-go/library/http"
)

type Forwarder struct {
	bridgingToken string
	compressLevel int64
	bridge        *websocket.Conn
}

func NewForwarder() *Forwarder {
	var level int64 = 9
	levelEnv := os.Getenv("BRIDGE_COMPRESS_LEVEL")
	if levelEnv != "" {
		var err error
		if level, err = strconv.ParseInt(levelEnv, 10, 64); err != nil {
			return nil
		}
	}
	return &Forwarder{bridgingToken: os.Getenv("BRIDGE_TOKEN"), compressLevel: level}
}

func (f *Forwarder) ForwardHTTP(w http.ResponseWriter, r *http.Request) {
	if f.bridge == nil {
		http2.WriteErr(w, r, fmt.Errorf("bridge is not up"))
		return
	}

	if r.Header.Get("bridging-base-url") == "" {
		http2.WriteErr(w, r, errors2.ErrBadRequest)
		return
	}

}

func (f *Forwarder) Serve(ws *websocket.Conn) {
	if f.bridge != nil {
		logrus.Infof("duplicate bridge ws connection client[%s]", ws.Config().Origin)
		return
	}

	if ws.Request().Header.Get("bridging-token") != f.bridgingToken {
		logrus.Infof("invalid bridge token client[%s]", ws.Config().Origin)
		return
	}

	f.bridge = ws

	var buf = make([]byte, 32*1024*1024)
	for {
		size, err := ws.Read(buf)
		if err != nil {
			logrus.Warnf("reading from bridge ws error[%v]", err)
			break
		}
		msg, err := proto.Deserialize(buf[0:size])
		if err != nil {
			logrus.Warnf("invalid msg[%s] error[%v]", buf[0:size], err)
			continue
		}

		if msg.CorrID == "0" {
			if msg.Method == "close_websocket" {
				logrus.Infof("recv msg[%v]", msg)
			} else if msg.Method == "websocket_msg" {
				logrus.Debugf("recv msg[%v]", msg)
			}
		} else {
			logrus.Infof("recv msg[%v]", msg)
		}
	}

	f.bridge = nil
}
