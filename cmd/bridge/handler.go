package main

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	http2 "github.com/bcmmacro/bridging-go/library/http"
)

type Handler struct {
	forwarder *Forwarder
}

func NewHandler() *Handler {
	return &Handler{forwarder: NewForwarder()}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO(zzl) insert logger to context and set req ID
	logrus.Infof("recv %s %s %s", r.Method, r.RemoteAddr, r.URL.String())
	if strings.ToUpper(r.Method) == "GET" && r.URL.Path == "/bridge" {
		// TODO(zzl) buf size should be configurable
		var upgrader = websocket.Upgrader{ReadBufferSize: 32 * 1024 * 1024, WriteBufferSize: 32 * 1024 * 1024}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logrus.Warnf("failed to upgrade bridge websocket %v", err)
			http2.WriteErr(w, r, errors2.ErrInternal)
			return
		}
		// TODO(zzl) change to Query param
		bridging_token := r.Header.Get("bridging-token")
		h.forwarder.Serve(bridging_token, conn)
		conn.Close()
		logrus.Info("bridge is closed")
	} else {
		h.forwarder.ForwardHTTP(w, r)
	}
}
