package ui

import (
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gui"
	"github.com/JastRedPanda/Nimbus/internal/webui"
)

// webBackend serves the windows as HTML from a loopback HTTP server and opens
// the user's browser at them.
//
// It carries no build tag: it is the one backend that works anywhere a browser
// does, which is what makes it a usable floor under every platform. It ranks
// below every native backend precisely because a browser tab is not a window.
type webBackend struct{}

func (webBackend) Name() string { return "web" }

func (webBackend) Settings(cfg *config.Config, _ func(int)) *config.Config {
	// The live font-scale preview has no counterpart here: the form is posted
	// in one go, so there is nothing to preview against.
	return webui.ShowSettings(cfg)
}

func (webBackend) Forecast(req gui.Forecast) {
	webui.ShowForecast(req.Lat, req.Lon, req.Units, req.Lang, req.Theme, req.WindUnit)
}

func (webBackend) About(theme string) { webui.ShowAbout(theme) }

func (webBackend) Error(title, message string) { webui.ShowError(title, message) }

func init() {
	gui.Register(gui.Factory{
		Name:  "web",
		Rank:  0,
		Probe: func() bool { return true },
		Open:  func() gui.Backend { return webBackend{} },
	})
}
