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
	"github.com/sirupsen/logrus"
	"golang.org/x/net/websocket"
)

type Gateway struct {
	bridge *websocket.Conn
	wss    map[int64]*websocket.Conn
}

func (gw *Gateway) Run(conf *config.Config) {
	bridge_netloc := conf.BridgeNetLoc
	bridge_token := conf.BridgeToken
	gw.connect(bridge_netloc, bridge_token)
}

// connect makes a persistant connection to bridge's websocket, to allow data to flow between private DC and outbound server.
func (gw *Gateway) connect(bridge_netloc string, bridge_token string) {
	bridgeURL, err := url.Parse(bridge_netloc + "/bridge")
	errors.Check(err)
	origin, _ := url.Parse("http://localhost/")
	wsConf := websocket.Config{
		Location: bridgeURL,
		Origin:   origin,
		Version:  websocket.ProtocolVersionHybi13,
		Header:   http.Header{"bridging-token": []string{bridge_token}},
	}
	wss, err := websocket.DialConfig(&wsConf)
	errors.Check(err)

	logrus.Info("Connected to bridge")
	gw.bridge = wss

	// for {
	buf := make([]byte, 32*1024*1024)
	size, err := wss.Read(buf)
	errors.Check(err)

	// TODO: Read from header instead
	ctx, logger := log.WithField(context.Background(), "ReqID", common.RandomInt64())
	msg, err := proto.Deserialize(ctx, buf[:size])
	errors.Check(err)

	logger.Printf("Recv bridge msg[%+v]\n", msg)

	method := msg.Method
	corrId := msg.CorrID
	args := msg.Args

	if method == "http" {
		handleHttp(ctx, corrId, args)
	}
	// }
}

// handleHttp handles incoming http requests by forwarding them to the appropriate services.
func handleHttp(ctx context.Context, corrId string, args *proto.Args) {
	logger := log.Ctx(ctx)
	req := deserializeRequest(args)
	logger.Printf("req [%+v]\n", req)

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Warningln("Failed to get a responses from http req[%v]", err)
		return
	}
	logger.Printf("Recv http resp[%v]\n", resp)

	p := sanitizeResponse(resp)
	logger.Printf("Sanitize http resp[%v]\n", p)
}

func sanitizeResponse(resp *http.Response) proto.Packet {
	// resp.Header
	return proto.Packet{}
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

func (gw *Gateway) send(ctx context.Context, p proto.Packet) {
	logger := log.Ctx(ctx)
	logger.Info("Send bridge msg[%s]", p)
	msg, err := p.Serialize(ctx, gzip.DefaultCompression)
	errors.Check(err)

	_, err = gw.bridge.Write(msg)
	errors.Check(err)
}
