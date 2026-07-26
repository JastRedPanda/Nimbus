//go:build windows

package fonts

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/lxn/win"
)

var loaded bool
var tempPath string

// Load registers the embedded typeface with GDI so CreateFontIndirect can ask
// for it by family name. Only the Win32 backend needs this; other platforms
// rasterise the glyphs directly with Glyph.
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
