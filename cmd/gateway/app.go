package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"github.com/bcmmacro/bridging-go/internal/config"
	"github.com/bcmmacro/bridging-go/internal/proto"
	"github.com/bcmmacro/bridging-go/library/common"
	"github.com/bcmmacro/bridging-go/library/errors"
	"github.com/bcmmacro/bridging-go/library/log"
)

type Gateway struct {
	bridge *websocket.Conn
	wss    map[string]*websocket.Conn
}

// Run starts the gateway and accept incoming requests via websockets.
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
		} else if method == "open_websocket" {
			go gw.handleOpenWebsocket(ctx, corrID, args)
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

// handleOpenWebsocket opens a single websocket connection per request with downstream services.
func (gw *Gateway) handleOpenWebsocket(ctx context.Context, corrID string, args *proto.Args) {
	logger := log.Ctx(ctx)
	wsid := args.WSID
	logger.Println(logger, wsid)
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
	var p proto.Packet
	if err != nil {
		logger.Warningln("Failed to get a response from http req[%v]", err)
		args := proto.MakeHTTPErrprRespArgs(500)
		p = createProtoPackage(corrID, "http_result", args)
	} else {
		logger.Infof("Recv http resp[%v]\n", resp)
		p = sanitizeResponse(ctx, resp, corrID)
		logger.Infof("Sanitize http resp[%v]\n", p)
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
func deserializeRequest(ctx context.Context, args *proto.Args) (*http.Request, error) {
	logger := log.Ctx(ctx)
	url, err := urlTransform(args)
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

// urlTransform replaces original url to bridging-base-url.
func urlTransform(args *proto.Args) (string, error) {
	url, err := url.Parse(args.URL)
	if err != nil {
		return "", err
	}

	for k, v := range args.Headers {
		if strings.ToLower(k) == "bridging-base-url" {
			url.Host = v
		}
	}
	return url.String(), nil
}

// wsUrlTransform is the sibling function to urlTransform for websocket destination
func wslUrlTransform(args *proto.Args) (string, error) {
	return "", nil
}
