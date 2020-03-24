package main

import (
	"flag"
	"log"

	"github.com/BurntSushi/toml"
	"github.com/Gorynychdo/aster_go/internal/app/ariserver"
)

var (
	configPath string
)

func init() {
	flag.StringVar(&configPath, "config-path", "configs/ariserver.toml", "path to config file")
}

func main() {
	flag.Parse()

	config := ariserver.NewConfig()
	_, err := toml.DecodeFile(configPath, config)
	if err != nil {
		log.Fatal(err)
	}

	if err := ariserver.Start(config); err != nil {
		log.Fatal(err)
	}
}
