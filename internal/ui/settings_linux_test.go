//go:build linux && !qt

package ui

import (
	"testing"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
)

// The arithmetic that places a panel at a remembered position. It lives in this
// file because it needs no display: everything else in placePanel talks to GDK,
// and these two functions are deliberately the parts that do not.
//
// What used to be here as well - the mapping between stored values and form
// options - moved to internal/config with the code it covers, where it runs on
// every platform rather than only on the one this file is tagged for.

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
