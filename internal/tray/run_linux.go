//go:build linux

package tray

import (
	"log"

	"fyne.io/systray"
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gtk"
)

// Run owns the GTK main loop for the process and runs the tray alongside it on
// its own D-Bus goroutine.
//
// The tray library is pure Go and starts no toolkit of its own, so if Nimbus
// did not run GTK here there would be no loop at all and every native window
// would quietly fall back to the browser UI.
func Run(cfg *config.Config) {
	// gtk_init and gtk_main have to happen on one and the same OS thread. A
	// goroutine can be migrated between them otherwise - not while blocked
	// inside the loop, but during Init itself, which is enough to break GTK.
	gtk.LockThread()
	if err := gtk.Init(); err != nil {
		log.Printf("tray: GTK unavailable, windows will use the browser UI: %v", err)
	}

	a := newApp(cfg)
	start, end := systray.RunWithExternalLoop(a.ready, func() {})

	// Registered before start(): the library derives the item's ItemIsMenu
	// D-Bus property from whether a tap handler exists, and it computes that
	// during startup. Register afterwards and the host is told the icon is
	// menu-only, so it never forwards a click - silently, with no error.
	systray.SetOnTapped(func() { a.showForecast() })

	start()
	gtk.Main()
	// Only now: tearing the tray down first would close its bus connection and
	// run the exit callback while the loop is still dispatching.
	end()
}

// quit ends the process by stopping the loop Run is blocked on. systray.Quit
// alone is a no-op under an external loop - it closes a channel nobody reads.
func quit() { gtk.Invoke(gtk.MainQuit) }
