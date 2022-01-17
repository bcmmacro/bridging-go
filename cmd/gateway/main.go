package main

import (
	"fmt"
	"os"

	"github.com/bcmmacro/bridging-go/internal/config"
)

func main() {
	conf := argParse(os.Args)
	gw := Gateway{}
	gw.Run(conf)
}

func argParse(args []string) *config.Config {
	if len(args) != 2 {
		fmt.Println("Usage: ./gateway cfg_path")
		os.Exit(1)
	}
	return config.Get(args[1])
}
