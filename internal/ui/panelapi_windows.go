//go:build windows

package ui

// Win32 entry points the forecast panel needs that github.com/lxn/win does not
// export. Verified absent by grepping the module source at
// go list -m -f '{{.Dir}}' github.com/lxn/win.
//
// Every call goes through syscall.SyscallN rather than LazyProc.Call, and it is
// worth saying plainly that this is a house style rather than a safety
// requirement, because this comment used to claim the opposite. BOTH are safe:
// syscall.SyscallN is covered by unsafe.Pointer rule (4), and Proc.Call and
// LazyProc.Call are marked //go:uintptrescapes in the standard library
// (syscall/dll_windows.go), which is the same keepalive guarantee reached a
// different way. darkmode_windows.go uses Proc.Call and is not wrong to.
// runtime.KeepAlive where it appears below is belt and braces, not a fix.
//
// The DLL handles are private to this file on purpose: darkmode_windows.go has
// its own user32DLL/gdi32DLL and this file must not depend on names another
// file owns. LoadLibrary is reference counted, so a second lazy handle to
// user32.dll costs nothing.
//
// One entry here is a NARROWER form of something lxn/win does export -
// adjustWindowRectEx against its AdjustWindowRect - and its comment says why the
// exported one cannot answer the question the panel is asking. That is the only
// justification for a duplicate; "it was easier to write" is not.
//
// What is deliberately NOT here: the panel's drag path needs ReleaseCapture,
// GetWindowRect, WM_NCLBUTTONDOWN and HTCAPTION to start the move loop and
// WM_ENTERSIZEMOVE to notice the loop the caption starts on its own, and
// forceForeground needs GetForegroundWindow. All six are present in the pinned
// lxn/win, so a hand binding would be a second, divergent declaration of
// something that already works. Check the module source before adding to this
// file, the same way the entries below were checked.

import (
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

var (
	panelUser32 = syscall.NewLazyDLL("user32.dll")
	panelGdi32  = syscall.NewLazyDLL("gdi32.dll")

	procMonitorFromRect    = panelUser32.NewProc("MonitorFromRect")
	procAdjustWindowRectEx = panelUser32.NewProc("AdjustWindowRectEx")
	procGetTextFace        = panelGdi32.NewProc("GetTextFaceW")
)

// monitorFromRect wraps HMONITOR MonitorFromRect(LPCRECT lprc, DWORD dwFlags).
//
// Deliberately not MonitorFromPoint, which answers the same question but takes
// POINT BY VALUE - two int32 packed into one register-sized argument on amd64,
// which is a hand-binding trap for no gain. A 1x1 rect at the pointer gives the
// identical answer through a plain pointer parameter.
//
// MonitorFromWindow is in lxn/win but is no use here: the monitor has to be
// known before the window exists, because it decides where the window goes.
func monitorFromRect(rc *win.RECT, flags uint32) win.HMONITOR {
	if err := procMonitorFromRect.Find(); err != nil {
		return 0
	}
	ret, _, _ := syscall.SyscallN(procMonitorFromRect.Addr(),
		uintptr(unsafe.Pointer(rc)), uintptr(flags))
	runtime.KeepAlive(rc)
	return win.HMONITOR(ret)
}

// adjustWindowRectEx wraps
//
//	BOOL AdjustWindowRectEx(LPRECT lpRect, DWORD dwStyle, BOOL bMenu,
//	                        DWORD dwExStyle)
//
// It answers how much larger a window is than the client area it is asked for.
// Given a zeroed rect it returns the frame alone: Right-Left is the width of the
// borders and Bottom-Top the borders plus the caption.
//
// lxn/win has the non-Ex AdjustWindowRect, which darkmode_windows.go's
// frameOverhead uses, and the Ex form is used here anyway: the extended style is
// what decides a window's frame, and a frame computed for a style the window was
// not created with lands as client area below what WM_PAINT draws - a strip that
// the back buffer does not cover and WM_ERASEBKGND refuses to erase, i.e.
// uninitialised memory along the bottom of the panel.
//
// The panel no longer asks for a caption of its own size - WS_EX_TOOLWINDOW and
// its SM_CYSMCAPTION caption are gone, and the caption is the ordinary
// SM_CYCAPTION one - so today the two calls would agree. Keeping the Ex form is
// still right: it answers for whatever the window was actually given, and it will
// go on being right if that ever changes again.
//
// Not AdjustWindowRectExForDpi, which is the per-monitor-DPI form: this process
// declares SYSTEM dpiAwareness in its manifest, so every window it owns is drawn
// at the system DPI and that is the DPI this call already answers for.
func adjustWindowRectEx(rc *win.RECT, style uint32, menu bool, exStyle uint32) bool {
	if err := procAdjustWindowRectEx.Find(); err != nil {
		return false
	}
	var b uintptr
	if menu {
		b = 1
	}
	ret, _, _ := syscall.SyscallN(procAdjustWindowRectEx.Addr(),
		uintptr(unsafe.Pointer(rc)), uintptr(style), b, uintptr(exStyle))
	runtime.KeepAlive(rc)
	return ret != 0
}

// getTextFace wraps int GetTextFaceW(HDC hdc, int c, LPWSTR lpName).
//
// It answers the typeface name of the font currently SELECTED INTO hdc - the
// realised font, not the LOGFONT that was asked for - which is the only way to
// find out that the font mapper substituted something for a face name it could
// not find. c is a count of WCHARs including room for the terminator, and the
// return value is the number of characters copied, or 0 on failure.
//
// The buffer is one WCHAR longer than LF_FACESIZE so that the trailing zero
// make() supplies terminates the string even in the pathological case of a name
// that fills the buffer exactly.
func getTextFace(dc win.HDC) (string, bool) {
	if err := procGetTextFace.Find(); err != nil {
		return "", false
	}
	buf := make([]uint16, win.LF_FACESIZE+1)
	ret, _, _ := syscall.SyscallN(procGetTextFace.Addr(),
		uintptr(dc), uintptr(win.LF_FACESIZE), uintptr(unsafe.Pointer(&buf[0])))
	runtime.KeepAlive(buf)
	if ret == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}
