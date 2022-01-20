package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/bcmmacro/bridging-go/internal/proto"
	"github.com/bcmmacro/bridging-go/library/common"
	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	http2 "github.com/bcmmacro/bridging-go/library/http"
	"github.com/bcmmacro/bridging-go/library/log"
)

type Forwarder struct {
	bridgingToken string
	compressLevel int64
	bridge        *websocket.Conn
	mutex         sync.Mutex
	reqs          map[string]chan *proto.Args
	wss           map[string]*websocket.Conn
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
	return &Forwarder{
		bridgingToken: os.Getenv("BRIDGE_TOKEN"),
		compressLevel: level,
		reqs:          make(map[string]chan *proto.Args),
		wss:           make(map[string]*websocket.Conn)}
}

func (f *Forwarder) ForwardHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if f.bridge == nil {
		http2.WriteErr(w, r, errors2.ErrInternal)
		return
	}

	if r.Header.Get("bridging-base-url") == "" {
		http2.WriteErr(w, r, errors2.ErrBadRequest)
		return
	}

	args, err := proto.MakeHTTPReqArgs(ctx, r)
	if err != nil {
		http2.WriteErr(w, r, errors2.ErrBadRequest)
		return
	}

	resp, err := f.req(ctx, proto.HTTP, args)
	if err != nil {
		http2.WriteErr(w, r, errors2.ErrInternal)
		return
	}

	w.WriteHeader(int(resp.StatusCode))
	for k, v := range resp.Headers {
		w.Header()[k] = []string{v}
	}
	w.Write([]byte(resp.Content))
}

func (f *Forwarder) ForwardOpenWebsocket(ctx context.Context, r *http.Request, ws *websocket.Conn) (string, error) {
	logger := log.Ctx(ctx)
	if f.bridge == nil {
		return "", fmt.Errorf("invalid")
	}
	bridgingBaseURL := r.URL.Query().Get("bridging-base-url")
	if bridgingBaseURL == "" {
		return "", fmt.Errorf("invalid")
	}
	wsID := uuid.New().String()
	args, err := proto.MakeHTTPReqArgs(ctx, r)
	if err != nil {
		return "", err
	}
	args.WSID = wsID
	resp, err := f.req(ctx, proto.OPEN_WEBSOCKET, args)
	if err != nil {
		return "", err
	}
	if resp.Exception != "" {
		logger.Warnf("failed to open websocket error[%v]", resp.Exception)
		return "", fmt.Errorf(resp.Exception)
	}
	f.mutex.Lock()
	f.wss[wsID] = ws
	f.mutex.Unlock()
	return wsID, nil
}

func (f *Forwarder) ForwardWebsocketMsg(ctx context.Context, wsID string, ws *websocket.Conn, msg []byte) error {
	if f.bridge == nil {
		return fmt.Errorf("invalid")
	}

	_, corrID := common.CorrIDCtx(ctx)
	p := proto.Packet{
		CorrID: corrID,
		Method: proto.WEBSOCKET_MSG,
		Args:   &proto.Args{WSID: wsID, Msg: string(msg)}}
	return f.send(ctx, &p)
}

func (f *Forwarder) ForwardCloseWebsocket(ctx context.Context, wsID string, ws *websocket.Conn) error {
	if f.bridge == nil {
		return fmt.Errorf("invalid")
	}

	f.mutex.Lock()
	if _, ok := f.wss[wsID]; !ok {
		f.mutex.Unlock()
		return nil
	}
	f.mutex.Unlock()

	_, err := f.req(ctx, proto.CLOSE_WEBSOCKET, &proto.Args{WSID: wsID})
	f.mutex.Lock()
	delete(f.wss, wsID)
	f.mutex.Unlock()
	return err
}

func (f *Forwarder) Serve(ctx context.Context, bridgingToken string, ws *websocket.Conn) {
	logger := log.Ctx(ctx)
	client := ws.RemoteAddr().String()
	logger.Infof("connected bridge client[%s]", client)

	if f.bridge != nil {
		logger.Infof("duplicate bridge ws connection client[%s]", client)
		return
	}

	if bridgingToken != f.bridgingToken {
		logger.Infof("invalid bridge token client[%s]", client)
		return
	}

	f.bridge = ws

	for {
		msgType, buf, err := ws.ReadMessage()
		if err != nil {
			logger.Warnf("reading from bridge ws error[%v]", err)
			break
		}
		logger.Debugf("read %d", len(buf))
		if msgType != websocket.BinaryMessage {
			logger.Infof("drop msg type[%d]", msgType)
			continue
		}
		packet, err := proto.Deserialize(ctx, buf)
		if err != nil {
			continue
		}
		// use the CorrID from the message
		_, logger2 := common.CorrIDCtxLogger(common.CtxWithCorrID(ctx, packet.CorrID))

		if packet.Method == proto.CLOSE_WEBSOCKET {
			logger2.Infof("recv [%v]", packet)
			wsID := packet.Args.WSID
			f.mutex.Lock()
			ws, ok := f.wss[wsID]
			if ok {
				delete(f.wss, wsID)
			}
			f.mutex.Unlock()
			if ok {
				ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""), time.Now().Add(time.Second*3))
				ws.Close()
			}
		} else if packet.Method == proto.WEBSOCKET_MSG {
			logger2.Debugf("recv [%v]", packet)
			wsID := packet.Args.WSID
			f.mutex.Lock()
			ws, ok := f.wss[wsID]
			f.mutex.Unlock()
			if ok {
				ws.WriteMessage(websocket.TextMessage, []byte(packet.Args.Msg))
			}
		} else {
			logger2.Infof("recv [%v]", packet)

			f.mutex.Lock()
			ch, ok := f.reqs[packet.CorrID]
			f.mutex.Unlock()
			if ok {
				ch <- packet.Args
				close(ch)
			}
		}
	}

	logger.Info("bridge is disconnected")
	f.bridge = nil
}

func (f *Forwarder) req(ctx context.Context, method proto.PacketMethod, args *proto.Args) (*proto.Args, error) {
	_, corrID := common.CorrIDCtx(ctx)
	c := make(chan *proto.Args)

	f.mutex.Lock()
	f.reqs[corrID] = c
	f.mutex.Unlock()

	defer func() {
		f.mutex.Lock()
		delete(f.reqs, corrID)
		f.mutex.Unlock()
	}()

	var p = proto.Packet{CorrID: corrID, Method: method, Args: args}
	if err := f.send(ctx, &p); err != nil {
		return nil, err
	}

	return <-c, nil
}

func (f *Forwarder) send(ctx context.Context, p *proto.Packet) error {
	logger := log.Ctx(ctx)
	msg, err := p.Serialize(ctx, int(f.compressLevel))
	if err != nil {
		return err
	}
	logger.Infof("send [%s]", p)
	err = f.bridge.WriteMessage(websocket.BinaryMessage, msg)
	if err != nil {
		logger.Warnf("failed to send error[%v]", err)
	}
	return err
}
