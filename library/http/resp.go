package http

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type responseSniffer struct {
	w http.ResponseWriter

	code int
	n    int64

	sn   int
	data []byte
}

var snifferPool = sync.Pool{
	New: func() interface{} {
		return &responseSniffer{data: make([]byte, 0, 4096)}
	},
}

func (w *responseSniffer) Reset(rw http.ResponseWriter) {
	w.w = rw
	w.code = -1
	w.n = 0
	w.sn = 0
	w.data = w.data[:0]
}

func (w *responseSniffer) SetSnifferBytes(n int) {
	w.sn = n
}

func (w *responseSniffer) Header() http.Header {
	return w.w.Header()
}

func (w *responseSniffer) WriteHeader(code int) {
	if w.code < 0 {
		w.code = code
	}
	w.w.WriteHeader(code)
}

func (w *responseSniffer) Write(b []byte) (int, error) {
	if w.code < 0 {
		w.code = 200
	}

	n, err := w.w.Write(b)
	w.n += int64(n)

	if w.sn > 0 && len(w.data) < w.sn { // if set sniffer && not full
		if len(w.data)+len(b) > w.sn {
			b = b[:w.sn-len(w.data)] // limit to w.sn bytes
		}
		w.data = append(w.data, b...)
	}

	return n, err
}

func (w *responseSniffer) Bytes() []byte {
	return w.data
}

func (w *responseSniffer) Code() int {
	return w.code
}

func (w *responseSniffer) TotalResponseBytes() int64 {
	return w.n
}

func formatRequestLog(w http.ResponseWriter, r *http.Request) string {
	tmp := make([]byte, 0, 16)
	code, respn := -1, int64(-1)
	sniffer, ok := w.(*responseSniffer)
	if ok {
		code = sniffer.Code()
		respn = sniffer.TotalResponseBytes()
	}
	buf := &strings.Builder{}
	buf.WriteString(r.Method)
	buf.WriteString(" ")
	buf.WriteString(r.URL.Path)
	if len(r.URL.RawQuery) > 0 {
		buf.WriteString("?")
		buf.WriteString(r.URL.RawQuery)
	}
	buf.WriteString(" ")
	buf.Write(strconv.AppendInt(tmp[:0], int64(code), 10))
	buf.WriteString(" ")
	buf.Write(strconv.AppendInt(tmp[:0], int64(respn), 10))
	return buf.String()
}
