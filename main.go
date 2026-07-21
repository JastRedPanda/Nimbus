package main

import (
	"log"

	"github.com/Lrt/Nimbus/internal/config"
	"github.com/Lrt/Nimbus/internal/tray"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	tray.Run(cfg)
}
