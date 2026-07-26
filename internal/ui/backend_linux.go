//go:build linux

package ui

import (
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/gui"
)

// gtkBackend draws real windows with GTK3, loaded at runtime through purego.
type gtkBackend struct{}

func (gtkBackend) Name() string { return "gtk3" }

func (gtkBackend) Settings(cfg *config.Config, onFontScale func(int)) *config.Config {
	return showSettings(cfg, onFontScale)
}

func (gtkBackend) Forecast(req gui.Forecast) {
	showForecast(req.Lat, req.Lon, req.Units, req.Lang, req.Theme, req.WindUnit)
}

func (gtkBackend) About(theme string) { showAbout(theme) }

func (gtkBackend) Error(title, message string) {
	ensureAppIcon()
	gtk.ShowError(title, message, "", "Close")
}

func init() {
	gui.Register(gui.Factory{
		Name: "gtk3",
		Rank: 100,
		// Asks the system what it can do rather than which desktop it is:
		// GTK runs perfectly well under KDE, and a machine without the library
		// is the only case that actually matters here.
		Probe: gtk.Ready,
		Open:  func() gui.Backend { return gtkBackend{} },
	})
}
