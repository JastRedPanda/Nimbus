//go:build linux

package ui

import (
	"testing"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
)

// These cover the mapping between what the config stores and what the form
// shows. Nothing here opens a window or writes a file: the Save handler calls
// config.Save, which would overwrite the real user configuration.

func TestIndexFindsOption(t *testing.T) {
	for _, c := range []struct {
		value string
		want  int
	}{
		{"celsius", 0}, {"fahrenheit", 1},
	} {
		if got := index(c.value, "celsius", "fahrenheit"); got != c.want {
			t.Errorf("index(%q) = %d, want %d", c.value, got, c.want)
		}
	}
}

func TestIndexFallsBackToFirst(t *testing.T) {
	// A config written by a newer version, or edited by hand, must not select
	// nothing at all - the form has to show something.
	if got := index("kelvin", "celsius", "fahrenheit"); got != 0 {
		t.Errorf("unknown value gave %d, want the first option", got)
	}
}

func TestPickRoundTripsWithIndex(t *testing.T) {
	opts := []string{"auto", "dark", "light"}
	for _, v := range opts {
		if got := pick(index(v, opts...), opts...); got != v {
			t.Errorf("round trip of %q gave %q", v, got)
		}
	}
}

func TestPickClampsOutOfRange(t *testing.T) {
	for _, i := range []int{-1, 3, 99} {
		if got := pick(i, "a", "b"); got != "a" {
			t.Errorf("pick(%d) = %q, want the first option", i, got)
		}
	}
}

func TestParseCoordKeepsPreviousOnGarbage(t *testing.T) {
	// An empty or mistyped field must not silently relocate the user.
	for _, s := range []string{"", "abc", "50,45", "--"} {
		if got := parseCoord(s, 50.4501); got != 50.4501 {
			t.Errorf("parseCoord(%q) = %v, want the fallback", s, got)
		}
	}
	if got := parseCoord("-33.8688", 0); got != -33.8688 {
		t.Errorf("parseCoord of a valid negative coordinate gave %v", got)
	}
}

func TestIntervalIndexMatchesLabels(t *testing.T) {
	if len(intervalLabels()) != len(intervals) {
		t.Fatal("label list and interval list have drifted apart")
	}
	for i, iv := range intervals {
		if got := intervalIndex(iv.minutes); got != i {
			t.Errorf("intervalIndex(%d) = %d, want %d", iv.minutes, got, i)
		}
	}
}

func TestIntervalIndexUnknownValue(t *testing.T) {
	// config.Default() stores 10 minutes, which is not one of the offered
	// options, so the dropdown cannot show it. Saving then silently changes the
	// stored value - inherited from the other backends, which offer the same
	// five choices, but worth pinning so it is a decision rather than a
	// surprise.
	if got := intervalIndex(10); got != 0 {
		t.Errorf("intervalIndex(10) = %d, want the first option", got)
	}
}

// The rest cover the arithmetic that places a panel at a remembered position.
// They live in this file because they need no display: everything else in
// placePanel talks to GDK, and these two functions are deliberately the parts
// that do not.

func TestInsideRejectsPointsOffTheRectangle(t *testing.T) {
	// A second monitor left of the primary one, which is why the negative
	// coordinates below are ordinary rather than nonsense.
	area := gtk.Rect{X: -1920, Y: 0, W: 1920, H: 1080}
	for _, c := range []struct {
		x, y int
		want bool
	}{
		{-1920, 0, true},     // the top-left corner belongs to the area
		{-1000, 500, true},   // well inside
		{0, 500, false},      // one past the right edge, on the next monitor
		{-1920, 1080, false}, // one past the bottom edge
		{-1921, 500, false},  // one before the left edge
		{100, 100, false},    // the monitor this was saved on is gone
	} {
		if got := inside(area, c.x, c.y); got != c.want {
			t.Errorf("inside(%v, %d, %d) = %v, want %v", area, c.x, c.y, got, c.want)
		}
	}
}

func TestInsideAcceptsAPointOverTheDesktopPanel(t *testing.T) {
	// The rectangle handed to inside is monitor geometry, not the work area, and
	// this is the case that distinguishes them: a panel dropped with its top-left
	// corner under a 24px top panel is still on that monitor, so the position is
	// kept and clamped back into view. Against a work area starting at Y=24 the
	// same point read as off-screen and the drag was thrown away, while Windows
	// honoured it - the parity gap this test exists to hold shut.
	geom := gtk.Rect{X: 0, Y: 0, W: 1920, H: 1080}
	if !inside(geom, 300, 5) {
		t.Error("a point over the desktop panel reads as off every monitor")
	}
}

func TestClampToAreaKeepsThePanelWhole(t *testing.T) {
	area := gtk.Rect{X: 0, Y: 24, W: 1920, H: 1056} // a 24px top panel
	for _, c := range []struct {
		name         string
		x, y, w, h   int
		wantX, wantY int
	}{
		{"already inside is untouched", 300, 400, 620, 500, 300, 400},
		{"flush against the edges is untouched", 1300, 580, 620, 500, 1300, 580},
		{"hanging off the right is pulled in", 1800, 400, 620, 500, 1300, 400},
		{"hanging off the bottom is pulled up", 300, 1000, 620, 500, 300, 580},
		{"above the work area drops below the top panel", 300, 0, 620, 500, 300, 24},
		{"left of the work area is pulled right", -50, 400, 620, 500, 0, 400},
		{"taller than the work area shows its top", 300, 400, 620, 2000, 300, 24},
		{"wider than the work area shows its left", 300, 400, 3000, 500, 0, 400},
	} {
		x, y := clampToArea(c.x, c.y, c.w, c.h, area)
		if x != c.wantX || y != c.wantY {
			t.Errorf("%s: clampToArea(%d, %d, %d, %d) = %d,%d, want %d,%d",
				c.name, c.x, c.y, c.w, c.h, x, y, c.wantX, c.wantY)
		}
	}
}
