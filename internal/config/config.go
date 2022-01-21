package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"

	errs "github.com/bcmmacro/bridging-go/library/errors"
	"github.com/bcmmacro/bridging-go/library/log"
	"github.com/sirupsen/logrus"
)

type Config struct {
	BridgeNetLoc string
	BridgeToken  string
	WhitelistMap WhitelistMap
}

type config struct {
	BridgeNetLoc string            `json:"bridge_netloc"`
	BridgeToken  string            `json:"bridge_token"`
	Whitelist    []WhitelistConfig `json:"whitelist"`
}

type WhitelistMap map[WhitelistEntry]bool

type WhitelistConfig struct {
	Netloc []string `json:"netloc"`
	Method []string `json:"method"`
	Scheme []string `json:"scheme"`
	Path   []string `json:"path"`
}

type WhitelistEntry struct {
	Netloc string
	Method string
	Scheme string
	Path   string
}

func Get(path string) *Config {
	data, err := os.ReadFile(path)
	errs.Check(err)

	return Deserialize(data)
}

func (conf *WhitelistMap) Check(ctx context.Context, method string, url *url.URL) error {
	logger := log.Ctx(ctx)
	wlEntry := WhitelistEntry{
		Netloc: url.Host,
		Method: strings.ToUpper(method),
		Scheme: url.Scheme,
		Path:   url.Path,
	}
	_, present := (*conf)[wlEntry]
	if !present {
		logger.Warnf("forbidden [%v]\n", wlEntry)
		return errors.New("forbidden")
	}
	return nil
}

func Deserialize(data []byte) *Config {
	var conf config
	err := json.Unmarshal(data, &conf)
	errs.Check(err)

	// Each whitelist route should be a separate entry in a hashmap for faster lookup
	whitelistMap := map[WhitelistEntry]bool{}
	for _, entry := range conf.Whitelist {
		for _, netloc := range entry.Netloc {
			for _, method := range entry.Method {
				for _, scheme := range entry.Scheme {
					for _, path := range entry.Path {
						wle := WhitelistEntry{
							Netloc: netloc,
							Method: strings.ToUpper(method),
							Scheme: scheme,
							Path:   path,
						}
						whitelistMap[wle] = true
					}
				}
			}
		}
	}
	logrus.Infof("Constructed whitelist for downstream routes [%v]", whitelistMap)
	confMap := Config{
		BridgeNetLoc: conf.BridgeNetLoc,
		BridgeToken:  conf.BridgeToken,
		WhitelistMap: whitelistMap,
	}
	return &confMap
}
