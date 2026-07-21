package fonts

import (
	_ "embed"
	"os"
	"path/filepath"
	"syscall"

	"github.com/lxn/win"
)

//go:embed weathericons.ttf
var weatherIconsTTF []byte

var loaded bool
var tempPath string

func Load() bool {
	if loaded {
		return true
	}
	tempPath = filepath.Join(os.TempDir(), "nimbus-weathericons.ttf")
	err := os.WriteFile(tempPath, weatherIconsTTF, 0644)
	if err != nil {
		return false
	}
	fn, _ := syscall.UTF16PtrFromString(tempPath)
	ret := win.AddFontResourceEx(fn, win.FR_PRIVATE, nil)
	loaded = ret > 0
	return loaded
}

func Cleanup() {
	if tempPath != "" {
		fn, _ := syscall.UTF16PtrFromString(tempPath)
		win.RemoveFontResourceEx(fn, win.FR_PRIVATE, nil)
		os.Remove(tempPath)
		tempPath = ""
		loaded = false
	}
}

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
