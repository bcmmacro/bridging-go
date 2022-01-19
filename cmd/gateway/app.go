package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/bcmmacro/bridging-go/internal/config"
	"github.com/bcmmacro/bridging-go/internal/proto"
	"github.com/bcmmacro/bridging-go/library/common"
	"github.com/bcmmacro/bridging-go/library/errors"
	"github.com/bcmmacro/bridging-go/library/log"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type Gateway struct {
	bridge *websocket.Conn
	// wss    map[int64]*websocket.Conn
}

// Run starts the gateway and accept incoming requests via websockets.
func (gw *Gateway) Run(conf *config.Config) {
	bridge_netloc := conf.BridgeNetLoc
	bridge_token := conf.BridgeToken
	gw.connect(bridge_netloc, bridge_token)
}

// connect makes a persistant connection to bridge's websocket, to allow data to flow between private DC and outbound server.
func (gw *Gateway) connect(bridge_netloc string, bridge_token string) {
	bridgeURL := bridge_netloc + "/bridge"
	wss, _, err := websocket.DefaultDialer.Dial(bridgeURL, http.Header{"bridging-token": []string{bridge_token}})
	if err != nil {
		logrus.Fatal("Dial: ", err)
	}
	defer wss.Close()

	logrus.Info("Connected to bridge")
	gw.bridge = wss

	for {
		_, wsMsg, err := wss.ReadMessage()
		ctx := context.Background()
		if err != nil {
			logrus.Println("Read: ", err)
			return
		}
		msg, _ := proto.Deserialize(ctx, wsMsg)
		ctx, logger := log.WithField(ctx, "ReqID", msg.CorrID)

		logger.Printf("Recv bridge msg: %+v\n", msg)

		method := msg.Method
		corrID := msg.CorrID
		args := msg.Args

		if method == "http" {
			go gw.handleHttp(ctx, corrID, args)
		}
	}
}

// send transmits a packet from gateway to bridge.
func (gw *Gateway) send(ctx context.Context, p proto.Packet) {
	logger := log.Ctx(ctx)
	logger.Printf("Send bridge msg[%+v]\n", p)
	msg, err := p.Serialize(ctx, gzip.DefaultCompression)
	errors.Check(err)

	err = gw.bridge.WriteMessage(websocket.BinaryMessage, msg)
	if err != nil {
		logger.Warnf("Failed to transmit packet to bridge [%v]", err)
	}
}

// handleHttp handles incoming http requests by forwarding them to the appropriate services.
func (gw *Gateway) handleHttp(ctx context.Context, corrID string, args *proto.Args) {
	logger := log.Ctx(ctx)
	req := deserializeRequest(args)
	logger.Printf("Build http Req [%+v]\n", req)

	client := http.Client{}
	resp, err := client.Do(req)
	var p proto.Packet
	if err != nil {
		logger.Warningln("Failed to get a response from http req[%v]", err)
		args := proto.MakeHTTPErrprRespArgs(500)
		p = createProtoPackage(corrID, "http_result", args)
	} else {
		logger.Printf("Recv http resp[%v]\n", resp)
		p = sanitizeResponse(ctx, resp, corrID)
		logger.Printf("Sanitize http resp[%v]\n", p)
	}
	gw.send(ctx, p)
}

// sanitizeResponse removes unnecessary data from headers and parses response into a Packet.
func sanitizeResponse(ctx context.Context, resp *http.Response, corrID string) proto.Packet {
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
	return createProtoPackage(corrID, "http_result", args)
}

// createProtoPackage creates a package struct which is sent over websockets to bridge.
func createProtoPackage(corrID string, method string, args *proto.Args) proto.Packet {
	var p proto.Packet
	p.CorrID = corrID
	p.Method = method
	p.Args = args
	return p
}

// deserializeRequest converts Args to a http request.
func deserializeRequest(args *proto.Args) *http.Request {
	url := urlTransform(args)
	req, err := http.NewRequest(args.Method, url, bytes.NewReader(common.IntSliceToByteSlice(args.Body)))
	errors.Check(err)

	for k, v := range args.Headers {
		req.Header.Add(k, v)
	}
	return req
}

// urlTransform replaces original url to bridging-base-url.
func urlTransform(args *proto.Args) string {
	url, err := url.Parse(args.URL)
	errors.Check(err)

	for k, v := range args.Headers {
		if strings.ToLower(k) == "bridging-base-url" {
			url.Host = v
		}
	}
	return url.String()
}
