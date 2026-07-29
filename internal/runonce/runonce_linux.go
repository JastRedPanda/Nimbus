//go:build linux

package runonce

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const appName = "nimbus"

func lockPath() string {
	return filepath.Join(os.TempDir(), appName+".lock")
}

var lockFile *os.File

func Lock() bool {
	path := lockPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return true
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		return false
	}

	lockFile = f
	f.Truncate(0)
	f.WriteString(strconv.Itoa(os.Getpid()))
	f.Sync()
	return true
}

func Unlock() {
	if lockFile != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		os.Remove(lockFile.Name())
		lockFile = nil
	}
}
