//go:build windows

package ui

// Day and month names for the forecast panel.
//
// This is a verbatim copy of dates_linux.go. Go's time package formats dates in
// English only, so the card layout carries its own tables, and both backends now
// draw the same card layout and need the same tables. The duplication is
// deliberate rather than accidental: dates_linux.go carries a //go:build linux
// tag and belongs to the GTK backend, and merging the two means dropping that
// tag and renaming the file to dates.go - a change to a file this backend does
// not own. Do that, and delete this file, the next time both are touched
// together.

import (
	"fmt"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/i18n"
)

var weekdayLong = map[i18n.Lang][7]string{
	i18n.UK: {"Неділя", "Понеділок", "Вівторок", "Середа", "Четвер", "Пʼятниця", "Субота"},
	i18n.EN: {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
}

var weekdayShort = map[i18n.Lang][7]string{
	i18n.UK: {"нд", "пн", "вт", "ср", "чт", "пт", "сб"},
	i18n.EN: {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
}

// Ukrainian dates take the genitive: "26 липня", not "26 липень".
var monthGenitive = map[i18n.Lang][12]string{
	i18n.UK: {"січня", "лютого", "березня", "квітня", "травня", "червня",
		"липня", "серпня", "вересня", "жовтня", "листопада", "грудня"},
	i18n.EN: {"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"},
}

func table[T any](m map[i18n.Lang]T, l i18n.Lang) T {
	if v, ok := m[l]; ok {
		return v
	}
	return m[i18n.EN]
}

// parseDay reads the ISO date Open-Meteo returns. A malformed value yields the
// zero time, and callers fall back to showing the raw string.
func parseDay(iso string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", iso)
	return t, err == nil
}

// headerDate renders the long form for the header, e.g. "Неділя, 26 липня".
func headerDate(iso string, l i18n.Lang) string {
	t, ok := parseDay(iso)
	if !ok {
		return iso
	}
	names := table(weekdayLong, l)
	months := table(monthGenitive, l)
	return fmt.Sprintf("%s, %d %s", names[int(t.Weekday())], t.Day(), months[int(t.Month())-1])
}

// shortDay renders the day-card caption, e.g. "нд".
func shortDay(iso string, l i18n.Lang) string {
	t, ok := parseDay(iso)
	if !ok {
		return iso
	}
	return table(weekdayShort, l)[int(t.Weekday())]
}
