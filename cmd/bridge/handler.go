package main

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/cors"

	"github.com/bcmmacro/bridging-go/library/common"
	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	http2 "github.com/bcmmacro/bridging-go/library/http"
	"github.com/bcmmacro/bridging-go/library/log"
)

type Handler struct {
	corsCheck *cors.Cors
	forwarder *Forwarder
}

func NewHandler(corsCheck *cors.Cors) *Handler {
	return &Handler{forwarder: NewForwarder(), corsCheck: corsCheck}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, logger := log.WithField(r.Context(), "ReqID", common.RandomInt64())
	r = r.WithContext(ctx)
	logger.Infof("recv %s %s %s", r.Method, r.RemoteAddr, r.URL.String())

	isWebsocket := r.Header.Get("Upgrade") == "websocket"
	if isWebsocket {
		// TODO(zzl) buf size should be configurable
		var isBridge = r.URL.Path == "/bridge"
		var upgrader = websocket.Upgrader{
			ReadBufferSize:  32 * 1024 * 1024,
			WriteBufferSize: 32 * 1024 * 1024,
			CheckOrigin: func(r *http.Request) bool {
				if isBridge {
					return true
				}
				return h.corsCheck.OriginAllowed(r)
			}}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Warnf("failed to upgrade websocket %v", err)
			http2.WriteErr(w, r, errors2.ErrInternal)
			return
		}

		if isBridge {
			// TODO(zzl) change to Query param
			bridgingToken := r.Header.Get("bridging-token")
			h.forwarder.Serve(ctx, bridgingToken, conn)
		} else {
			wsID, err := h.forwarder.ForwardOpenWebsocket(ctx, r, conn)
			if err != nil {
				logger.Warnf("failed to open websocket error[%v]", err)
			} else {
				for {
					_, msg, err := conn.ReadMessage()
					if err != nil {
						logger.Warnf("failed to read websocket error[%v]", err)
						break
					}
					h.forwarder.ForwardWebsocketMsg(ctx, wsID, conn, msg)
				}
			}
			h.forwarder.ForwardCloseWebsocket(ctx, wsID, conn)
		}
		conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""), time.Now().Add(time.Second*3))
		conn.Close()
	} else {
		h.forwarder.ForwardHTTP(ctx, w, r)
	}
}
