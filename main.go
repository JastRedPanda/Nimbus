package main

import (
	"log"

	"github.com/JastRedPanda/Nimbus/internal/build"
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/logfile"
	"github.com/JastRedPanda/Nimbus/internal/tray"
)

func main() {
	// Before anything that can fail: a GUI build has no console, so without
	// this every diagnostic in the program is written to a closed handle.
	logfile.Open()
	log.Printf("Nimbus %s starting", build.Version)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	tray.Run(cfg)
}
