package proto

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/bcmmacro/bridging-go/library/common"
	"github.com/bcmmacro/bridging-go/library/log"
)

type Args struct {
	Method     string              `json:"method,omitempty"` // http method
	URL        string              `json:"url,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	Client     string              `json:"client,omitempty"`
	WSID       string              `json:"ws_id,omitempty"`
	Msg        string              `json:"msg,omitempty"`
	StatusCode int64               `json:"status_code,omitempty"`
	Exception  string              `json:"exception,omitempty"`
	Body       []int8              `json:"body,omitempty"`    // TODO(zzl) type []byte Bytes and add a Marshal() to Bytes
	Content    string              `json:"content,omitempty"` // TODO(zzl) remove either Body or Content
}

func (args *Args) String() string {
	s, _ := json.Marshal(args.truncated())
	return string(s)
}

// to avoid long unreadable log lines
func (args *Args) truncated() *Args {
	return &Args{Method: args.Method, URL: args.URL,
		Headers: args.Headers, Client: args.Client, WSID: args.WSID,
		Msg: common.CutStr(args.Msg, 1000), StatusCode: args.StatusCode, Exception: args.Exception,
		Body: common.CutInt8(args.Body, 1000), Content: common.CutStr(args.Content, 1000),
	}
}

// urlTransform replaces original url to bridging-base-url.
func (args *Args) UrlTransform() (string, error) {
	url, err := url.Parse(args.URL)
	if err != nil {
		return "", err
	}

	for k, v := range args.Headers {
		if strings.ToLower(k) == "bridging-base-url" && len(v) > 0 {
			url.Host = v[0]
		}
	}
	return url.String(), nil
}

// wsUrlTransform is the sibling function to urlTransform for websocket destination.
func (args *Args) WsUrlTransform() (*url.URL, error) {
	url, err := url.Parse(args.URL)
	if err != nil {
		return nil, err
	}

	for k, v := range url.Query() {
		if k == "bridging-base-url" {
			url.Host = v[0]
			u := url.Query()
			u.Del("bridging-base-url")
			url.RawQuery = u.Encode()
		}
	}
	return url, nil
}

type PacketMethod string

const (
	OPEN_WEBSOCKET_RESULT  PacketMethod = "open_websocket_result"
	OPEN_WEBSOCKET         PacketMethod = "open_websocket"
	CLOSE_WEBSOCKET_RESULT PacketMethod = "close_websocket_result"
	CLOSE_WEBSOCKET        PacketMethod = "close_websocket"
	WEBSOCKET_MSG          PacketMethod = "websocket_msg"
	HTTP_RESULT            PacketMethod = "http_result"
	HTTP                   PacketMethod = "http"
)

type Packet struct {
	CorrID string       `json:"corr_id"`
	Method PacketMethod `json:"method"`
	Args   *Args        `json:"args"`
}

func (p *Packet) String() string {
	s, _ := json.Marshal(&Packet{CorrID: p.CorrID, Method: p.Method, Args: p.Args.truncated()})
	return string(s)
}

func Deserialize(ctx context.Context, data []byte) (*Packet, error) {
	logger := log.Ctx(ctx)
	buf := bytes.NewBuffer(data)
	r, err := gzip.NewReader(buf)
	defer func() {
		if err := r.Close(); err != nil {
			logger.Warnf("failed to close gzip reader error[%v]", err)
		}
	}()

	if err != nil {
		logger.Warnf("failed to decompress error[%v]", err)
		return nil, err
	}

	var b bytes.Buffer
	_, err = b.ReadFrom(r)
	if err != nil {
		logger.Warnf("failed to read from decompressed buffer error[%v]", err)
		return nil, err
	}

	var p Packet
	err = json.Unmarshal(b.Bytes(), &p)
	if err != nil {
		logger.Warnf("failed to unmarshal json error[%v]", err)
		return nil, err
	}
	return &p, nil
}

func (p *Packet) SerializeJSON(ctx context.Context) ([]byte, error) {
	logger := log.Ctx(ctx)
	data, err := json.Marshal(p)
	if err != nil {
		logger.Warnf("failed to marshal packet error[%v]", err)
		return nil, err
	}
	return data, nil
}

// Serialize converts the packet into byte array by json marshalling, and gzipped if compressor is not nil.
// caller can reuse the same compressor to reduce cpu usage.
func (p *Packet) Serialize(ctx context.Context, compressor *gzip.Writer) ([]byte, error) {
	logger := log.Ctx(ctx)
	data, err := p.SerializeJSON(ctx)
	if err != nil {
		return nil, err
	}

	if compressor == nil {
		return data, nil
	}

	var buf bytes.Buffer
	compressor.Reset(&buf)
	_, err = compressor.Write(data)
	if err != nil {
		logger.Warnf("failed to compress error[%v]", err)
		return nil, err
	}
	if err = compressor.Flush(); err != nil {
		logger.Warnf("failed to flush gzip writer error[%v]", err)
		return nil, err
	}
	if err = compressor.Close(); err != nil {
		logger.Warnf("failed to close gzip writer error[%v]", err)
		return nil, err
	}
	return buf.Bytes(), nil
}

func MakeHTTPReqArgs(ctx context.Context, r *http.Request) (*Args, error) {
	logger := log.Ctx(ctx)

	var args Args
	args.Method = r.Method

	// TODO(zzl) should split URL
	scheme := "http"
	if r.Header.Get("Upgrade") == "websocket" {
		scheme = "ws"
	}
	args.URL = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.String())
	args.Client = r.RemoteAddr

	args.Headers = make(map[string][]string)
	for k, v := range r.Header {
		args.Headers[k] = append([]string(nil), v...)
	}

	if body, err := ioutil.ReadAll(r.Body); err != nil {
		logger.Warnf("failed to read body error[%v]", err)
		return nil, err
	} else {
		args.Body = common.ByteSliceToIntSlice(body)
	}
	return &args, nil
}

func MakeHTTPRespArgs(ctx context.Context, r *http.Response) (*Args, error) {
	logger := log.Ctx(ctx)
	var args Args

	args.Headers = make(map[string][]string)
	for k, v := range r.Header {
		args.Headers[k] = append([]string(nil), v...)
	}

	args.StatusCode = int64(r.StatusCode)

	if body, err := ioutil.ReadAll(r.Body); err != nil {
		logger.Warnf("failed to read body error[%v]", err)
		return nil, err
	} else {
		args.Content = string(body)
	}
	return &args, nil
}

func MakeHTTPErrprRespArgs(statusCode int) *Args {
	var args Args
	args.Headers = nil
	args.StatusCode = int64(statusCode)
	args.Content = ""
	return &args
}
