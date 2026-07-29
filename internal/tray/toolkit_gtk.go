//go:build linux && !qt

package tray

import (
	"log"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
)

// The toolkit half of the Linux tray, GTK edition.
//
// It is a file of its own, and behind a build tag, because the Qt build must not
// contain a single line of it. Its twin - toolkit_qt.go - answers the same three
// questions with nothing at all, since the Qt backend brings a loop of its own.
//
// That is what "strictly one toolkit per binary" is implemented as. Before the
// split, Run called gtk.Init unconditionally, so a Qt build would have loaded
// libgtk-3 at startup on every machine and dragged GTK into the Qt package's
// dependencies for a library it never drew with.

// toolkitInit starts GTK, or records that it could not be started.
//
// gtk_init and gtk_main have to happen on one and the same OS thread. A
// goroutine can be migrated between them otherwise - not while blocked inside
// the loop, but during Init itself, which is enough to break GTK.
func toolkitInit() {
	gtk.LockThread()
	usingToolkit = gtk.Init() == nil
	if !usingToolkit {
		log.Print("tray: GTK unavailable, windows will open in the browser")
	}
}

// toolkitRun runs the GTK loop and reports whether it ran at all. False means
// there is no toolkit loop to run, and the caller has to block some other way.
func toolkitRun() bool {
	if !usingToolkit {
		return false
	}
	gtk.Main()
	return true
}

// toolkitQuit asks the GTK loop to return.
func toolkitQuit() {
	if !usingToolkit {
		return
	}
	// Invoke can only fail when the GTK libraries never loaded, which means
	// there is no loop to post to at all - precisely the case the forced exit
	// exists for. Log it rather than return, because the user still asked to
	// quit.
	if err := gtk.Invoke(gtk.MainQuit); err != nil {
		log.Printf("tray: cannot reach the GTK loop, the exit will have to be forced: %v", err)
	}
}
