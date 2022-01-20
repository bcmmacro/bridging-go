package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/bcmmacro/bridging-go/internal/config"
	"github.com/bcmmacro/bridging-go/internal/proto"
	"github.com/bcmmacro/bridging-go/library/common"
	"github.com/bcmmacro/bridging-go/library/log"
)

type Gateway struct {
	bridge *websocket.Conn
	ws     map[string]*websocket.Conn
	mutex  sync.Mutex
}

func NewGateway() *Gateway {
	return &Gateway{
		bridge: nil,
		ws:     map[string]*websocket.Conn{},
	}
}

// Run starts the gateway, connects to bridge before accepting incoming requests via websockets.
func (gw *Gateway) Run(conf *config.Config) {
	bridgeNetloc := conf.BridgeNetLoc
	bridgeToken := conf.BridgeToken
	gw.connect(bridgeNetloc, bridgeToken)
}

// connect makes a persistant connection to bridge's websocket, to allow data to flow between private DC and outbound server.
func (gw *Gateway) connect(bridgeNetloc string, bridgeToken string) {
	bridgeURL := bridgeNetloc + "/bridge"
	wss, _, err := websocket.DefaultDialer.Dial(bridgeURL, http.Header{"bridging-token": []string{bridgeToken}})
	if err != nil {
		logrus.Fatal("Dial: ", err) // TODO: implement a retry instead of panicking
	}
	defer wss.Close()

	logrus.Info("Connected to bridge")
	gw.bridge = wss

	for {
		_, wsMsg, err := wss.ReadMessage()
		ctx := context.Background()
		if err != nil {
			logrus.Errorln("Read: ", err)
			continue
		}
		msg, err := proto.Deserialize(ctx, wsMsg)
		if err != nil {
			continue
		}
		ctx, logger := log.WithField(ctx, "ReqID", msg.CorrID)

		logger.Infof("Recv bridge msg: %s\n", msg)

		method := proto.PacketMethod(msg.Method)
		corrID := msg.CorrID
		args := msg.Args

		switch method {
		case proto.HTTP:
			go gw.handleHttp(ctx, corrID, args)
		case proto.OPEN_WEBSOCKET:
			go gw.handleOpenWebsocket(ctx, corrID, args)
		case proto.WEBSOCKET_MSG:
			gw.mutex.Lock()
			conn, present := gw.ws[args.WSID]
			if present {
				send(ctx, conn, args.Msg)
			}
			gw.mutex.Unlock()
		case proto.CLOSE_WEBSOCKET:
			gw.mutex.Lock()
			conn, present := gw.ws[args.WSID]
			if present {
				err := conn.Close()
				if err != nil {
					logger.Warnf("Failed to close downstream websockets connection. Err[%v]", err)
				}
				delete(gw.ws, args.WSID)
			}
			gw.mutex.Unlock()
			gw.send(ctx, createProtoPackage(corrID, proto.CLOSE_WEBSOCKET_RESULT, &proto.Args{WSID: args.WSID}))
		default:
			logger.Warnf("Unsupported method passed down by bridge method[%v]", msg.Method)
		}
	}
}

// send by gateway transmits a packet from gateway to bridge.
func (gw *Gateway) send(ctx context.Context, p *proto.Packet) {
	send(ctx, gw.bridge, p)
}

// send is a generic wrapper to send data to websocket connections,
// if data is of type string, it is converted to json before sending,
// else if data is of type Packet, it is converted to json, then gzipped before sending.
func send(ctx context.Context, conn *websocket.Conn, data interface{}) {
	logger := log.Ctx(ctx)
	logger.Infof("Send msg[%s]\n", data)

	switch p := data.(type) {
	case *proto.Packet:
		msg, err := p.Serialize(ctx, gzip.DefaultCompression)
		if err != nil {
			return
		}
		err = conn.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			logger.Warnf("Failed to transmit packet to bridge")
		}
	case string:
		err := conn.WriteMessage(websocket.TextMessage, []byte(p))
		if err != nil {
			logger.Warnf("Failed to forward websocket message to downstream service")
		}
	default:
		logger.Warnf("Invalid data passed to send function")
	}
}

// handleOpenWebsocket opens a single websocket connection per request with downstream services.
func (gw *Gateway) handleOpenWebsocket(ctx context.Context, corrID string, args *proto.Args) {
	logger := log.Ctx(ctx)
	wsid := args.WSID

	url, err := args.WsUrlTransform()
	if err != nil {
		logger.Warn("Failed to transform url to it's intended destination")
		return
	}
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		logger.Warnf("Failed to open websockets connection with destination[%v]", url)
		return
	}
	defer ws.Close()
	logger.Infof("Connected ws url[%v]\n", url)

	// Store downstream websocket connections in Gateway
	gw.mutex.Lock()
	gw.ws[wsid] = ws
	gw.mutex.Unlock()

	p := createProtoPackage(corrID, proto.OPEN_WEBSOCKET_RESULT, &proto.Args{WSID: wsid})
	gw.send(ctx, p)

	for {
		_, wsMsg, err := ws.ReadMessage()
		if err != nil {
			// Inform bridge that downstream websockets is disconnected
			logger.Warnf("Invalid message received [%v]", err)
			p := createProtoPackage(corrID, proto.CLOSE_WEBSOCKET, &proto.Args{WSID: wsid})
			gw.send(ctx, p)

			gw.mutex.Lock()
			delete(gw.ws, wsid)
			gw.mutex.Unlock()
			break
		}
		// Forward downstream websockets message to bridge
		logger.Debug("Recv msg[%v]", string(wsMsg))
		p := createProtoPackage(corrID, proto.WEBSOCKET_MSG, &proto.Args{WSID: wsid, Msg: string(wsMsg)})
		gw.send(ctx, p)
	}
}

// handleHttp handles incoming http requests by forwarding them to the appropriate services.
func (gw *Gateway) handleHttp(ctx context.Context, corrID string, args *proto.Args) {
	logger := log.Ctx(ctx)
	req, err := deserializeRequest(ctx, args)
	if err != nil {
		logger.Warn("Failed to deserialize incoming http request")
		return
	}

	logger.Infof("Build http Req [%+v]\n", req)

	client := http.Client{}
	resp, err := client.Do(req)
	var p *proto.Packet
	if err != nil {
		logger.Warningln("Failed to get a response from http req[%v]", err)
		args := proto.MakeHTTPErrprRespArgs(500)
		p = createProtoPackage(corrID, proto.HTTP_RESULT, args)
	} else {
		logger.Infof("Recv http resp[%v]\n", resp)
		p = sanitizeResponse(ctx, resp, corrID)
		logger.Infof("Sanitize http resp[%v]\n", p)
	}
	gw.send(ctx, p)
}

// sanitizeResponse removes unnecessary data from headers and parses response into a Packet.
func sanitizeResponse(ctx context.Context, resp *http.Response, corrID string) *proto.Packet {
	logger := log.Ctx(ctx)
	for _, header := range []string{"Content-Encoding", "Content-Length"} {
		if resp.Header.Get(header) != "" {
			resp.Header.Del(header)
		}
	}

	args, err := proto.MakeHTTPRespArgs(ctx, resp)
	if err != nil {
		logger.Warnf("Failed to create http resp args [%v]\n", err)
		args = proto.MakeHTTPErrprRespArgs(400)
	}
	return createProtoPackage(corrID, proto.HTTP_RESULT, args)
}

// createProtoPackage creates a package struct which is sent over websockets to bridge.
func createProtoPackage(corrID string, method proto.PacketMethod, args *proto.Args) *proto.Packet {
	var p proto.Packet
	p.CorrID = corrID
	p.Method = method
	p.Args = args
	return &p
}

// deserializeRequest converts Args to a http request.
func deserializeRequest(ctx context.Context, args *proto.Args) (*http.Request, error) {
	logger := log.Ctx(ctx)
	url, err := args.UrlTransform()
	if err != nil {
		logger.Warn("Failed to transform url to it's intended destination")
		return nil, err
	}

	req, err := http.NewRequest(args.Method, url, bytes.NewReader(common.IntSliceToByteSlice(args.Body)))
	if err != nil {
		logger.Warn("Failed to parse args into a http request obj")
		return req, err
	}

	for k, v := range args.Headers {
		req.Header.Add(k, v)
	}
	return req, nil
}
