package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

func setUpRoute(router *httprouter.Router) {
	router.GET("/*path", handleGet)
}


func handleGet(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	logrus.Infof("recv GET %s %s", r.RemoteAddr, r.URL.String())
}