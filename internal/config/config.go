package config

import (
	"encoding/json"
	"os"

	"github.com/bcmmacro/bridging-go/library/errors"
)

type Config struct {
	BridgeNetLoc string            `json:"bridge_netloc"`
	BridgeToken  string            `json:"bridge_token"`
	Whitelist    []WhitelistConfig `json:"whitelist"`
}

type WhitelistConfig struct {
	Netloc []string `json:"netloc"`
	Method []string `json:"method"`
	Scheme []string `json:"scheme"`
	Path   []string `json:"path"`
}

func Get(path string) *Config {
	data, err := os.ReadFile(path)
	errors.Check(err)

	return Deserialize(data)
}

func Deserialize(data []byte) *Config {
	var conf Config
	err := json.Unmarshal(data, &conf)
	errors.Check(err)

	return &conf
}
