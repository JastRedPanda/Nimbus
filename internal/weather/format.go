package weather

import (
	"fmt"

	"github.com/JastRedPanda/Nimbus/internal/i18n"
)

// The forecast table's three numeric columns, formatted once for every backend.
//
// They live here, beside the data they format, because THREE backends now draw
// the same table - GTK, Win32 and Qt - and each of them was a candidate for its
// own copy. Duplication here is not a style question: the columns were
// per-backend until a parity audit found the copies drifting, with the
// Fahrenheit suffix appended to Celsius numbers in both of them. A shared pure
// function cannot drift, and - the part that matters on a machine with no
// Windows - it can be tested on Linux, where the maintainer actually is.

// TempRange is the Temp column, e.g. "+24/+17°C".
//
// Open-Meteo is asked for no temperature_unit, so the API always answers in
// Celsius (weather.go:94) and the conversion has to happen here. Appending
// i18n's "°F" without it - which is what both backends did after the table
// landed - labels Celsius numbers as Fahrenheit.
func TempRange(d DailyForecast, units string, l i18n.Lang) string {
	hi, lo := d.TempMax, d.TempMin
	if units == "fahrenheit" {
		hi = hi*9/5 + 32
		lo = lo*9/5 + 32
	}
	return fmt.Sprintf("%+.0f/%+.0f%s", hi, lo, l.TempUnit(units))
}

// WindSpeed is the Wind column, e.g. "3.4 м/с". Open-Meteo is asked for km/h,
// so metres per second is derived here.
func WindSpeed(d DailyForecast, windUnit string, l i18n.Lang) string {
	speed := d.WindMax
	if windUnit == "ms" {
		speed /= 3.6
	}
	return fmt.Sprintf("%.1f %s", speed, l.WindUnitCfg(windUnit))
}

// Precip is the Precip column, e.g. "0.3 мм".
func Precip(d DailyForecast, l i18n.Lang) string {
	return fmt.Sprintf("%.1f %s", d.PrecipSum, l.PrecipUnit())
}
