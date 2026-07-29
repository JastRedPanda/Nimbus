//go:build windows

package runonce

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const appName = "nimbus"

func lockPath() string {
	return filepath.Join(os.TempDir(), appName+".lock")
}

var lockFile *os.File

func Lock() bool {
	path := lockPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		lockFile = f
		f.WriteString(strconv.Itoa(os.Getpid()))
		f.Sync()
		return true
	}

	data, readErr := os.ReadFile(path)
	if readErr == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil {
			proc, procErr := os.FindProcess(pid)
			if procErr == nil && proc.Signal(os.Signal(nil)) == nil {
				return false
			}
		}
	}

	os.Remove(path)
	f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return true
	}
	lockFile = f
	f.WriteString(strconv.Itoa(os.Getpid()))
	f.Sync()
	return true
}

func Unlock() {
	if lockFile != nil {
		lockFile.Close()
		os.Remove(lockFile.Name())
		lockFile = nil
	}
}
