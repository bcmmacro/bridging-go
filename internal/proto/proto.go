package proto

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/bcmmacro/bridging-go/library/common"
	"github.com/bcmmacro/bridging-go/library/log"
)

type Args struct {
	Method     string            `json:"method,omitempty"` // http method
	URL        string            `json:"url,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"` // TODO(zzl) change to map[string][]string
	Client     string            `json:"client,omitempty"`
	WSID       string            `json:"ws_id,omitempty"`
	Msg        string            `json:"msg,omitempty"`
	StatusCode int64             `json:"status_code,omitempty"`
	Exception  string            `json:"exception,omitempty"`
	Body       []int64           `json:"body,omitempty"`
	Content    string            `json:"content,omitempty"` // TODO(zzl) remove either Body or Content
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
		Body: common.CutInt(args.Body, 1000), Content: common.CutStr(args.Content, 1000),
	}
}

type Packet struct {
	CorrID string `json:"corr_id"`
	Method string `json:"method"` // desired operation: http, http_result, open_websocket etc.
	Args   *Args  `json:"args"`
}

func (p *Packet) String() string {
	s, _ := json.Marshal(&Packet{CorrID: p.CorrID, Method: p.Method, Args: p.Args.truncated()})
	return string(s)
}

func Deserialize(ctx context.Context, data []byte) (*Packet, error) {
	logger := log.Ctx(ctx)
	buf := bytes.NewBuffer(data)
	r, err := gzip.NewReader(buf)
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

func (p *Packet) Serialize(ctx context.Context, level int) ([]byte, error) {
	logger := log.Ctx(ctx)
	data, err := json.Marshal(p)
	if err != nil {
		logger.Warnf("failed to marshal packet error[%v]", err)
		return nil, err
	}

	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		logger.Warnf("failed to create gzip writer error[%v]", err)
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		logger.Warnf("failed to compress error[%v]", err)
		return nil, err
	}
	if err = w.Flush(); err != nil {
		logger.Warnf("failed to flush writer error[%v]", err)
		return nil, err
	}
	if err = w.Close(); err != nil {
		logger.Warnf("failed to close writer error[%v]", err)
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

	args.Headers = make(map[string]string)
	for k, v := range r.Header {
		for _, vv := range v {
			args.Headers[k] = vv
		}
	}

	if body, err := ioutil.ReadAll(r.Body); err != nil {
		logger.Warnf("failed to read body error[%v]", err)
		return nil, err
	} else {
		args.Body = common.ByteSliceToIntSlice(body)
	}
	return &args, nil
}
