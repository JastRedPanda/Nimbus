//go:build windows

package runonce

import (
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW    = kernel32.NewProc("CreateMutexW")
	procReleaseMutex    = kernel32.NewProc("ReleaseMutex")
	procCloseHandle     = kernel32.NewProc("CloseHandle")
	mu                  uintptr
	mutexName           = syscall.StringToUTF16Ptr("Local\\Nimbus-{7a3c1b0e-1d4a-4e2f-8c9d-0a1b2c3d4e5f}")
)

func Lock() bool {
	ret, _, err := procCreateMutexW.Call(
		0,  // lpMutexAttributes
		1,  // bInitialOwner — true
		uintptr(unsafe.Pointer(mutexName)),
	)
	mu = ret
	if mu == 0 {
		return true
	}
	if err == syscall.Errno(183) { // ERROR_ALREADY_EXISTS
		procCloseHandle.Call(mu)
		mu = 0
		return false
	}
	return true
}

func Unlock() {
	if mu != 0 {
		procReleaseMutex.Call(mu)
		procCloseHandle.Call(mu)
		mu = 0
	}
}
