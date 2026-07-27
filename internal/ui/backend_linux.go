//go:build linux

package ui

import (
	"log"

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

func (gtkBackend) Forecast(req gui.Forecast) { showForecast(req) }

func (gtkBackend) About(theme string) { showAbout(theme) }

// Error marshals onto the GTK thread like every other method here. The Backend
// contract says a method may be called from any goroutine, and both calls inside
// need the toolkit thread: ensureAppIcon touches GDK, and ShowError reads and
// writes the single-dialog handle that is documented GTK-thread-only.
func (gtkBackend) Error(title, message string) {
	if err := gtk.Invoke(func() {
		ensureAppIcon()
		gtk.ShowError(title, message, "", "Close")
	}); err != nil {
		log.Printf("gui: cannot show the error dialog: %v (message was: %s)", err, message)
	}
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
