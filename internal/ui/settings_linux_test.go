//go:build linux

package ui

import "testing"

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
