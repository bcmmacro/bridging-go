package proto

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/sirupsen/logrus"
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
	Body       []byte            `json:"body,omitempty"`
	Content    string            `json:"content,omitempty"` // TODO(zzl) remove either Body or Content
}

func (args *Args) String() string {
	s, _ := json.Marshal(args)
	return string(s)
}

type Packet struct {
	CorrID string `json:"corr_id"`
	Method string `json:"method"` // desired operation: http, http_result, open_websocket etc.
	Args   *Args  `json:"args"`
}

func (p *Packet) String() string {
	s, _ := json.Marshal(p)
	return string(s)
}

func Deserialize(data []byte) (*Packet, error) {
	buf := bytes.NewBuffer(data)
	r, err := gzip.NewReader(buf)
	if err != nil {
		logrus.Warnf("failed to decompress error[%v]", err)
		return nil, err
	}

	var b bytes.Buffer
	_, err = b.ReadFrom(r)
	if err != nil {
		logrus.Warnf("failed to read from decompressed buffer error[%v]", err)
		return nil, err
	}

	var p Packet
	err = json.Unmarshal(b.Bytes(), &p)
	if err != nil {
		logrus.Warnf("failed to unmarshal json error[%v]", err)
		return nil, err
	}
	return &p, nil
}

func (p *Packet) Serialize(level int) ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		logrus.Warnf("failed to marshal packet error[%v]", err)
		return nil, err
	}

	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		logrus.Warnf("failed to create gzip writer error[%v]", err)
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		logrus.Warnf("failed to compress error[%v]", err)
		return nil, err
	}
	if err = w.Flush(); err != nil {
		logrus.Warnf("failed to flush writer error[%v]", err)
		return nil, err
	}
	if err = w.Close(); err != nil {
		logrus.Warnf("failed to close writer error[%v]", err)
		return nil, err
	}
	return buf.Bytes(), nil
}

func MakeHTTPReqArgs(r *http.Request) (*Args, error) {
	var args Args
	args.Method = r.Method

	// TODO(zzl) should split URL
	args.URL = fmt.Sprintf("http://%s%s", r.Host, r.URL.String())
	args.Client = r.Host

	args.Headers = make(map[string]string)
	for k, v := range r.Header {
		for _, vv := range v {
			args.Headers[k] = vv
		}
	}

	if body, err := ioutil.ReadAll(r.Body); err != nil {
		return nil, err
	} else {
		args.Body = body
	}
	return &args, nil
}
