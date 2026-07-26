//go:build linux

package ui

import (
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/webui"
)

// The Linux backend is being moved to GTK one window at a time. ShowAbout
// (about_linux.go) and ShowForecast (forecast_linux.go) are already native;
// settings still opens the browser UI.

func ShowSettings(cfg *config.Config, onFontChange func(int)) *config.Config {
	return webui.ShowSettings(cfg)
}
