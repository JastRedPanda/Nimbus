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
}

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
			log.Printf("gui: backend %q was forced but could not start", forced)
			return nil
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
	return nil
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
