package main

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"

	"github.com/bcmmacro/bridging-go/library/common"
	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	http2 "github.com/bcmmacro/bridging-go/library/http"
)

type Handler struct {
	forwarder *Forwarder
	upgrader  *websocket.Upgrader
}

func NewHandler(corsCheck *cors.Cors) *Handler {
	return &Handler{forwarder: NewForwarder(), upgrader: &websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			if r.URL.Path == "/bridge" {
				return true
			}
			return corsCheck.OriginAllowed(r)
		}}}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, logger := common.CorrIDCtxLogger(r.Context())
	logger.Infof("recv %s %s %s", r.Method, r.RemoteAddr, r.URL.String())

	isWebsocket := r.Header.Get("Upgrade") == "websocket"
	if isWebsocket {
		conn, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warnf("failed to upgrade websocket %v", err)
			http2.WriteErr(w, r, errors2.ErrInternal)
			return
		}

		defer func() {
			conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""), time.Now().Add(time.Second*3))
			conn.Close()
			logger.Infof("closed websocket")
		}()

		if r.URL.Path == "/bridge" {
			// TODO(zzl) change to Query param
			bridgingToken := r.Header.Get("bridging-token")
			h.forwarder.Serve(ctx, bridgingToken, conn)
		} else {
			wsID, err := h.forwarder.ForwardOpenWebsocket(ctx, r, conn)
			if err != nil {
				logger.Warnf("failed to open websocket error[%v]", err)
			} else {
				for {
					msgType, msg, err := conn.ReadMessage()
					ctx, logger = common.CorrIDCtxLogger(r.Context())
					if err != nil {
						logger.Warnf("failed to read websocket error[%v]", err)
						break
					}
					if msgType == websocket.BinaryMessage || msgType == websocket.TextMessage {
						h.forwarder.ForwardWebsocketMsg(ctx, wsID, conn, msg)
					} else {
						logger.Infof("drop message type[%d]", msgType)
					}
				}
				h.forwarder.ForwardCloseWebsocket(ctx, wsID, conn)
			}
		}
	} else {
		h.forwarder.ForwardHTTP(ctx, w, r)
	}
}
