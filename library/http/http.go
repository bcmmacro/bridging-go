package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	errors2 "github.com/bcmmacro/bridging-go/library/errors"
)

// The Response interface it implemented by user
//	who want to response a customised response.
type Response interface {
	WriteResponse(w http.ResponseWriter, r *http.Request)
}

type rawJSONResponse struct {
	data interface{}
}

// RawJSONResponse allows the use of ordinary structs as Response
func RawJSONResponse(d interface{}) Response {
	return rawJSONResponse{d}
}

// WriteResponse implements Response
func (d rawJSONResponse) WriteResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d.data)
}

// File represents a file request or response object
type File struct {
	Name    string
	Content []byte
}

func RawFileResponse(name string, content []byte) Response {
	return File{
		Name:    name,
		Content: content,
	}
}

// WriteResponse implements Response
func (d File) WriteResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", http.DetectContentType(d.Content))
	w.Header().Set("Content-Disposition", "attachment; filename="+d.Name)
	_, _ = w.Write(d.Content)
}

func WriteErr(w http.ResponseWriter, r *http.Request, err error) {
	var resp Response
	if errors.As(err, &resp) {
		resp.WriteResponse(w, r)
	} else {
		// we use a client side err code for context.Canceled err
		if errors.Is(err, context.Canceled) {
			errors2.ErrContextCanceled.WriteResponse(w, r)
		} else {
			errors2.ErrInternal.WriteResponse(w, r)
		}
	}
}

func WriteData(w http.ResponseWriter, r *http.Request, d interface{}) {
	resp, ok := d.(Response)
	if ok {
		resp.WriteResponse(w, r)
	} else {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Code int         `json:"code"`
			Msg  string      `json:"msg"`
			Data interface{} `json:"data,omitempty"`
		}{
			Code: 200,
			Msg:  "",
			Data: d,
		})
	}
}
