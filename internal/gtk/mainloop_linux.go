//go:build linux

package gtk

import (
	"errors"
	"runtime"
	"sync"
)

var (
	initOnce sync.Once
	initErr  error
	started  bool
)

// Init initialises GTK on the calling goroutine.
//
// The caller MUST have called runtime.LockOSThread first, and MUST call Main
// from that same goroutine afterwards. The hazard is not that the loop wanders
// - a goroutine blocked inside gtk_main cannot migrate, because purego routes
// the call through the same machinery cgo uses and that pins the thread. The
// hazard is earlier: without a lock the goroutine can move threads part way
// through Init itself, leaving gtk_init and gtk_main on two different threads,
// which GTK does not survive.
func Init() error {
	initOnce.Do(func() {
		if initErr = Load(); initErr != nil {
			return
		}
		if initCheck(0, 0) == 0 {
			initErr = errors.New("gtk: gtk_init_check failed (no display?)")
			return
		}
		started = true
	})
	return initErr
}

// Main runs the GTK event loop and blocks until MainQuit. It must be called
// from the same goroutine that called Init.
func Main() {
	if !started {
		return
	}
	mainRun()
}

// MainQuit asks the loop to return. It must run ON the loop - schedule it with
// Invoke rather than calling it from another goroutine.
func MainQuit() { mainQuit() }

// LockThread pins the calling goroutine for the lifetime of the process. It is
// written here rather than inherited from whatever the tray library happens to
// do in its own package init, so the guarantee is visible where it matters.
func LockThread() { runtime.LockOSThread() }
