//go:build !linux

package tray

import (
	"fyne.io/systray"
	"github.com/JastRedPanda/Nimbus/internal/config"
)

// Run starts the tray and its native event loop. Only the Linux build has a
// GTK loop to own; elsewhere the tray library runs the platform's own.
func Run(cfg *config.Config) {
	a := newApp(cfg)
	// Set before the loop starts: the handler's presence decides how the item
	// advertises itself, and that is computed during startup.
	systray.SetOnTapped(func() { a.showForecast() })
	systray.Run(a.ready, func() {})
}

func quit() { systray.Quit() }
