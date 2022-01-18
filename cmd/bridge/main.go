package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/cors"
	"github.com/sirupsen/logrus"

	_ "github.com/bcmmacro/bridging-go/library/log"
)

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Fatalf("failed to load .env error[%v]", err)
	}

	c := cors.New(cors.Options{
		OptionsPassthrough: false,
		AllowCredentials:   true,
		AllowedOrigins:     strings.Split(os.Getenv("BRIDGE_CORS_ALLOW_ORIGINS"), ","),
		AllowedMethods:     strings.Split(os.Getenv("BRIDGE_CORS_ALLOW_METHODS"), ","),
		AllowedHeaders:     strings.Split(os.Getenv("BRIDGE_CORS_ALLOW_HEADERS"), ","),
	})
	handler := NewHandler(c)

	port := ":8000"
	if portEnv := os.Getenv("PORT"); portEnv != "" {
		port = fmt.Sprintf(":%s", os.Getenv("PORT"))
	}
	logrus.Infof("listenging on port %s", port)
	logrus.Fatal(http.ListenAndServe(port, c.Handler(handler)))
}
