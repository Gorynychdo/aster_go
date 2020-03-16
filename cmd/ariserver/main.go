package main

import (
	"flag"
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

	server := ariserver.NewServer()

	if err := server.Start(configPath); err != nil {
		return
	}

	server.Serve()
}
