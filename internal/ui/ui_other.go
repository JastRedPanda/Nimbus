//go:build !windows

package ui

import (
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/webui"
)

func ShowSettings(cfg *config.Config, onFontChange func(int)) *config.Config {
	return webui.ShowSettings(cfg)
}

func ShowForecast(lat, lon float64, units, lang, theme, windUnit string) {
	webui.ShowForecast(lat, lon, units, lang, windUnit)
}

func ShowAbout(theme string) {
	webui.ShowAbout()
}
