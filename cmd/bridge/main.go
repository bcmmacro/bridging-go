package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"

	http2 "github.com/bcmmacro/bridging-go/library/http"
)

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{})

	if err := godotenv.Load(); err != nil {
		log.Fatalf("failed to load .env error[%v]", err)
	}

	proc := NewHandler()
	handler := http2.CreateCORS(proc, os.Getenv("BRIDGE_CORS_ALLOW_ORIGINS"), os.Getenv("BRIDGE_CORS_ALLOW_METHODS"), os.Getenv("BRIDGE_CORS_ALLOW_HEADERS"))

	port := ":8000"
	if portEnv := os.Getenv("PORT"); portEnv != "" {
		port = fmt.Sprintf(":%s", os.Getenv("PORT"))
	}
	logrus.Infof("listenging on port %s", port)
	logrus.Fatal(http.ListenAndServe(port, handler))
}
