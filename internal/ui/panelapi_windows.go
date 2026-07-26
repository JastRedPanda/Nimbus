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

import (
	"runtime"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

var (
	panelUser32 = syscall.NewLazyDLL("user32.dll")

	procUpdateLayeredWindow = panelUser32.NewProc("UpdateLayeredWindow")
	procMonitorFromRect     = panelUser32.NewProc("MonitorFromRect")
)

// dwFlags for UpdateLayeredWindow, from winuser.h. ULW_ALPHA is the only one
// that means "use the per-pixel alpha in the source bitmap".
const ulwAlpha = 0x00000002

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
