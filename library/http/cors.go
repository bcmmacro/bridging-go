package http

import "net/http"

type handler struct {
	allowedOrigin  string
	allowedMethods string
	allowedHeaders string
	inner          http.Handler
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", h.allowedOrigin)
	w.Header().Set("Access-Control-Allow-Methods", h.allowedMethods)
	w.Header().Set("Access-Control-Allow-Headers", h.allowedHeaders)
	h.inner.ServeHTTP(w, r)
}

func CreateCORS(h http.Handler, allowedOrigin, allowedMethods, allowedHeaders string) http.Handler {
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}
	if allowedMethods == "" {
		allowedMethods = "*"
	}
	if allowedHeaders == "" {
		allowedHeaders = "*"
	}
	return &handler{allowedOrigin: allowedOrigin, allowedMethods: allowedMethods, allowedHeaders: allowedHeaders, inner: h}
}
