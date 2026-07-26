package gui

import (
	"testing"

	"github.com/JastRedPanda/Nimbus/internal/config"
)

type fake struct{ name string }

func (f fake) Name() string                                    { return f.name }
func (fake) Settings(*config.Config, func(int)) *config.Config { return nil }
func (fake) Forecast(Forecast)                                 {}
func (fake) About(string)                                      {}
func (fake) Error(string, string)                              {}

// withFactories swaps the registry for the duration of one test. The registry
// is package state that real backends fill from init(), so a test must not add
// to it or the outcome would depend on which platform ran the test.
func withFactories(t *testing.T, fs ...Factory) {
	t.Helper()
	mu.Lock()
	saved, savedCurrent := factories, current
	factories, current = fs, nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		factories, current = saved, savedCurrent
		mu.Unlock()
	})
}

func f(name string, rank int, probe bool) Factory {
	return Factory{
		Name:  name,
		Rank:  rank,
		Probe: func() bool { return probe },
		Open:  func() Backend { return fake{name} },
	}
}

func TestHighestRankWins(t *testing.T) {
	withFactories(t, f("low", 1, true), f("high", 10, true))
	if got := Current().Name(); got != "high" {
		t.Errorf("selected %q, want the highest-ranked candidate", got)
	}
}

func TestFailedProbeIsSkipped(t *testing.T) {
	withFactories(t, f("native", 10, false), f("fallback", 1, true))
	if got := Current().Name(); got != "fallback" {
		t.Errorf("selected %q, want the fallback when the native probe fails", got)
	}
}

func TestOpenReturningNilMovesOn(t *testing.T) {
	broken := f("broken", 10, true)
	broken.Open = func() Backend { return nil }
	withFactories(t, broken, f("fallback", 1, true))
	if got := Current().Name(); got != "fallback" {
		t.Errorf("selected %q, want the next candidate when Open yields nothing", got)
	}
}

func TestForcedBackendSkipsProbe(t *testing.T) {
	// Forcing must not fall back: a bug report has to reproduce the
	// configuration it names, not something that happens to work.
	t.Setenv(EnvBackend, "native")
	withFactories(t, f("native", 10, false), f("fallback", 1, true))
	if got := Current().Name(); got != "native" {
		t.Errorf("selected %q, want the forced backend even though its probe fails", got)
	}
}

func TestUnknownForcedNameFallsThrough(t *testing.T) {
	t.Setenv(EnvBackend, "nonexistent")
	withFactories(t, f("fallback", 1, true))
	if got := Current().Name(); got != "fallback" {
		t.Errorf("selected %q, want normal selection after an unknown name", got)
	}
}

func TestNullBackendIsAlwaysRegistered(t *testing.T) {
	// Current must never return nil: a caller forced to nil-check the GUI will
	// eventually forget to.
	if b := Current(); b == nil {
		t.Fatal("Current returned nil")
	}
}
