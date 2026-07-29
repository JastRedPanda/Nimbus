//go:build !linux

package tray

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/JastRedPanda/Nimbus/internal/config"
)

// quitGrace is how long a quit request is given to end the process the ordinary
// way before it is forced. The native loop turns a posted quit message into an
// exit in milliseconds, so anything still alive this much later is stuck rather
// than slow, and the margin is wide enough that a healthy shutdown never reaches
// it.
const quitGrace = 5 * time.Second

var (
	quitOnce sync.Once
	quitCh   = make(chan struct{})
)

// Run starts the tray and its native event loop. Only the Linux build has a
// GTK loop to own; elsewhere the tray library runs the platform's own.
func Run(cfg *config.Config) {
	a := newApp(cfg)
	// Set before the loop starts: the handler's presence decides how the item
	// advertises itself, and that is computed during startup.
	systray.SetOnTapped(func() { a.showForecast() })

	// Both before the loop, so the first quit request already has something
	// waiting to guarantee it.
	go watchSignals()
	go forceExitOnStall()

	systray.Run(a.ready, func() {})
}

// quit asks the process to end, and guarantees that it does.
//
// systray.Quit posts a close message to the native window and returns, so it
// only ends the process if the message pump is still dispatching; worse, it is
// guarded by a sync.Once inside the library, so a request lost that way can
// never be retried. Publishing the request on quitCh as well gives
// forceExitOnStall the second path that the posted message does not have.
func quit() {
	log.Print("tray: quit requested")
	quitOnce.Do(func() { close(quitCh) })
	systray.Quit()
}

// forceExitOnStall ends the process once a quit request has gone unanswered for
// quitGrace, whatever the native loop is doing.
//
// A forced exit skips the tray library's own teardown: its exit callback, which
// is empty here, and destroying the tray window. The shell drops the icon when
// the process goes either way, and config writes are atomic temp-file renames,
// so nothing in flight is lost. Set against that, the alternative is a process
// the user can only end from the task manager.
func forceExitOnStall() {
	<-quitCh
	time.Sleep(quitGrace)
	log.Printf("tray: the native loop has not exited %s after the quit request, forcing the exit", quitGrace)
	os.Exit(0)
}

// watchSignals routes the polite termination signals into the same quit as the
// menu item, so a logout or a Ctrl-C gets the same teardown and the same
// guarantee instead of dying by signal.
func watchSignals() {
	// Buffered: signal delivery never blocks, so an unbuffered channel would
	// drop the second signal - the one that has to be the way out when the first
	// quit is itself stuck.
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	s := <-ch
	log.Printf("tray: %v received, quitting", s)
	quit()

	// Notify has taken over the default disposition, so without this a second
	// signal would do nothing at all.
	s = <-ch
	log.Printf("tray: %v received while already quitting, exiting immediately", s)
	os.Exit(1)
}
