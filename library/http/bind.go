package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"

	errors2 "github.com/bcmmacro/bridging-go/library/errors"
	reflect2 "github.com/bcmmacro/bridging-go/library/reflect"
	log "github.com/sirupsen/logrus"
)

// MaxProcessTimeout indicates the max duration of process,
// bind.ProcessTimeout should never exceed the value, as well as http.Server
const MaxProcessTimeout = 60 * time.Second

type Auth func(ctx context.Context, r *http.Request) context.Context

var defaultAuth Auth

func SetDefaultAuth(auth Auth) {
	defaultAuth = auth
}

type mappingField struct {
	Name     string
	Required bool
}

// Bind implements binding between http request and handler.
// the handler can be defined as:
//		`func(ctx context.Context, r YourRequestStruct) (YourResponseStruct, error)`
// or	`func(ctx context.Context, r YourRequestStruct) error`
//
// it supports tree types of bindings via field tags
// * `json`, it unmarhals application/json body to request, and respects encoding/json pkg
// * `uri`, it converts param from httprouter.Params, see: https://github.com/julienschmidt/httprouter#named-parameters
// * `query`, it converts param from querystr
//
// for example:
//
// type UpdateRequestStruct struct {
// 	User int64  `uri:"user"`
// 	Op   string `query:"op"`
// 	Name string `json:"name"`
// }
//
// for http request `POST /user/123?op=1` with body `{"name":"Tom"}`,
// it can be converted to `UpdateRequestStruct{User: 123, Op:"1", Name:"Tom"}` with the help of router path `/user/:user` by using httprouter
// by default, `uri` fields are required, while `query` fields are optional,
// you can overwrite the behaviour by adding `optional` or `required` tag option, like:
//		User int64  `uri:"user,optional"`
//		Op   string `query:"op,required"`
// Bind will returns err if the field is required but get nothing from inputs
//
// for `YourResponseStruct`, it simply return the struct as json, you can also implement `httputil.Response` for customized output.
type Bind struct {
	name string

	newFunc reflect.Value

	fn reflect.Value
	in reflect.Type

	emptyReturn bool

	queryMapping     map[string]mappingField
	pathParamMapping map[string]mappingField

	processTimeout time.Duration

	auth Auth
}

func (b *Bind) clone() *Bind {
	ret := *b // ok by now
	return &ret
}

const defaultProcessTimeout = 180 * time.Second

// ProcessTimeout sets handle time that considered to be slow.
// Bind will log the request if exceeded the time.
// default: 10 seconds.
// max: 60 seconds.
func (b *Bind) ProcessTimeout(t time.Duration) *Bind {
	if t > MaxProcessTimeout {
		panic(fmt.Sprintf("ProcessTimeout max: %s", MaxProcessTimeout))
	}
	b = b.clone()
	b.processTimeout = t
	return b
}

// NewBind creates a bind with name for metrics and callback for http
func NewBind(name string, callback interface{}) *Bind {
	v := reflect.ValueOf(callback)
	if v.Kind() != reflect.Func {
		panic("not a func")
	}

	bind := &Bind{name: name, fn: v, auth: defaultAuth, processTimeout: defaultProcessTimeout}

	// check args
	t := v.Type()
	if t.NumIn() != 2 {
		panic("args# != 2")
	}
	tpContext := reflect.TypeOf((*context.Context)(nil)).Elem()
	if !t.In(0).Implements(tpContext) {
		panic("args[0] not context.Context")
	}
	a := t.In(1)
	if a.Kind() != reflect.Ptr {
		panic("args[1] not struct pointer")
	}
	a = a.Elem()
	if a.Kind() != reflect.Struct {
		panic("args[1] not struct pointer")
	}
	bind.in = a

	// check returns
	tpError := reflect.TypeOf((*error)(nil)).Elem()
	if t.NumOut() == 1 {
		if !t.Out(0).Implements(tpError) {
			panic("returns[0] not error type")
		}
		bind.emptyReturn = true
	} else if t.NumOut() == 2 {
		if !t.Out(1).Implements(tpError) {
			panic("returns[1] not error type")
		}
	} else {
		panic("return# != 1|2")
	}

	bind.queryMapping = getQueryMapping(a, 3)
	bind.pathParamMapping = getPathParamMapping(a)

	return bind
}

func getQueryMapping(t reflect.Type, level int) map[string]mappingField {
	if level == 0 { // avoid stack overflow caused by recursive type
		return nil
	}
	mapping := make(map[string]mappingField)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		ft := f.Type
		if s := f.Tag.Get("query"); s != "" {
			tag, opts := reflect2.ParseTag(s)
			if tag == "-" {
				continue
			}
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for k, v := range getQueryMapping(ft, level-1) {
					mapping[tag+"."+k] = mappingField{Name: f.Name + "." + v.Name, Required: v.Required}
				}
			} else {
				required := opts.Contains("required") // by default, query is optional
				mapping[tag] = mappingField{Name: f.Name, Required: required}
			}
		}
	}
	return mapping
}

func getPathParamMapping(a reflect.Type) map[string]mappingField {
	mapping := make(map[string]mappingField)
	for i := 0; i < a.NumField(); i++ {
		f := a.Field(i)
		if s := f.Tag.Get("uri"); s != "" {
			tag, opts := reflect2.ParseTag(s)
			if tag == "-" {
				continue
			}
			optional := opts.Contains("optional") // by default, path param is required
			mapping[s] = mappingField{Name: f.Name, Required: !optional}
		}
	}
	return mapping
}

func (x *Bind) newIn() reflect.Value {
	if f := x.newFunc; f.IsValid() {
		return f.Call(nil)[0]
	}
	return reflect.New(x.in)
}

func (x *Bind) init(r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return errors2.ErrBadRequest.WithMsg("parse form err")
	}
	return nil
}

func (x *Bind) ParseBody(r *http.Request, rv *reflect2.ReflectValue) error {
	if r.Method == "GET" || r.Method == "HEAD" {
		return nil // these methods should not have body
	}

	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(rv.GetInterface()); err != nil {
			return errors2.ErrBadRequest.WithMsg("json decode err")
		}
	} else if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if !rv.HasField("Files") {
			return nil
		}
		ff := []*File{}
		r.ParseMultipartForm(16 << 24)
		f := r.MultipartForm
		if f != nil && f.File != nil {
			for k, vv := range f.File {
				if len(vv) == 0 {
					continue
				}
				f, err := vv[0].Open()
				if err != nil {
					continue
				}
				buf := &bytes.Buffer{}
				_, err = io.Copy(buf, f)
				f.Close()
				if err != nil {
					continue
				}
				ff = append(ff, &File{
					Name:    k,
					Content: buf.Bytes(),
				})
			}
			rv.SetField("Files", ff)
		}
	}
	return nil
}

// fixes httprouter.Params.ByName can't tell if it's set
func getPathParams(params httprouter.Params, name string) (string, bool) {
	for _, p := range params {
		if p.Key == name {
			return p.Value, true
		}
	}
	return "", false
}

func (x *Bind) serveHTTP(r *http.Request) (interface{}, error) {
	if err := x.init(r); err != nil {
		return nil, err
	}
	in := x.newIn()
	rv := reflect2.NewReflectValue(in)

	// try fill the args with http query
	for q, f := range x.queryMapping {
		vv, ok := r.Form[q]
		if !ok && f.Required {
			return nil, errors2.ErrBadRequest
		}
		if len(vv) == 0 {
			continue
		}
		if err := rv.SetFieldByStr(f.Name, vv...); err != nil {
			return nil, errors2.ErrBadRequest.WithErr(err)
		}
	}

	ctx := r.Context()

	// try fill args with path params i.e http://host/:ticket_id/tags
	params := httprouter.ParamsFromContext(r.Context())
	for paramName, f := range x.pathParamMapping {
		paramValue, ok := getPathParams(params, paramName)
		if !ok {
			if f.Required {
				// it's considered missing param name in parh definition
				log.Errorf("Bind(%s): path missing param %q", x.name, paramName)
				return nil, errors2.ErrBadRequest
			}
			continue
		}
		if err := rv.SetFieldByStr(f.Name, paramValue); err != nil {
			return nil, errors2.ErrBadRequest.WithErr(err)
		}
	}

	if err := x.ParseBody(r, rv); err != nil {
		return nil, err
	}

	// security fields, MUST not set by client
	_ = rv.UnsetField("Base")
	_ = rv.UnsetField("Head")

	vv := x.fn.Call([]reflect.Value{reflect.ValueOf(ctx), in})
	if x.emptyReturn {
		err, _ := vv[0].Interface().(error)
		return struct{}{}, err
	}
	err, _ := vv[1].Interface().(error)
	return vv[0].Interface(), err
}

type httpServeResponse struct {
	data interface{}
	err  error
}

type codeMsg struct { // only use for getting code & msg
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (c codeMsg) Error() string {
	return fmt.Sprintf("code{%d:%s}", c.Code, c.Msg)
}

func (x *Bind) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t0 := time.Now()

	ctx := r.Context()
	timeoutCtx, cancelFunc := context.WithTimeout(ctx, x.processTimeout)
	defer cancelFunc()

	sniffer := snifferPool.Get().(*responseSniffer)
	defer snifferPool.Put(sniffer)
	sniffer.Reset(w)
	sniffer.SetSnifferBytes(4096)
	w = sniffer

	resp := make(chan httpServeResponse, 1)
	go func() {
		defer func() {
			err := recover()
			if err == nil {
				return
			}
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			log.Errorf("%s panic: %s\n\nstacktrace:\n%s", x.name, err, buf)
			resp <- httpServeResponse{
				err: errors2.ErrPanic,
			}
		}()
		data, err := x.serveHTTP(r.WithContext(x.auth(timeoutCtx, r)))
		resp <- httpServeResponse{
			data: data,
			err:  err,
		}
	}()

	var data interface{}
	var err error
	select {
	case result := <-resp:
		data = result.data
		err = result.err
	case <-timeoutCtx.Done():
		// may caused by disconnected connection, Go http pkg would cancel the ctx
		if err = ctx.Err(); err == nil {
			err = errors2.ErrServerTimeout
		}
	}

	cost := time.Since(t0)
	logf := log.Infof
	if err != nil {
		WriteErr(w, r, err)
		// for context.Canceled case, the connection is disconnected before x.processTimeout
		// so we consider it as a client side err
		if errors.Is(err, context.Canceled) || errors2.IsClientErr(err) {
			logf = log.Warnf
		} else {
			logf = log.Errorf
		}
	} else {
		WriteData(w, r, data)
	}

	m := &codeMsg{}
	_ = json.Unmarshal(sniffer.Bytes(), m)

	logf("Bind[%s] %s err[%v] cost[%s]", x.name, formatRequestLog(w, r), err, cost)
}
