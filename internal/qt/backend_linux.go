//go:build linux && qt

package qt

import (
	"log"
	"runtime"
	"sync"

	"github.com/JastRedPanda/Nimbus/internal/build"
	"github.com/JastRedPanda/Nimbus/internal/gui"
)

// backend draws with Qt. It satisfies gui.Backend; the windows themselves are in
// qtshim/shim.cpp and are reached through the C functions load_linux.go binds.
type backend struct{}

func (backend) Name() string { return "qt" }

// About and Error are the two windows small enough to live here; the forecast
// panel and the settings form have files of their own.
func (backend) About(theme string) {
	invoke(func() {
		qtAbout("About Nimbus", build.Subtitle, build.Line())
	})
}

func (backend) Error(title, message string) {
	invoke(func() { qtError(title, message) })
}

// Run owns the calling goroutine's thread and runs the Qt event loop until Quit.
//
// Qt has the same thread affinity GTK does: the loop belongs to the thread that
// created the QApplication, and every widget call has to arrive there - which is
// what invoke is for. LockOSThread is what makes that promise keepable, since a
// goroutine is otherwise free to move between threads at any function call.
func (backend) Run() {
	runtime.LockOSThread()
	started = qtInit() != 0
	if started {
		// Before the loop, and therefore before any window: the icon belongs to
		// the application, so installing it here covers every window this
		// process will ever open.
		setAppIcon()
	}
	// Published before the loop starts, so a menu click that arrived during
	// startup is answered rather than dropped - see invoke.
	close(ready)
	if !started {
		log.Print("qt: QApplication could not start; no Qt windows will be drawn")
		return
	}
	qtRun()
}

// Quit leaves the loop. Safe from any goroutine - the shim posts it rather than
// touching a widget.
func (backend) Quit() { qtQuit() }

var probeOnce struct {
	sync.Once
	ok bool
}

// probe answers whether this backend can draw at all: the shim has to load, which
// on a machine without Qt it cannot, because the object needs libQt6Widgets and
// its forty relatives at load time.
//
// It deliberately does NOT create a QApplication to find out. Qt aborts the
// process when it cannot reach a display, so asking that question the direct way
// would turn "no display" into a crash during backend selection.
func probe() bool {
	probeOnce.Do(func() {
		if err := load(); err != nil {
			logLoadFailure(err)
			return
		}
		probeOnce.ok = true
	})
	return probeOnce.ok
}

func init() {
	gui.Register(gui.Factory{
		Name: "qt",
		// The only native backend in this binary: a Qt build carries no GTK at
		// all, by build tag, so there is nothing to outrank. What is left below
		// it is the browser fallback at rank 0, which is what draws the windows
		// if Qt cannot be loaded - the package declares Qt as a dependency, so
		// that is a broken installation rather than an expected state, but it is
		// better to open a browser tab than to draw nothing.
		Rank:  100,
		Probe: probe,
		Open: func() gui.Backend {
			if !probe() {
				return nil
			}
			return backend{}
		},
	})
}
