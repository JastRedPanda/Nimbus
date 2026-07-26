package ui

import "testing"

func TestFormatVersion(t *testing.T) {
	for _, c := range []struct {
		version, date, want string
	}{
		{"1.0.15", "07.2026", "1.0.15 · 07.2026"},
		{"dev", "unknown", "dev"},
		{"dev", "07.2026", "dev"},       // a dated dev build is still a dev build
		{"1.0.15", "unknown", "1.0.15"}, // linked without the date flag
		{"1.0.15", "", "1.0.15"},        // ditto, empty rather than placeholder
		{"", "07.2026", "dev"},          // -X given an empty value
	} {
		if got := formatVersion(c.version, c.date); got != c.want {
			t.Errorf("formatVersion(%q, %q) = %q, want %q", c.version, c.date, got, c.want)
		}
	}
}
