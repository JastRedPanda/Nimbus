//go:build linux && qt

package tray

// The toolkit half of the Linux tray, Qt edition, and it is empty on purpose.
//
// The Qt backend owns an event loop of its own and starts it through gui.Looper,
// which Run reaches before it ever gets here - so there is no second toolkit to
// initialise, to run or to stop. usingToolkit stays false for the whole life of
// the process, and every branch that depends on it is skipped.
//
// The point of the file is what it does NOT contain: no import of internal/gtk,
// so a Qt build carries no GTK binding at all and can never dlopen libgtk-3. Its
// twin, toolkit_gtk.go, has the real thing.

func toolkitInit() {}

func toolkitRun() bool { return false }

func toolkitQuit() {}
