package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"

	http2 "github.com/bcmmacro/bridging-go/library/http"
)

func main() {
	router := httprouter.New()
	proc := NewHandler()
	setUpRoute(router, proc)
	handler := http2.CreateCORS(router, "")

	port := ":5000"
	logrus.Infof("listenging on port %s", port)
	logrus.Fatal(http.ListenAndServe(port, handler))
}
