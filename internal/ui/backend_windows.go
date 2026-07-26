//go:build windows

package ui

import (
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gui"
)

// win32Backend draws windows with the Win32 API directly.
type win32Backend struct{}

func (win32Backend) Name() string { return "win32" }

func (win32Backend) Settings(cfg *config.Config, onFontScale func(int)) *config.Config {
	return showSettings(cfg, onFontScale)
}

func (win32Backend) Forecast(req gui.Forecast) {
	showForecast(req.Lat, req.Lon, req.Units, req.Lang, req.Theme, req.WindUnit)
}

func (win32Backend) About(theme string) { showAbout(theme) }

func (win32Backend) Error(_, message string) { showError(message) }

func init() {
	gui.Register(gui.Factory{
		Name:  "win32",
		Rank:  100,
		Probe: func() bool { return true },
		Open:  func() gui.Backend { return win32Backend{} },
	})
}
