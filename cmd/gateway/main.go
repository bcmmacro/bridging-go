// Gateway is a tool to forward http/websockets requests in a interval facing private Datacenter
// to the downstream services, before forwarding it back to public facing Bridge service.
//
// On initialization, it makes a connection to the Bridge service and allows free flow of data
// between the 2 services via websockets
package main

import (
	"os"

	"github.com/bcmmacro/bridging-go/internal/config"
	"github.com/sirupsen/logrus"
)

func main() {
	conf := argParse(os.Args)
	gw := NewGateway(&conf.WhitelistMap)
	gw.Run(conf)
}

func argParse(args []string) *config.Config {
	if len(args) != 2 {
		logrus.Error("Usage: ./gateway cfg_path")
		os.Exit(1)
	}
	return config.Get(args[1])
}
