package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"

	http2 "github.com/bcmmacro/bridging-go/library/http"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{})

	router := httprouter.New()
	proc := NewHandler()
	router.GET("/*path", proc.HandleGet)
	handler := http2.CreateCORS(router, "*")

	// TODO(zzl) make port configurable
	port := ":8000"
	logrus.Infof("listenging on port %s", port)
	logrus.Fatal(http.ListenAndServe(port, handler))
}
