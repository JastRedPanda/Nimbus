//go:build windows

package ui

// Win32 entry points the forecast panel needs that github.com/lxn/win does not
// export. Verified absent by grepping the module source at
// go list -m -f '{{.Dir}}' github.com/lxn/win.
//
// Every call goes through syscall.SyscallN rather than LazyProc.Call. The
// unsafe.Pointer rules give a pointer-to-uintptr conversion its keepalive
// guarantee only inside the argument list of an ASSEMBLY-implemented function;
// Proc.Call is ordinary Go, so the pointer it was handed is not kept alive for
// the duration of the call. runtime.KeepAlive belts the braces.
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
// GetWindowRect, WM_NCLBUTTONDOWN and HTCAPTION to start the move loop,
// WM_ENTERSIZEMOVE and WM_EXITSIZEMOVE to know that the loop is running and
// GetForegroundWindow to settle a deactivation that arrived while it was, and the
// settings dialog's checkbox needs BS_AUTOCHECKBOX. All eight are present in the
// pinned lxn/win, so a hand binding would be a second, divergent declaration of
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

	procUpdateLayeredWindow = panelUser32.NewProc("UpdateLayeredWindow")
	procMonitorFromRect     = panelUser32.NewProc("MonitorFromRect")
	procAdjustWindowRectEx  = panelUser32.NewProc("AdjustWindowRectEx")
	procGetTextFace         = panelGdi32.NewProc("GetTextFaceW")
)

// dwFlags for UpdateLayeredWindow, from winuser.h.
//
// ULW_ALPHA is the only one that means "use the per-pixel alpha in the source
// bitmap"; ULW_OPAQUE, "draw an opaque layered window", puts the same bitmap up
// with the alpha channel ignored, and is what the panel asks for on a display
// too shallow to composite alpha - see perPixelAlpha. ULW_COLORKEY, the third,
// is not wanted here: it makes one exact RGB value transparent, which cannot
// express an antialiased edge.
//
// Note that ULW_ALPHA degrades to ULW_OPAQUE by itself on such a display,
// silently and successfully, which is precisely why the panel decides which
// palette to draw BEFORE it calls this rather than reading the return value.
const (
	ulwAlpha  = 0x00000002
	ulwOpaque = 0x00000004
)

// BLENDFUNCTION.BlendOp. lxn/win declares AC_SRC_ALPHA but not AC_SRC_OVER,
// and about_windows.go has its own copy under a different name; this one is
// deliberately independent so neither file can break the other.
const blendOpSrcOver = 0x00

// updateLayeredWindow wraps
//
//	BOOL UpdateLayeredWindow(HWND hWnd, HDC hdcDst, POINT *pptDst, SIZE *psize,
//	                         HDC hdcSrc, POINT *pptSrc, COLORREF crKey,
//	                         BLENDFUNCTION *pblend, DWORD dwFlags)
//
// It moves, resizes and repaints the window in one call: pptDst is the new
// position in SCREEN coordinates, psize the new size, pptSrc the origin within
// the source DC. hdcDst may be 0, which asks for the default palette.
//
// This is the only way to get per-pixel alpha onto a window.
// SetLayeredWindowAttributes is the trap next door: its bAlpha is "similar to
// the SourceConstantAlpha member of BLENDFUNCTION", i.e. one opacity for the
// whole window including the text, and calling it even once permanently breaks
// UpdateLayeredWindow for that window until WS_EX_LAYERED is cleared and set
// again.
func updateLayeredWindow(
	hwnd win.HWND, hdcDst win.HDC, ptDst *win.POINT, size *win.SIZE,
	hdcSrc win.HDC, ptSrc *win.POINT, crKey win.COLORREF,
	blend *win.BLENDFUNCTION, flags uint32,
) (bool, syscall.Errno) {
	if err := procUpdateLayeredWindow.Find(); err != nil {
		return false, syscall.EINVAL
	}
	ret, _, errno := syscall.SyscallN(procUpdateLayeredWindow.Addr(),
		uintptr(hwnd),
		uintptr(hdcDst),
		uintptr(unsafe.Pointer(ptDst)),
		uintptr(unsafe.Pointer(size)),
		uintptr(hdcSrc),
		uintptr(unsafe.Pointer(ptSrc)),
		uintptr(crKey),
		uintptr(unsafe.Pointer(blend)),
		uintptr(flags),
	)
	runtime.KeepAlive(ptDst)
	runtime.KeepAlive(size)
	runtime.KeepAlive(ptSrc)
	runtime.KeepAlive(blend)
	return ret != 0, errno
}

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
// not created with lands as client area below the composed image - a strip that
// WM_PAINT does not cover and WM_ERASEBKGND refuses to erase, i.e. uninitialised
// memory along the bottom of the panel.
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
