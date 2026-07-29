//go:build linux

package gtk

import (
	"errors"
	"log"
	"runtime"
	"sync"
)

// mainQuitAttempts caps how many enclosing main loops MainQuit will chase. GTK
// nesting deeper than this inside Nimbus would be a bug of its own, and a bound
// is cheaper than trusting the level to keep dropping on every iteration.
const mainQuitAttempts = 8

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
//
// gtk_main_quit ends only the innermost loop, so one call is not enough when a
// nested gtk_main is on the stack: the outer one keeps running, Main stays
// blocked, and on the quit path that means a process the user cannot close.
//
// The levels cannot be drained in a single callback either. gtk_main_level only
// drops once a gtk_main has actually unwound, which cannot happen while this
// callback is still on its stack, so "for mainLevel() > 0 { mainQuit() }" would
// spin forever and take the loop's own thread with it. Each call therefore ends
// one level and, if another is still above it, queues itself again: the loop
// that is left dispatches that idle as soon as the inner one has returned.
//
// A loop nested by g_main_loop_run rather than gtk_main - gtk_dialog_run is the
// one Nimbus could plausibly meet - is invisible to gtk_main_level and cannot be
// ended from here at all. That is why ShowError does not use gtk_dialog_run, and
// why the caller still needs a fallback that does not depend on the loop.
func MainQuit() {
	if !started {
		// GTK never initialised, so there is no loop to end and nothing here can
		// stop the process. The caller must not treat this as "the quit has been
		// scheduled": tray.quit decides between this and closing its own channel
		// from the choice Run actually made, not from asking whether GTK is
		// available, precisely so a silent return here cannot swallow a quit.
		return
	}
	quitOneLevel(mainQuitAttempts)
}

func quitOneLevel(attempts int) {
	mainQuit()
	if mainLevel() <= 1 {
		return
	}
	if attempts <= 1 {
		log.Printf("gtk: %d main loops are still nested after as many quit requests, giving up on a clean return", mainLevel())
		return
	}
	if err := Invoke(func() { quitOneLevel(attempts - 1) }); err != nil {
		log.Printf("gtk: cannot queue the quit for the enclosing main loop: %v", err)
	}
}

// LockThread pins the calling goroutine for the lifetime of the process. It is
// written here rather than inherited from whatever the tray library happens to
// do in its own package init, so the guarantee is visible where it matters.
func LockThread() { runtime.LockOSThread() }
