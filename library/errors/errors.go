package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// placeholder for 200, can be used for message
var OK = CodeMsgData{code: 2000, httpStatusCode: 200}

var ( // 5xxx internal err
	ErrInternal        = CodeMsgData{code: 5000, httpStatusCode: 500}
	ErrForward2Backend = CodeMsgData{code: 5002, httpStatusCode: 502}
	ErrPanic           = CodeMsgData{code: 5100, httpStatusCode: 500}
	ErrAuth            = CodeMsgData{code: 5101, httpStatusCode: 500}
	ErrBackendService  = CodeMsgData{code: 5102, httpStatusCode: 502}
)

var ( // 4xxx client err
	ErrBadRequest       = CodeMsgData{code: 4000, httpStatusCode: 400}
	ErrUnauthorized     = CodeMsgData{code: 4001, httpStatusCode: 401}
	ErrForbidden        = CodeMsgData{code: 4003, httpStatusCode: 403}
	ErrNotFound         = CodeMsgData{code: 4004, httpStatusCode: 404}
	ErrMethodForbidden  = CodeMsgData{code: 4005, httpStatusCode: 405}
	ErrConflict         = CodeMsgData{code: 4006, httpStatusCode: 409}
	ErrServerTimeout    = CodeMsgData{code: 4008, httpStatusCode: 408}
	ErrHelpdeskDisabled = CodeMsgData{code: 4009, httpStatusCode: 403}
	ErrContextCanceled  = CodeMsgData{code: 4099, httpStatusCode: 499}
)

const (
	clientErrCodeMin = 4000
	clientErrCodeMax = 4999
)

func Check(e error) {
	if e != nil {
		panic(e)
	}
}

// IsClientErrCode returns true if the code is 4xxx
func IsClientErrCode(code int) bool {
	return code >= clientErrCodeMin && code <= clientErrCodeMax
}

// IsClientErr returns true if the err code is 4xxx
func IsClientErr(err error) bool {
	var e CodeMsgData
	if errors.As(err, &e) && IsClientErrCode(e.code) {
		return true
	}
	return false
}

type CodeMsgData struct {
	httpStatusCode int

	code int
	msg  string
	data interface{}
}

// GetCode returns the code field
func (d CodeMsgData) GetCode() int {
	return d.code
}

// WithCode returns new CodeMsgData with the specified code
func (d CodeMsgData) WithCode(c int) CodeMsgData {
	d.code = c
	return d
}

// GetMsg returns the msg field
func (d CodeMsgData) GetMsg() string {
	return d.msg
}

func (d CodeMsgData) WithMsg(f string, a ...interface{}) CodeMsgData {
	d.msg = fmt.Sprintf(f, a...)
	return d
}

// WithData returns new CodeMsgData with the specified data
func (d CodeMsgData) WithData(data interface{}) CodeMsgData {
	d.data = data
	return d
}

// WithErr returns new CodeMsgData with the err.
// if err is CodeMsgData then return the value,
// or it will set CodeMsgData.err.
func (d CodeMsgData) WithErr(err error) CodeMsgData {
	if errors.As(err, &d) {
		return d
	}
	d.msg = err.Error()
	return d
}

// StatusCode returns new CodeMsgData with the specified http status code
func (d CodeMsgData) StatusCode(c int) CodeMsgData {
	d.httpStatusCode = c
	return d
}

// WriteResponse implements httputil.Response
func (d CodeMsgData) WriteResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if d.httpStatusCode != 0 {
		if d.httpStatusCode >= 100 && d.httpStatusCode <= 999 {
			w.WriteHeader(d.httpStatusCode)
		}
		if d.code == 0 { // use httpStatusCode if code not set
			d.code = d.httpStatusCode
		}
	}

	_ = json.NewEncoder(w).Encode(struct {
		Code int         `json:"code"`
		Msg  string      `json:"msg"`
		Data interface{} `json:"data,omitempty"`
	}{
		Code: d.code,
		Msg:  d.msg,
		Data: d.data,
	})
}

// Error implements error
func (d CodeMsgData) Error() string {
	return fmt.Sprintf("CodeMsgData{code=%d msg=%q}", d.code, d.msg)
}
