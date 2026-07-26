// Package fonts owns the embedded Weather Icons typeface and turns its
// codepoints into weather symbols.
//
// Only the Win32 backend uses it: register_windows.go hands the face to GDI and
// forecast_windows.go draws the codepoints below. The GTK backend draws colour
// artwork from internal/wicons instead and never touches this package.
package fonts

import _ "embed"

//go:embed weathericons.ttf
var weatherIconsTTF []byte

const (
	WiDaySunny     = "\uf00d"
	WiDayCloudy    = "\uf002"
	WiCloud        = "\uf041"
	WiFog          = "\uf014"
	WiRain         = "\uf019"
	WiShowers      = "\uf01a"
	WiSnow         = "\uf01b"
	WiThunderstorm = "\uf01e"
)

func IconForCode(code int) string {
	switch {
	case code == 0:
		return WiDaySunny
	case code <= 2:
		return WiDayCloudy
	case code <= 3:
		return WiCloud
	case code >= 45 && code <= 48:
		return WiFog
	case code >= 51 && code <= 57:
		return WiRain
	case code >= 61 && code <= 65:
		return WiShowers
	case code >= 71 && code <= 77:
		return WiSnow
	case code >= 80 && code <= 86:
		return WiShowers
	case code >= 95:
		return WiThunderstorm
	default:
		return WiCloud
	}
}
