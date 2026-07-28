package config

import (
	"strconv"
	"strings"
)

// The vocabulary the settings windows and the configuration file share, and the
// arithmetic that maps one onto the other.
//
// It lives here rather than beside a settings window because there are three of
// them - GTK, Win32 and Qt - and everything in this file was, or was about to
// be, written once per backend. Two of those copies had already drifted by the
// time this was collected: ParseCoord trimmed whitespace on Windows and not on
// Linux, so a coordinate pasted with padding was accepted on one platform and
// silently discarded on the other. This side of the boundary is also the right
// one on the merits - the config is what says which values are legal - and it
// has the practical advantage that the tests run on every platform instead of
// only the one whose backend happened to hold the code.

// ParseCoord keeps the previous value when the field holds nonsense, rather than
// silently moving the user to the Gulf of Guinea. Trimming is deliberate: a
// pasted value is exactly where the padding comes from.
func ParseCoord(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return fallback
	}
	return v
}

// Index maps a stored value to its position among the options a form offers. An
// unknown value falls back to the first, which is what the config defaults to -
// a file written by a newer version, or edited by hand, must still select
// something, because a form that shows nothing selected cannot be read.
func Index(value string, options ...string) int {
	for i, o := range options {
		if o == value {
			return i
		}
	}
	return 0
}

// Pick is Index the other way round, and clamps for the same reason: an index
// out of range means the group could not be read, and the first option is the
// safe answer because it is the one Index gives an unrecognised value.
func Pick(i int, options ...string) string {
	if i < 0 || i >= len(options) {
		return options[0]
	}
	return options[i]
}

// Interval is one entry in the update-interval dropdown.
type Interval struct {
	Minutes int
	Label   string
}

// Intervals is what the dropdown offers, in the order it offers it.
//
// Note that Default().UpdateInterval is 10 minutes, which is NOT one of these,
// so the dropdown cannot show it and saving silently changes the stored value to
// five. That is inherited behaviour, pinned by a test so it stays a decision
// rather than a surprise.
var Intervals = []Interval{
	{5, "5 min"},
	{30, "30 min"},
	{60, "1 hour"},
	{720, "12 hours"},
	{1440, "24 hours"},
}

// IntervalIndex is the position of a stored interval, or the first option when
// the stored value is not one of the offered ones.
func IntervalIndex(minutes int) int {
	for i, iv := range Intervals {
		if iv.Minutes == minutes {
			return i
		}
	}
	return 0
}

// IntervalLabels is the captions alone, for a dropdown that takes a list of
// strings.
func IntervalLabels() []string {
	out := make([]string, len(Intervals))
	for i, iv := range Intervals {
		out[i] = iv.Label
	}
	return out
}
