// Package gui is the seam between Nimbus and whatever draws its windows.
//
// The contract is deliberately at the level of the application's TASKS - show
// the settings and give me back a config, show the forecast, show the about box
// - and not at the level of widgets or layout. No geometry, no control types
// and no coordinates cross this boundary. That rule is the whole reason the
// seam is finishable: a widget-and-layout abstraction has to reimplement a
// layout model once per backend, which is precisely where libui spent two
// maintainer generations and still calls itself mid-alpha.
//
// The Backend interface is INTERNAL AND UNSTABLE ON PURPOSE. It was designed
// against one real implementation, so the second one will certainly want
// something it does not offer, or be unable to honour something it promises.
// Keeping it unexported-in-spirit is the licence to change it freely when that
// happens, instead of treating every new backend as a breaking change - the
// same reason Qt gives no compatibility guarantee for its own platform layer.
package gui

import (
	"log"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/config"
)

// EnvBackend forces a named backend, skipping probing entirely, so a bug report
// reproduces the configuration it names instead of quietly falling back to
// something that happens to work.
const EnvBackend = "NIMBUS_GUI_BACKEND"

// Forecast is everything a backend needs to draw the forecast view.
//
// It is a struct rather than six positional arguments because the values belong
// together. The trade-off is worth stating: adding a METHOD to Backend breaks
// every backend that has not implemented it, which is the compiler doing the
// review for us, whereas adding a FIELD here compiles clean against a backend
// that ignores it. Put behaviour in methods; put only genuinely shared data
// here.
type Forecast struct {
	Lat, Lon float64
	Units    string // "celsius" | "fahrenheit"
	Lang     string // "en" | "uk"
	Theme    string // "auto" | "dark" | "light"
	WindUnit string // "ms" | "kmh"

	// At is where to put the panel. nil means anchor it at the corner nearest
	// the pointer, which is what a panel the user has never moved does - see
	// OnMove for what "moved" has to mean before a position is remembered at all.
	//
	// Deciding this is the caller's job, not the panel's: the caller is the only
	// one that knows whether a remembered position should be honoured at all.
	At *Point

	// Interval задаёт периодичность автообновления данных. 0 — не обновлять.
	Interval time.Duration

	// OnMove reports the panel's final position as it closes, so a caller that
	// wants to remember it can. May be nil.
	//
	// It is called ONLY IF the panel actually changed position under the user's
	// hand during this showing, which takes BOTH of two things: a press was handed
	// to the window manager's move loop at least once, and the window's position at
	// close differs from its position at that first handoff.
	//
	// Neither half is enough. Without the handoff, the first ever open would report
	// the position the panel was merely placed at, and a caller that treats "no
	// remembered position" as "anchor near the pointer" would lose pointer
	// anchoring forever after one glance at the forecast. Without the position
	// comparison, a bare click on the panel body would do the same, because a click
	// is handed to the move loop exactly like a drag - as is a drag cancelled with
	// Escape, which several window managers undo as part of their own interactive-
	// move gesture and will keep undoing regardless of whether the application
	// itself is listening for the key.
	//
	// Backends read BOTH positions through the same call - gtk_window_get_position
	// on GTK, GetWindowRect on Win32 - and never compare against the placement they
	// asked for. That is what keeps the rule correct on Wayland, where a client
	// cannot know its own toplevel's position and the toolkit answers (0,0) for
	// every window: both reads are then equal, so nothing is reported, which is the
	// right answer there rather than persisting a corner the user never chose.
	//
	// It is always called on a throwaway goroutine, with the coordinates already
	// read on whatever thread owns the windows. That is one contract for every
	// backend, and the reason for it is that the callback writes the config file:
	// running it inline would put a disk write inside the toolkit's main loop.
	OnMove func(x, y int)
}

// Point is a screen position in the same coordinate space the window manager
// uses, which is why negative values are ordinary rather than a mistake: a
// monitor placed left of the primary one has them.
type Point struct{ X, Y int }

// Backend draws Nimbus's windows. Every method may be called from any
// goroutine; a backend that needs a particular thread marshals internally.
type Backend interface {
	// Name identifies the backend in logs and in EnvBackend.
	Name() string

	// Settings opens the settings window and blocks until the user is done.
	// It returns the configuration to adopt, or nil if nothing should change.
	// Cancelling must return nil rather than blocking forever.
	Settings(cfg *config.Config, onFontScale func(int)) *config.Config

	// Forecast opens the forecast view and returns immediately. Fetching the
	// data is the backend's business, so a slow network cannot stall the
	// caller - which is the tray's single menu-dispatch loop.
	Forecast(req Forecast)

	// About opens the about window and returns immediately.
	About(theme string)

	// Error reports a failure to the user. It must work on every machine the
	// backend claims to support: it is the last thing left when something
	// else has already gone wrong.
	Error(title, message string)
}

// Looper is implemented by a backend that owns an event loop of its own.
//
// Most backends do not: the browser fallback has no loop, the null one draws
// nothing, and the GTK backend's loop belongs to the process rather than to the
// backend - internal/tray has run it since before this contract existed, and
// moving it would be churn for nothing. A toolkit that has to be started and
// stopped as a unit, as Qt does, says so by implementing this instead.
//
// Run blocks until Quit, and owns the goroutine's OS thread for that whole time.
// Quit may be called from any goroutine.
type Looper interface {
	Run()
	Quit()
}

// Factory describes a backend that MIGHT be usable on this machine.
type Factory struct {
	// Name is matched against EnvBackend.
	Name string

	// Rank orders the candidates, highest first. Native backends outrank
	// fallbacks.
	Rank int

	// Probe reports whether this backend can run here and now. It must ask the
	// system what it can do - can this library be loaded, is a compositor
	// running - and must not ask which desktop environment it is on. The
	// desktop's name predicts nothing reliable: a GTK app runs perfectly well
	// under KDE, and what actually varies between desktops is which services
	// are present, which is what a probe measures directly.
	Probe func() bool

	// Open builds the backend. Returning nil means it turned out to be
	// unusable after all, and selection moves on to the next candidate.
	Open func() Backend
}

var (
	mu        sync.Mutex
	factories []Factory
	current   Backend
)

// Register adds a candidate. Backends call it from init() in a build-tagged
// file, so the set of candidates is decided at compile time by target and only
// the choice between them is made at runtime.
func Register(f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories = append(factories, f)
}

// Current returns the selected backend, choosing one on first use.
func Current() Backend {
	mu.Lock()
	defer mu.Unlock()
	if current == nil {
		current = choose()
	}
	return current
}

func choose() Backend {
	sorted := append([]Factory(nil), factories...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Rank > sorted[j].Rank })

	if forced := os.Getenv(EnvBackend); forced != "" {
		for _, f := range sorted {
			if f.Name != forced {
				continue
			}
			// Probing is skipped on purpose: the user asserted this backend,
			// and a silent fallback would hide the very thing being reported.
			if b := f.Open(); b != nil {
				log.Printf("gui: using backend %q (forced by %s)", f.Name, EnvBackend)
				return b
			}
			// The null backend, not nil. nullBackend's own doc promises Current
			// never returns nil precisely because a caller that has to nil-check
			// the GUI will eventually forget, and this was the one path that broke
			// that promise: a forced backend that failed to open cached a nil
			// interface, and the next tray click dereferenced it. The forced name
			// is still honoured in the sense that matters - no silent fallback to
			// a DIFFERENT drawing backend, which is what the probe skip above is
			// about; the null backend draws nothing and says so.
			log.Printf("gui: backend %q was forced but could not start; nothing will be drawn", forced)
			return nullBackend{}
		}
		log.Printf("gui: unknown backend %q, ignoring %s (known: %s)", forced, EnvBackend, names(sorted))
	}

	for _, f := range sorted {
		if f.Probe != nil && !f.Probe() {
			continue
		}
		if b := f.Open(); b != nil {
			log.Printf("gui: using backend %q", f.Name)
			return b
		}
	}
	// Unreachable while the null backend is registered - it probes true and always
	// opens - but stated rather than assumed, for the same reason as above.
	log.Print("gui: no backend could be opened at all; nothing will be drawn")
	return nullBackend{}
}

func names(fs []Factory) string {
	out := ""
	for i, f := range fs {
		if i > 0 {
			out += ", "
		}
		out += f.Name
	}
	return out
}
