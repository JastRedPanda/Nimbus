package config

import "testing"

// The mapping between what the configuration stores and what a settings form
// shows. Untagged, so it runs on every platform - all three backends share this
// code now, and the two that had their own copies of it disagreed.

func TestIndexFindsOption(t *testing.T) {
	for _, c := range []struct {
		value string
		want  int
	}{
		{"celsius", 0}, {"fahrenheit", 1},
	} {
		if got := Index(c.value, "celsius", "fahrenheit"); got != c.want {
			t.Errorf("Index(%q) = %d, want %d", c.value, got, c.want)
		}
	}
}

func TestIndexFallsBackToFirst(t *testing.T) {
	// A config written by a newer version, or edited by hand, must not select
	// nothing at all - the form has to show something.
	if got := Index("kelvin", "celsius", "fahrenheit"); got != 0 {
		t.Errorf("unknown value gave %d, want the first option", got)
	}
}

func TestPickRoundTripsWithIndex(t *testing.T) {
	opts := []string{"auto", "dark", "light"}
	for _, v := range opts {
		if got := Pick(Index(v, opts...), opts...); got != v {
			t.Errorf("round trip of %q gave %q", v, got)
		}
	}
}

func TestPickClampsOutOfRange(t *testing.T) {
	for _, i := range []int{-1, 3, 99} {
		if got := Pick(i, "a", "b"); got != "a" {
			t.Errorf("Pick(%d) = %q, want the first option", i, got)
		}
	}
}

// TestAppearanceRadioOrderFavoursModern pins the option order the appearance
// group is built with. Modern has to be the FIRST option: index answers 0 for
// anything it does not recognise and pick answers the first option for any index
// out of range, so putting the system look first would make a hand-edited or
// downgraded config file mean "system look" - the opposite of the rule the panel
// applies to the same string.
func TestAppearanceRadioOrderFavoursModern(t *testing.T) {
	opts := []string{"modern", "system"}
	for _, c := range []struct {
		value string
		want  int
	}{
		{"modern", 0}, {"system", 1}, {"", 0}, {"Modern", 0}, {"glass", 0},
	} {
		if got := Index(c.value, opts...); got != c.want {
			t.Errorf("Index(%q) = %d, want %d", c.value, got, c.want)
		}
	}
	if got := Pick(Index("glass", opts...), opts...); got != "modern" {
		t.Errorf("an unrecognised stored look saves as %q, want \"modern\"", got)
	}
}

func TestParseCoordKeepsPreviousOnGarbage(t *testing.T) {
	// An empty or mistyped field must not silently relocate the user.
	for _, s := range []string{"", "abc", "50,45", "--"} {
		if got := ParseCoord(s, 50.4501); got != 50.4501 {
			t.Errorf("ParseCoord(%q) = %v, want the fallback", s, got)
		}
	}
	if got := ParseCoord("-33.8688", 0); got != -33.8688 {
		t.Errorf("parseCoord of a valid negative coordinate gave %v", got)
	}
}

func TestIntervalIndexMatchesLabels(t *testing.T) {
	if len(IntervalLabels()) != len(Intervals) {
		t.Fatal("label list and interval list have drifted apart")
	}
	for i, iv := range Intervals {
		if got := IntervalIndex(iv.Minutes); got != i {
			t.Errorf("IntervalIndex(%d) = %d, want %d", iv.Minutes, got, i)
		}
	}
}

func TestIntervalIndexUnknownValue(t *testing.T) {
	// config.Default() stores 10 minutes, which is not one of the offered
	// options, so the dropdown cannot show it. Saving then silently changes the
	// stored value - inherited from the other backends, which offer the same
	// five choices, but worth pinning so it is a decision rather than a
	// surprise.
	if got := IntervalIndex(10); got != 0 {
		t.Errorf("IntervalIndex(10) = %d, want the first option", got)
	}
}
