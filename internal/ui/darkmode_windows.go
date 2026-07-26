//go:build windows

package ui

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

var (
	dwmDLL                    = syscall.NewLazyDLL("dwmapi.dll")
	dwmSetWindowAttributeProc = dwmDLL.NewProc("DwmSetWindowAttribute")

	gdi32DLL             = syscall.NewLazyDLL("gdi32.dll")
	createSolidBrushProc = gdi32DLL.NewProc("CreateSolidBrush")
	createPenProc        = gdi32DLL.NewProc("CreatePen")

	user32DLL    = syscall.NewLazyDLL("user32.dll")
	fillRectProc = user32DLL.NewProc("FillRect")
)

func createPen(style, width int32, color win.COLORREF) win.HPEN {
	ret, _, _ := createPenProc.Call(uintptr(style), uintptr(width), uintptr(color))
	return win.HPEN(ret)
}

const DWMWA_USE_IMMERSIVE_DARK_MODE = 20

func createDarkBrush() win.HBRUSH {
	ret, _, _ := createSolidBrushProc.Call(uintptr(win.RGB(45, 45, 45)))
	return win.HBRUSH(ret)
}

func createEditBrush() win.HBRUSH {
	ret, _, _ := createSolidBrushProc.Call(uintptr(win.RGB(55, 55, 55)))
	return win.HBRUSH(ret)
}

func setDarkTitleBar(hwnd win.HWND, dark bool) {
	if dwmSetWindowAttributeProc.Find() != nil {
		return
	}
	v := uintptr(0)
	if dark {
		v = 1
	}
	dwmSetWindowAttributeProc.Call(
		uintptr(hwnd),
		DWMWA_USE_IMMERSIVE_DARK_MODE,
		uintptr(unsafe.Pointer(&v)),
		4,
	)
}

// personalizeKey holds the light/dark choice made in Settings >
// Personalisation > Colours. AppsUseLightTheme is the one that governs
// application windows - the neighbouring SystemUsesLightTheme governs the
// taskbar and Start menu, and the two are set independently, so reading the
// wrong one gives the wrong answer on a very common configuration.
const (
	personalizeKey    = `SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	appsUseLightTheme = "AppsUseLightTheme"
)

// resolveDark turns the configured theme into the palette to draw with.
//
// "dark" and "light" are the user overriding the desktop, so they win. "auto"
// means follow the desktop, which is the whole point of the setting and is
// what the GTK backend does by reading the theme's own foreground colour;
// Windows states the preference outright, so here it is simply read.
func resolveDark(theme string) bool {
	switch theme {
	case "dark":
		return true
	case "light":
		return false
	}
	return systemDark()
}

// systemDark reports whether Windows is currently drawing applications dark.
//
// Every failure answers "light": the value is absent on Windows 8 and on the
// early Windows 10 releases that predate the setting, and a machine that
// cannot state a preference is a machine running the light theme.
func systemDark() bool {
	sub, err := syscall.UTF16PtrFromString(personalizeKey)
	if err != nil {
		return false
	}
	var key win.HKEY
	if win.RegOpenKeyEx(win.HKEY_CURRENT_USER, sub, 0, win.KEY_READ, &key) != win.ERROR_SUCCESS {
		return false
	}
	defer win.RegCloseKey(key)

	name, err := syscall.UTF16PtrFromString(appsUseLightTheme)
	if err != nil {
		return false
	}
	var (
		value uint32
		kind  uint32
		size  = uint32(unsafe.Sizeof(value))
	)
	if win.RegQueryValueEx(key, name, nil, &kind, (*byte)(unsafe.Pointer(&value)), &size) != win.ERROR_SUCCESS {
		return false
	}
	if kind != win.REG_DWORD || size != uint32(unsafe.Sizeof(value)) {
		return false
	}
	// 1 is light, 0 is dark. The value names the theme apps should use, not
	// the mode they should switch on, which is why the sense reads inverted.
	return value == 0
}

// baseDPI is what a layout literal in this package means. Windows calls it
// USER_DEFAULT_SCREEN_DPI.
const baseDPI = 96

// minLayoutDPI is the floor for a layout that has been shrunk to fit the
// screen. Below this the text stops being readable, and a window that is
// slightly too tall is a better outcome than one nobody can read.
const minLayoutDPI = 72

// dpiOf reports the DPI a window is drawn at.
//
// GetDpiForWindow arrived in Windows 10 1607; lxn/win's binding falls back on
// its own to GetDeviceCaps(LOGPIXELSY) against the window's DC, which is how
// the same question was asked before that. A process whose manifest declares
// no DPI awareness is told 96 whatever the screen really is - the system
// scales its output afterwards - so every layout below reduces to exactly the
// pixel values it had before the manifest landed.
func dpiOf(hwnd win.HWND) int32 {
	dpi := int32(win.GetDpiForWindow(hwnd))
	if dpi <= 0 {
		return baseDPI
	}
	return dpi
}

// scaleDPI converts a layout value written at 96 DPI into device pixels. This
// is the multiplier Microsoft's own high-DPI guidance prescribes.
func scaleDPI(v int, dpi int32) int32 { return win.MulDiv(int32(v), dpi, baseDPI) }

// frameOverhead is how much wider and taller a window is than its client area
// for a given style - the caption and the borders.
func frameOverhead(style uint32) (w, h int32) {
	var rc win.RECT
	win.AdjustWindowRect(&rc, style, false)
	return rc.Right - rc.Left, rc.Bottom - rc.Top
}

// layoutDPI is the DPI a fixed layout should actually be scaled by.
//
// It starts from the window's real DPI and shrinks it until the window fits
// the monitor's work area. That step is not optional: the settings window is
// 700 units tall, and 700 at 150% is 1050 pixels, which does not fit the
// ~1008 pixel work area of the very ordinary 1080p screen that is set to 150%
// in the first place. Scaling the layout without this check would trade a
// blurry window for one whose buttons are underneath the taskbar.
func layoutDPI(hwnd win.HWND, contentW, contentH int, style uint32) int32 {
	dpi := dpiOf(hwnd)

	mon := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST)
	if mon == 0 {
		return dpi
	}
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if !win.GetMonitorInfo(mon, &mi) {
		return dpi
	}

	ow, oh := frameOverhead(style)
	dpi = fitDPI(dpi, contentW, mi.RcWork.Right-mi.RcWork.Left, ow)
	dpi = fitDPI(dpi, contentH, mi.RcWork.Bottom-mi.RcWork.Top, oh)
	if dpi < minLayoutDPI {
		dpi = minLayoutDPI
	}
	return dpi
}

// fitDPI is the largest DPI at which content units still fit avail pixels once
// the non-client overhead is paid for. The overhead does not scale with the
// layout - it comes from the system's own metrics - so it is subtracted rather
// than divided out.
func fitDPI(dpi int32, content int, avail, overhead int32) int32 {
	if content <= 0 || avail <= overhead {
		return dpi
	}
	if fit := (avail - overhead) * baseDPI / int32(content); fit < dpi {
		return fit
	}
	return dpi
}

// centreOn places a window of the given size in the middle of the work area of
// the monitor it is currently on, clamped so no edge lands off it.
//
// The alternative is CW_USEDEFAULT, which cascades each new window down and to
// the right from the last - fine for a 200 pixel window, wrong for a dialog
// that is nearly as tall as the work area, because the cascade offset is
// enough to push its buttons off the bottom.
func centreOn(hwnd win.HWND, w, h int32) {
	mon := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST)
	if mon == 0 {
		win.SetWindowPos(hwnd, 0, 0, 0, w, h, win.SWP_NOMOVE|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
		return
	}
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if !win.GetMonitorInfo(mon, &mi) {
		win.SetWindowPos(hwnd, 0, 0, 0, w, h, win.SWP_NOMOVE|win.SWP_NOZORDER|win.SWP_NOACTIVATE)
		return
	}

	x := mi.RcWork.Left + (mi.RcWork.Right-mi.RcWork.Left-w)/2
	y := mi.RcWork.Top + (mi.RcWork.Bottom-mi.RcWork.Top-h)/2
	if x < mi.RcWork.Left {
		x = mi.RcWork.Left
	}
	if y < mi.RcWork.Top {
		y = mi.RcWork.Top
	}
	win.SetWindowPos(hwnd, 0, x, y, w, h, win.SWP_NOZORDER|win.SWP_NOACTIVATE)
}

// uiFontPt is the shell's text size. Windows has drawn its own dialogs at 9pt
// since Vista.
const uiFontPt = 9

// uiFont builds the font child controls should be drawn in at this DPI.
//
// A control that is never sent WM_SETFONT draws in SYSTEM_FONT, the bitmap
// face from Windows 3.1, which is why an untouched Win32 window looks nothing
// like the desktop around it. The face is taken from the user's own message
// font so a locale whose script the default face cannot render still gets
// readable text; only the size has to follow the DPI, and it is stated in
// points precisely so that it can.
func uiFont(dpi int32) win.HFONT {
	lf := win.LOGFONT{
		LfHeight:         -win.MulDiv(uiFontPt, dpi, 72),
		LfWeight:         win.FW_NORMAL,
		LfCharSet:        win.DEFAULT_CHARSET,
		LfOutPrecision:   win.OUT_DEFAULT_PRECIS,
		LfClipPrecision:  win.CLIP_DEFAULT_PRECIS,
		LfQuality:        win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	setFaceName(&lf, shellFontFace())
	return win.CreateFontIndirect(&lf)
}

// setFaceName copies a face into a LOGFONT, keeping room for the terminator
// that GDI needs to find the end of the name.
func setFaceName(lf *win.LOGFONT, face string) {
	name := syscall.StringToUTF16(face)
	if len(name) > len(lf.LfFaceName) {
		name = name[:len(lf.LfFaceName)]
		name[len(name)-1] = 0
	}
	copy(lf.LfFaceName[:], name)
}

// shellFontFace is the face Windows uses for message text - Segoe UI on a
// western install, something that can draw the script on others.
//
// lxn/win's NONCLIENTMETRICS predates iPaddedBorderWidth, so cbSize comes out
// four bytes short of the current definition. That is the size Microsoft
// documents for the same struct on XP, and it is the size every application
// built against the older headers still passes, which is why the system
// accepts both. If it ever does not, the fallback below is the answer.
func shellFontFace() string {
	var ncm win.NONCLIENTMETRICS
	ncm.CbSize = uint32(unsafe.Sizeof(ncm))
	if win.SystemParametersInfo(win.SPI_GETNONCLIENTMETRICS, ncm.CbSize, unsafe.Pointer(&ncm), 0) {
		if face := syscall.UTF16ToString(ncm.LfMessageFont.LfFaceName[:]); face != "" {
			return face
		}
	}
	return "Segoe UI"
}

func fillRect(hdc win.HDC, rc *win.RECT, brush win.HBRUSH) {
	fillRectProc.Call(uintptr(hdc), uintptr(unsafe.Pointer(rc)), uintptr(brush))
}

func eraseBg(hwnd win.HWND, wParam uintptr, brush win.HBRUSH) {
	var rc win.RECT
	win.GetClientRect(hwnd, &rc)
	fillRect(win.HDC(wParam), &rc, brush)
}

// handleCtlColor answers a WM_CTLCOLOR* message for a dark window. It returns
// 0 when it has nothing to say, and the caller must then fall through to
// DefWindowProc: returning a null brush from these messages is not "no
// opinion", it is an invalid answer.
func handleCtlColor(hwnd win.HWND, wParam, lParam uintptr, dark bool, darkBrush, editBrush win.HBRUSH) uintptr {
	if !dark {
		return 0
	}
	_ = hwnd
	hdc := win.HDC(wParam)
	hChild := win.HWND(lParam)

	buf := make([]uint16, 32)
	win.GetClassName(hChild, &buf[0], 32)
	cls := syscall.UTF16ToString(buf)

	switch cls {
	case "Edit":
		win.SetTextColor(hdc, win.COLORREF(0x00FFFFFF))
		win.SetBkColor(hdc, win.RGB(55, 55, 55))
		return uintptr(editBrush)
	case "ComboBox", "ListBox":
		win.SetTextColor(hdc, win.COLORREF(0x00FFFFFF))
		win.SetBkColor(hdc, win.RGB(45, 45, 45))
		return uintptr(darkBrush)
	case "Static", "Button", "msctls_trackbar32":
		// A trackbar asks its parent for a background brush with
		// WM_CTLCOLORSTATIC, which is the only way to colour it: the control
		// has no message of its own for it.
		win.SetTextColor(hdc, win.COLORREF(0x00FFFFFF))
		win.SetBkMode(hdc, win.TRANSPARENT)
		return uintptr(darkBrush)
	}
	return 0
}
