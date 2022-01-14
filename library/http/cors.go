package http

import "net/http"

type handler struct {
	allowedOrigin string
	inner         http.Handler
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", h.allowedOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET, POST, PUT, PATCH, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, x-auth-token")
	h.inner.ServeHTTP(w, r)
}

func CreateCORS(h http.Handler, allowedOrigin string) http.Handler {
	return &handler{allowedOrigin: allowedOrigin, inner: h}
}
