package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

func setUpRoute(router *httprouter.Router, handler *handler) {
	router.GET("/*path", handler.handleGet)
}

type handler struct {
	forwarder *Forwarder
}

func NewHandler() *handler {
	return &handler{forwarder: NewForwarder()}
}

func (h *handler) handleGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logrus.Infof("recv GET %s %s", r.RemoteAddr, r.URL.String())
	if r.URL.Path == "/bridge" {
		h.serveBridge(w, r, ps)
	}
}

func (h *handler) serveBridge(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	websocket.Handler(h.handleBridge).ServeHTTP(w, r)
}

func (h *handler) handleBridge(conn *websocket.Conn) {
	h.forwarder.Serve(conn)
	conn.Close()
}
