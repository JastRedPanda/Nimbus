package ui

import (
	"fmt"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

// toggleGrace is how long after the panel closed itself a tray click still
// counts as the click that closed it, rather than a fresh request to open.
//
// Clicking the tray icon can take focus away from the panel, and the panel
// closes on focus loss, so the close can land BEFORE the host delivers the
// click. Without this the second click would reopen the panel instead of
// dismissing it. Long enough to cover that ordering, short enough that a
// deliberate reopen a moment later still works.
const toggleGrace = 400 * time.Millisecond

// The forecast table's three numeric columns, formatted once for every backend.
//
// These live in an untagged file on purpose. They were duplicated per backend
// until a parity audit found the copies drifting: the Fahrenheit suffix was
// being appended to Celsius numbers in both of them, and a coordinate parser
// had gained a TrimSpace on one side only. A shared pure function cannot drift,
// and - the part that matters on a machine with no Windows - it can be tested
// on Linux, where the maintainer actually is.

// tempRange is the Temp column, e.g. "+24/+17°C".
//
// Open-Meteo is asked for no temperature_unit, so the API always answers in
// Celsius (weather.go:94) and the conversion has to happen here. Appending
// i18n's "°F" without it - which is what both backends did after the table
// landed - labels Celsius numbers as Fahrenheit.
func tempRange(d weather.DailyForecast, units string, l i18n.Lang) string {
	hi, lo := d.TempMax, d.TempMin
	if units == "fahrenheit" {
		hi = hi*9/5 + 32
		lo = lo*9/5 + 32
	}
	return fmt.Sprintf("%+.0f/%+.0f%s", hi, lo, l.TempUnit(units))
}

// windSpeed is the Wind column, e.g. "3.4 м/с". Open-Meteo is asked for km/h,
// so metres per second is derived here.
func windSpeed(d weather.DailyForecast, windUnit string, l i18n.Lang) string {
	speed := d.WindMax
	if windUnit == "ms" {
		speed /= 3.6
	}
	return fmt.Sprintf("%.1f %s", speed, l.WindUnitCfg(windUnit))
}

// precip is the Precip column, e.g. "0.3 мм".
func precip(d weather.DailyForecast, l i18n.Lang) string {
	return fmt.Sprintf("%.1f %s", d.PrecipSum, l.PrecipUnit())
}
