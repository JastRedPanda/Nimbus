//go:build !windows

package ui

import (
	"os/exec"
	"runtime"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/forecast"
)

func ShowSettings(cfg *config.Config) *config.Config {
	path, err := config.ConfigPath()
	if err != nil {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", "-t", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
	return nil
}

func ShowForecast(lat, lon float64, units, lang, theme string) {
	_ = theme
	forecast.Show(lat, lon, units, lang)
}
