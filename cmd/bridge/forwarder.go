package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/bcmmacro/bridging-go/internal/proto"
	"github.com/bcmmacro/bridging-go/library/errors"
	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	http2 "github.com/bcmmacro/bridging-go/library/http"
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

func (f *Forwarder) ForwardHTTP(w http.ResponseWriter, r *http.Request) {
	if f.bridge == nil {
		http2.WriteErr(w, r, errors.ErrInternal.WithData("bridge is not up"))
		return
	}

	if r.Header.Get("bridging-base-url") == "" {
		http2.WriteErr(w, r, errors2.ErrBadRequest)
		return
	}

	args, err := proto.MakeHTTPReqArgs(r)
	if err != nil {
		http2.WriteErr(w, r, errors2.ErrBadRequest)
		return
	}

	resp, err := f.req("http", args)

	w.WriteHeader(int(resp.StatusCode))
	for k, v := range resp.Headers {
		w.Header()[k] = []string{v}
	}
	w.Write([]byte(resp.Content))
}

func (f *Forwarder) ForwardOpenWebsocket(r *http.Request, ws *websocket.Conn) (string, error) {
	if f.bridge == nil {
		return "", fmt.Errorf("invalid")
	}
	bridgingBaseURL := r.URL.Query().Get("bridging-base-url")
	if bridgingBaseURL == "" {
		return "", fmt.Errorf("invalid")
	}
	wsID := uuid.New().String()
	args, err := proto.MakeHTTPReqArgs(r)
	if err != nil {
		return "", err
	}
	args.WSID = wsID
	resp, err := f.req("open_websocket", args)
	if err != nil {
		return "", err
	}
	if resp.Exception != "" {
		logrus.Warnf("failed to open websocket error[%v]", resp.Exception)
		return "", fmt.Errorf(resp.Exception)
	}
	f.mutex.Lock()
	f.wss[wsID] = ws
	f.mutex.Unlock()
	return wsID, nil
}

func (f *Forwarder) ForwardWebsocketMsg(wsID string, ws *websocket.Conn, msg []byte) error {
	if f.bridge == nil {
		return fmt.Errorf("invalid")
	}

	p := proto.Packet{
		CorrID: "0",
		Method: "websocket_msg",
		Args:   &proto.Args{WSID: wsID, Msg: string(msg)}}
	return f.send(&p)
}

func (f *Forwarder) ForwardCloseWebsocket(wsID string, ws *websocket.Conn) error {
	if f.bridge == nil {
		return fmt.Errorf("invalid")
	}

	f.mutex.Lock()
	if _, ok := f.wss[wsID]; !ok {
		f.mutex.Unlock()
		return nil
	}
	f.mutex.Unlock()

	_, err := f.req("close_websocket", &proto.Args{WSID: wsID})
	f.mutex.Lock()
	delete(f.wss, wsID)
	f.mutex.Unlock()
	return err
}

func (f *Forwarder) Serve(bridging_token string, ws *websocket.Conn) {
	client := ws.RemoteAddr().String()
	logrus.Infof("connected bridge client[%s]", client)

	if f.bridge != nil {
		logrus.Infof("duplicate bridge ws connection client[%s]", client)
		return
	}

	if bridging_token != f.bridgingToken {
		logrus.Infof("invalid bridge token client[%s]", client)
		return
	}

	f.bridge = ws

	for {
		_, buf, err := ws.ReadMessage()
		if err != nil {
			logrus.Warnf("reading from bridge ws error[%v]", err)
			break
		}
		logrus.Debugf("read %d", len(buf))
		msg, err := proto.Deserialize(buf)
		if err != nil {
			continue
		}

		if msg.CorrID == "0" {
			if msg.Method == "close_websocket" {
				logrus.Infof("recv msg[%v]", msg)
				wsID := msg.Args.WSID
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
			} else if msg.Method == "websocket_msg" {
				logrus.Debugf("recv msg[%v]", msg)
				wsID := msg.Args.WSID
				f.mutex.Lock()
				ws, ok := f.wss[wsID]
				f.mutex.Unlock()
				if ok {
					ws.WriteMessage(websocket.TextMessage, []byte(msg.Args.Msg))
				}
			}
		} else {
			logrus.Infof("recv msg[%v]", msg)

			f.mutex.Lock()
			if ch, ok := f.reqs[msg.CorrID]; ok {
				ch <- msg.Args
			}
			f.mutex.Unlock()
		}
	}

	logrus.Info("bridge is disconnected")
	f.bridge = nil
}

func (f *Forwarder) req(method string, args *proto.Args) (*proto.Args, error) {
	corr_id := uuid.New().String()
	c := make(chan *proto.Args)

	f.mutex.Lock()
	f.reqs[corr_id] = c
	f.mutex.Unlock()

	defer func() {
		f.mutex.Lock()
		delete(f.reqs, corr_id)
		f.mutex.Unlock()
	}()

	var p = proto.Packet{CorrID: corr_id, Method: method, Args: args}
	if err := f.send(&p); err != nil {
		return nil, err
	}

	return <-c, nil
}

func (f *Forwarder) send(p *proto.Packet) error {
	msg, err := p.Serialize(int(f.compressLevel))
	if err != nil {
		logrus.Warnf("failed to serialize %s", *p)
		return err
	}
	logrus.Infof("send [%s]", p)
	err = f.bridge.WriteMessage(websocket.BinaryMessage, msg)
	if err != nil {
		logrus.Warnf("failed to send %v", err)
	}
	return err
}
