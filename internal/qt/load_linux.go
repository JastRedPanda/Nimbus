//go:build linux && qt

// Package qt draws Nimbus's windows with Qt, for desktops where Qt is what the
// rest of the session is drawn with.
//
// HOW IT IS PUT TOGETHER, because it is not what it looks like at first glance.
// Qt is C++ and exports no C ABI - libQt6Widgets exports 7780 symbols and every
// one is a mangled C++ method - so unlike internal/gtk this package does NOT bind
// the toolkit. The windows themselves are written in C++, in qtshim/, and are
// reached through six plain C functions. This file loads that object and binds
// those six.
//
// The object is carried INSIDE the binary and loaded from memory, never from
// disk. That keeps the three properties the program is built around, all of which
// linking Qt into the executable would have cost:
//
//   - the binary is still one file, with nothing to install beside it,
//   - it still has four dynamic dependencies and no versioned glibc symbol, so it
//     still runs on distributions far older than the build host,
//   - and a machine with no Qt still starts it: the load fails, Probe answers
//     false, and the GUI layer picks another backend, exactly as it already does
//     when GTK is missing.
//
// Loading happens through memfd_create, so no file is ever written. That is not
// only tidiness: writing a shared object to a temp path and dlopening it is a
// local privilege-escalation shape - whoever wins the race between the write and
// the load gets code execution inside this process - and it fails outright when
// /tmp is mounted noexec. An anonymous in-memory file has no name to race for.
package qt

import (
	_ "embed"
	"fmt"
	"log"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/JastRedPanda/Nimbus/internal/appicon"
	"github.com/JastRedPanda/Nimbus/internal/fonts"
)

// shim is qtshim/ built with `go build -buildmode=c-shared`. The Makefile's qt
// target regenerates it; it is not checked in, which is why this file is behind
// the qt build tag - an ordinary build must not need a C++ toolchain, or Qt, or
// this file to exist.
//
//go:embed libnimbusqt.so
var shim []byte

// SYS_memfd_create on linux/amd64. Hardcoded rather than taken from x/sys/unix so
// that this package adds no dependency the rest of the program does not have.
const sysMemfdCreate = 319

var (
	loadOnce sync.Once
	loadErr  error

	qtInit     func() int32
	qtRun      func()
	qtQuit     func()
	qtInvoke   func(uintptr, uint64)
	qtAbout    func(string, string, string)
	qtError    func(string, string)
	qtFont     func(unsafe.Pointer, int32)
	qtIcon     func(unsafe.Pointer, int32)
	qtTheme    func(int32)
	qtCanTheme func() int32

	qtPanelBegin  func(string)
	qtPanelHeader func(string, int32)
	qtPanelRow    func(string, string, string, string, string)
	qtPanelShow   func(uint64, int32, int32, int32, uintptr)
	qtPanelClose  func() int32

	qtFormBegin     func(string)
	qtFormGroup     func(string)
	qtFormText      func(int32, string, string, string)
	qtFormList      func(int32, int32)
	qtFormChoice    func(int32, string)
	qtFormCombo     func(int32, string)
	qtFormOption    func(string, int32)
	qtFormSlider    func(int32, string, int32, int32, int32)
	qtFormButtons   func(string, string, string)
	qtFormShow      func(uint64, uintptr, uintptr)
	qtFormSet       func(int32, string)
	qtFormEnable    func(int32, int32)
	qtFormReport    func(int32)
	qtFormListClear func(int32)
	qtFormListAdd   func(int32, string)
)

// The event codes and actions of shim.h. Repeated here rather than derived,
// because a Go build never reads that header - it is compiled into the shim and
// nothing checks that the two agree. They are append-only for that reason: a
// renumbering would have to change both files in the same commit.
const (
	evClosed = 1
	evMoved  = 2
	evAction = 3
	evSearch = 4
	evPick   = 5
	evSlide  = 6

	actionCancel = 0
	actionSave   = 1
	actionReset  = 2

	alignStart  = 0
	alignCenter = 1
	alignEnd    = 2
)

// load maps the embedded object and binds every entry point. It is safe to call
// repeatedly; only the first call does anything.
func load() error {
	loadOnce.Do(func() {
		fd, _, errno := syscall.Syscall(sysMemfdCreate,
			uintptr(unsafe.Pointer(&[]byte("nimbus-qt\x00")[0])), 0, 0)
		if errno != 0 {
			// Old kernel, or a policy that forbids it. Not a fault: the caller
			// treats a load failure as "this backend is unavailable".
			loadErr = fmt.Errorf("memfd_create: %w", errno)
			return
		}
		f := int(fd)
		if _, err := syscall.Write(f, shim); err != nil {
			syscall.Close(f)
			loadErr = fmt.Errorf("writing the shim to memory: %w", err)
			return
		}
		// The descriptor stays open on purpose: /proc/self/fd/N is the only name
		// this object has, and the dynamic loader keeps needing it.
		lib, err := purego.Dlopen(fmt.Sprintf("/proc/self/fd/%d", f),
			purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			syscall.Close(f)
			// The overwhelmingly likely cause is that Qt itself is not installed:
			// the object needs libQt6Widgets and its 40 relatives at load time.
			loadErr = fmt.Errorf("loading the Qt shim (is Qt installed?): %w", err)
			return
		}
		purego.RegisterLibFunc(&qtInit, lib, "nimbus_qt_init")
		purego.RegisterLibFunc(&qtRun, lib, "nimbus_qt_run")
		purego.RegisterLibFunc(&qtQuit, lib, "nimbus_qt_quit")
		purego.RegisterLibFunc(&qtInvoke, lib, "nimbus_qt_invoke")
		purego.RegisterLibFunc(&qtAbout, lib, "nimbus_qt_about")
		purego.RegisterLibFunc(&qtError, lib, "nimbus_qt_error")
		purego.RegisterLibFunc(&qtFont, lib, "nimbus_qt_font")
		purego.RegisterLibFunc(&qtIcon, lib, "nimbus_qt_icon")
		purego.RegisterLibFunc(&qtTheme, lib, "nimbus_qt_theme")
		purego.RegisterLibFunc(&qtCanTheme, lib, "nimbus_qt_can_theme")

		purego.RegisterLibFunc(&qtPanelBegin, lib, "nimbus_qt_forecast_begin")
		purego.RegisterLibFunc(&qtPanelHeader, lib, "nimbus_qt_forecast_header")
		purego.RegisterLibFunc(&qtPanelRow, lib, "nimbus_qt_forecast_row")
		purego.RegisterLibFunc(&qtPanelShow, lib, "nimbus_qt_forecast_show")
		purego.RegisterLibFunc(&qtPanelClose, lib, "nimbus_qt_forecast_close")

		purego.RegisterLibFunc(&qtFormBegin, lib, "nimbus_qt_form_begin")
		purego.RegisterLibFunc(&qtFormGroup, lib, "nimbus_qt_form_group")
		purego.RegisterLibFunc(&qtFormText, lib, "nimbus_qt_form_text")
		purego.RegisterLibFunc(&qtFormList, lib, "nimbus_qt_form_list")
		purego.RegisterLibFunc(&qtFormChoice, lib, "nimbus_qt_form_choice")
		purego.RegisterLibFunc(&qtFormCombo, lib, "nimbus_qt_form_combo")
		purego.RegisterLibFunc(&qtFormOption, lib, "nimbus_qt_form_option")
		purego.RegisterLibFunc(&qtFormSlider, lib, "nimbus_qt_form_slider")
		purego.RegisterLibFunc(&qtFormButtons, lib, "nimbus_qt_form_buttons")
		purego.RegisterLibFunc(&qtFormShow, lib, "nimbus_qt_form_show")
		purego.RegisterLibFunc(&qtFormSet, lib, "nimbus_qt_form_set")
		purego.RegisterLibFunc(&qtFormEnable, lib, "nimbus_qt_form_enable")
		purego.RegisterLibFunc(&qtFormReport, lib, "nimbus_qt_form_report")
		purego.RegisterLibFunc(&qtFormListClear, lib, "nimbus_qt_form_list_clear")
		purego.RegisterLibFunc(&qtFormListAdd, lib, "nimbus_qt_form_list_add")
	})
	return loadErr
}

// setAppIcon gives every window this process opens the application artwork.
//
// Called once, on the Qt thread, right after the QApplication exists: the icon is
// a property of the application rather than of a window, so it reaches windows
// opened later without any of them having to ask.
//
// Without it the windows carried whatever the desktop draws for an application it
// does not recognise. The GTK backend has always installed the same two sizes
// through gtk.SetDefaultIcons; this is that call's twin, and both take their
// bytes from internal/appicon so the two backends cannot end up wearing different
// artwork.
func setAppIcon() {
	for _, b := range appicon.All() {
		if len(b) == 0 {
			continue
		}
		qtIcon(unsafe.Pointer(&b[0]), int32(len(b)))
	}
}

// ensureFont hands Qt the embedded weather typeface, once, so the symbol column
// can be drawn as text. Qt copies the bytes, so the slice is only borrowed for
// the length of the call - and it is a package-level embed either way, which is
// what makes taking its address safe: the Go heap does not move.
//
// Qt thread only, like everything else that reaches into the shim.
var fontOnce sync.Once

func ensureFont() {
	fontOnce.Do(func() {
		b := fonts.TTF()
		if len(b) == 0 {
			return
		}
		qtFont(unsafe.Pointer(&b[0]), int32(len(b)))
	})
}

// The invoke trampoline, and there is exactly one of it.
//
// purego.NewCallback's budget is fixed at about 2000 for the life of the process
// and nothing is ever reclaimed, so a callback per call would run out. One fixed
// trampoline dispatches by id instead, which is the same scheme internal/gtk uses
// for its three signal shapes.
var (
	cbMu        sync.Mutex
	cbSeq       uint64
	cbFns       = map[uint64]func(){}
	trampoline  uintptr
	trampOnce   sync.Once
	dispatchFor = func(id uint64) {
		cbMu.Lock()
		fn := cbFns[id]
		delete(cbFns, id)
		cbMu.Unlock()
		if fn != nil {
			fn()
		}
	}
)

// ready is closed once Run has finished deciding whether Qt started, and started
// tells which way it went. Both are needed: waiting alone would hang a caller
// forever when Qt failed to start, which is the shape of bug this project has
// already been bitten by twice.
var (
	ready   = make(chan struct{})
	started bool
)

// invoke runs fn on the Qt thread and returns without waiting for it. Every call
// that touches a widget goes through here, because Qt permits widget calls only
// from the thread that owns its loop.
//
// It waits for the loop to exist first. The tray dispatches its menu on its own
// goroutine and can reach a window before Run has created the QApplication - the
// gap is milliseconds at startup, but a click that lands in it used to be dropped
// without a word, because the shim answers a call made before init by returning.
// A dropped menu click is indistinguishable from a broken backend.
// It reports whether the call was scheduled at all. A caller that then waits for
// an answer - the settings window blocks on one, the forecast panel asks whether
// a click was consumed - has to know, or it waits for a reply that will never be
// sent.
func invoke(fn func()) bool {
	<-ready
	if !started {
		log.Print("qt: the toolkit never started; this window cannot be drawn")
		return false
	}
	trampOnce.Do(func() { trampoline = purego.NewCallback(dispatchFor) })
	cbMu.Lock()
	cbSeq++
	id := cbSeq
	cbFns[id] = fn
	cbMu.Unlock()
	qtInvoke(trampoline, id)
	return true
}

// The windows the shim can call back into, and the two trampolines it calls
// them through.
//
// Every window gets an id when it is built and hands it to the shim, which
// returns it untouched with each event. That is one indirection more than
// passing a pointer would need, and it is what makes the boundary safe: a Go
// pointer handed to C would have to outlive garbage collection on the say-so of
// a C++ object's lifetime, whereas an integer key into a map owned by this side
// cannot dangle - a callback for a window that has already been dropped finds
// nothing and returns.
//
// All of it is touched only from the Qt thread: registration happens inside an
// invoke, and the callbacks arrive on the loop. The mutex is there for the same
// reason the GTK binding's is - the cost is nothing and the alternative is a
// data race waiting for the day something calls in from elsewhere.
type window struct {
	// event handles NIMBUS_QT_EV_*, and field carries the one thing that does not
	// fit in an integer. Either may be nil for a window that has no use for it.
	event func(code, a, b int64)
	field func(key int64, value string)
}

var (
	winMu   sync.Mutex
	winSeq  uint64
	windows = map[uint64]*window{}

	winTrampOnce sync.Once
	eventTramp   uintptr
	fieldTramp   uintptr
)

func lookup(id uint64) *window {
	winMu.Lock()
	defer winMu.Unlock()
	return windows[id]
}

// register hands back the id and the two trampolines, binding them on first
// use. Two more callbacks and no more, however many windows are opened, for the
// same reason invoke has exactly one: purego's callback budget is fixed for the
// life of the process and nothing is ever reclaimed.
func register(w *window) uint64 {
	winTrampOnce.Do(func() {
		eventTramp = purego.NewCallback(func(id uint64, code, a, b int64) {
			if win := lookup(id); win != nil && win.event != nil {
				win.event(code, a, b)
			}
		})
		fieldTramp = purego.NewCallback(func(id uint64, key int64, value unsafe.Pointer) {
			if win := lookup(id); win != nil && win.field != nil {
				win.field(key, goString(value))
			}
		})
	})
	winMu.Lock()
	defer winMu.Unlock()
	winSeq++
	windows[winSeq] = w
	return winSeq
}

func drop(id uint64) {
	winMu.Lock()
	delete(windows, id)
	winMu.Unlock()
}

// goString copies a NUL-terminated C string. The shim's buffers live only for
// the length of the callback, so copying is not an optimisation to be avoided -
// it is the only correct thing to do.
// The argument is an unsafe.Pointer rather than the uintptr the rest of this
// package uses for C addresses, and that is deliberate: a uintptr would have to
// be converted back before it could be walked, and arithmetic on one is exactly
// what go vet's unsafe.Pointer rule is there to catch. purego passes the
// register through either way.
func goString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// logLoadFailure says once why Qt was not used, at the level of detail someone
// filing a report can act on.
func logLoadFailure(err error) {
	log.Printf("qt: backend unavailable, falling back: %v", err)
}
