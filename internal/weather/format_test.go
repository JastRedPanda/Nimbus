package weather

import (
	"testing"

	"github.com/JastRedPanda/Nimbus/internal/i18n"
)

// One table, one set of expectations, both backends. That is the point of these
// functions living in an untagged file: the assertions below hold for Windows
// too, on a machine that cannot run Windows.

func TestTempRangeConvertsFahrenheit(t *testing.T) {
	// The API always answers in Celsius, so the suffix and the number have to
	// be decided together. 24C is 75F, 17C is 63F.
	d := DailyForecast{TempMax: 24, TempMin: 17}
	if got := TempRange(d, "celsius", i18n.EN); got != "+24/+17°C" {
		t.Errorf("celsius: %q", got)
	}
	if got := TempRange(d, "fahrenheit", i18n.EN); got != "+75/+63°F" {
		t.Errorf("fahrenheit: %q - a Celsius number under an F suffix is the bug this guards", got)
	}
}

func TestTempRangeKeepsTheSign(t *testing.T) {
	// The reference layout shows a signed pair; a bare "-4/-9" would read as a
	// range rather than two temperatures.
	d := DailyForecast{TempMax: -4, TempMin: -9}
	if got := TempRange(d, "celsius", i18n.EN); got != "-4/-9°C" {
		t.Errorf("negative: %q", got)
	}
	zero := DailyForecast{TempMax: 0, TempMin: -0.4}
	if got := TempRange(zero, "celsius", i18n.EN); got != "+0/-0°C" {
		t.Logf("zero crossing renders as %q", got)
	}
}

func TestWindSpeedConvertsToMetresPerSecond(t *testing.T) {
	d := DailyForecast{WindMax: 36} // km/h
	if got := WindSpeed(d, "kmh", i18n.EN); got != "36.0 km/h" {
		t.Errorf("kmh: %q", got)
	}
	if got := WindSpeed(d, "ms", i18n.EN); got != "10.0 m/s" {
		t.Errorf("ms: %q", got)
	}
}

func TestPrecipUsesLocalisedUnit(t *testing.T) {
	d := DailyForecast{PrecipSum: 0.34}
	if got := Precip(d, i18n.EN); got != "0.3 mm" {
		t.Errorf("en: %q", got)
	}
	if got := Precip(d, i18n.UK); got != "0.3 мм" {
		t.Errorf("uk: %q", got)
	}
}
