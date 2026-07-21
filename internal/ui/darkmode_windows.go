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

	gdi32DLL           = syscall.NewLazyDLL("gdi32.dll")
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

func fillRect(hdc win.HDC, rc *win.RECT, brush win.HBRUSH) {
	fillRectProc.Call(uintptr(hdc), uintptr(unsafe.Pointer(rc)), uintptr(brush))
}

func eraseBg(hwnd win.HWND, wParam uintptr, brush win.HBRUSH) {
	var rc win.RECT
	win.GetClientRect(hwnd, &rc)
	fillRect(win.HDC(wParam), &rc, brush)
}

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
	case "ListBox":
		win.SetTextColor(hdc, win.COLORREF(0x00FFFFFF))
		win.SetBkColor(hdc, win.RGB(45, 45, 45))
		return uintptr(darkBrush)
	case "Static", "Button":
		win.SetTextColor(hdc, win.COLORREF(0x00FFFFFF))
		win.SetBkMode(hdc, win.TRANSPARENT)
		return uintptr(darkBrush)
	}
	return 0
}
