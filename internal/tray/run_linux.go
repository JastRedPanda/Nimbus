//go:build linux

package tray

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gui"
)

// quitGrace is how long a quit request is given to end the process the ordinary
// way before it is forced.
//
// An ordinary quit never comes close: the GTK loop drains a queued idle in
// microseconds, and the teardown after it is one D-Bus socket close. Anything
// still alive this much later is stuck, not slow. The value is generous on
// purpose, because being wrong in the other direction means cutting a healthy
// shutdown short.
const quitGrace = 5 * time.Second

var (
	quitOnce sync.Once
	quitCh   = make(chan struct{})

	// usingToolkit records the decision Run actually made about THIS file's own
	// loop, which is what quit has to act on. Asking the toolkit whether it could
	// be used answers a different question, and the two agreeing today is an
	// accident of ordering rather than a guarantee. Written before any goroutine
	// that reads it is started, and always false in a build whose backend brings
	// its own loop - see toolkit_qt.go.
	usingToolkit bool

	// loop is the chosen backend's own event loop, when it has one - Qt does, GTK
	// does not, because the GTK loop belongs to this package. Written before
	// start() and read by quit afterwards, so the ordering needs no lock.
	loop gui.Looper

	// loopReturned only sharpens the wording of the watchdog's log line, telling
	// a loop that refused to return apart from a teardown that hung after it. The
	// forced exit does not depend on it.
	loopReturned atomic.Bool
)

// Run owns the process's main loop and runs the tray alongside it on its own
// D-Bus goroutine.
//
// WHOSE loop that is depends on the build. The GTK build runs GTK's, started
// here in toolkit_gtk.go; the Qt build runs the backend's own through
// gui.Looper. The tray library itself is pure Go and starts no toolkit at all,
// so without one of the two there would be no loop, and every native window
// would quietly fall back to the browser UI.
func Run(cfg *config.Config) {
	// Whatever toolkit this build was compiled with, started here and on this
	// thread. In a build whose backend owns its own loop this does nothing at
	// all - see toolkit_qt.go.
	toolkitInit()

	a := newApp(cfg)
	start, end := systray.RunWithExternalLoop(a.ready, func() {})

	// Registered before start(): the library derives the item's ItemIsMenu
	// D-Bus property from whether a tap handler exists, and it computes that
	// during startup. Register afterwards and the host is told the icon is
	// menu-only, so it never forwards a click - silently, with no error.
	systray.SetOnTapped(func() { a.showForecast() })

	// Both before start(), so that the very first quit request - whichever way it
	// arrives - already has something waiting to guarantee it.
	go watchSignals()
	go forceExitOnStall()

	start()

	// Which loop this process runs is the chosen backend's business, and asking
	// here is what forces the choice to be made - before this, selection happened
	// lazily on the first window. A backend with a loop of its own owns the thread
	// from now until quit; everything else falls through to the arrangement that
	// was here before.
	if l, ok := gui.Current().(gui.Looper); ok {
		loop = l
		usingToolkit = false
		loop.Run()
	} else if !toolkitRun() {
		// The tray speaks D-Bus and needs no toolkit at all, so on a machine
		// where the toolkit is missing the icon, its menu and its tooltip still
		// work and only the windows degrade. Something has to block here regardless: without
		// it the process falls straight through to end() and the icon appears
		// and vanishes.
		<-quitCh
	}
	loopReturned.Store(true)
	log.Print("tray: main loop returned, releasing the tray")

	// Only now: tearing the tray down first would close its bus connection and
	// run the exit callback while the loop is still dispatching.
	releaseTray(end)
}

// quit asks the process to end, and guarantees that it does.
//
// The ordinary path is unchanged - post gtk_main_quit onto the loop, let Main
// return, let Run release the tray cleanly - but it is no longer the only path.
// A queued idle is not a dispatched idle: a loop that has stopped dispatching,
// whether from a dangling toolkit grab, a nested loop or a frozen X server,
// swallows the request with no error and no log line, and the user is left with
// a live process and a tray icon that answers nothing. That is how this bug was
// reported. So the request is also published on quitCh, where forceExitOnStall
// is waiting to end the process by force if the loop has not done it within
// quitGrace.
func quit() {
	log.Print("tray: quit requested")

	if loop != nil {
		loop.Quit()
	}

	toolkitQuit()

	// Always, on every path: Run itself reads this channel when there is no
	// toolkit loop to end, and the watchdog reads it either way.
	quitOnce.Do(func() { close(quitCh) })
}

// forceExitOnStall ends the process once a quit request has gone unanswered for
// quitGrace, whatever the toolkit is doing.
//
// What a forced exit skips is the tray teardown: systray's exit callback, which
// is empty here, and closing its D-Bus connection. That connection is a socket
// the kernel closes on exit anyway, and the bus emits the same NameOwnerChanged
// either way, so the host drops the icon just the same and no ghost is left in
// the panel. Nothing else is in flight to lose - the log file is unbuffered and
// config writes are atomic temp-file renames, so a config save is either fully
// applied or not at all. Set against that, the alternative is a process the user
// can only end with kill, which is the incident this exists to prevent.
//
// The exit status is 0 because the user got what they asked for; the log line is
// what records that it took force, so a shutdown that visibly stalls for a few
// seconds has an explanation instead of being a mystery.
func forceExitOnStall() {
	<-quitCh
	time.Sleep(quitGrace)

	if loopReturned.Load() {
		log.Printf("tray: the tray teardown has not finished %s after the quit request, forcing the exit", quitGrace)
	} else {
		log.Printf("tray: the main loop has not returned %s after the quit request, forcing the exit", quitGrace)
	}
	os.Exit(0)
}

// watchSignals routes the polite termination signals into the same quit as the
// menu item, so a session logging out - or a user who has given up and reached
// for kill - gets the same teardown and the same guarantee. Until now nothing
// handled them: the process died by signal, and the log could not tell that
// apart from a wedge, because both simply stop writing.
//
// SIGQUIT and SIGABRT are deliberately left alone. Their default disposition
// dumps every goroutine's stack, which is the one diagnostic that tells a wedged
// GTK loop from a wedged X server, and it must keep working.
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
	// signal would do nothing at all and push the user back to kill -9. The
	// watchdog above would still get there first; this is for the impatient.
	s = <-ch
	log.Printf("tray: %v received while already quitting, exiting immediately", s)
	os.Exit(1)
}

// releaseTray runs the tray library's teardown, absorbing a panic out of it.
//
// systray closes its bus connection unconditionally, but leaves that connection
// nil whenever its own startup bailed out - a session with no D-Bus, which is
// exactly the degraded machine the no-GTK branch of Run exists for. The nil
// dereference that follows would take the process down through a stderr panic
// that never reaches the log file, turning a successful quit into what looks
// like a crash. Everything this call had to do for the user is either already
// done or done by process exit, so log it and let Run return normally.
func releaseTray(end func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("tray: teardown panicked, exiting anyway: %v", r)
		}
	}()
	end()
}
