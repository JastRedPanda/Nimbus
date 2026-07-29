//go:build linux

package runonce

import (
	"os"
	"path/filepath"
	"strconv"
)

const appName = "nimbus"

func lockPath() (string, error) {
	// Следуем XDG: $XDG_RUNTIME_DIR предпочтительнее /tmp.
	// lock-файл хранит PID, чтобы можно было проверить, жив ли процесс.
	rd := os.Getenv("XDG_RUNTIME_DIR")
	var dir string
	if rd != "" {
		dir = rd
	} else {
		dir = filepath.Join(os.TempDir(), appName)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, appName+".lock"), nil
}

var lockFile *os.File

func Lock() bool {
	path, err := lockPath()
	if err != nil {
		return true // fallback — разрешаем запуск
	}

	// Пытаемся открыть с O_CREATE|O_EXCL — атомарно.
	lf, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		lockFile = lf
		// Пишем PID для диагностики.
		lf.WriteString(strconv.Itoa(os.Getpid()))
		lf.Sync()
		return true
	}

	// Файл уже существует — проверяем, жив ли процесс.
	old, readErr := os.ReadFile(path)
	if readErr == nil {
		if pid, parseErr := strconv.Atoi(string(old)); parseErr == nil {
			proc, procErr := os.FindProcess(pid)
			if procErr == nil && proc.Signal(os.Signal(nil)) == nil {
				return false
			}
		}
	}

	// Процесс мёртв, удаляем старый lock и пробуем снова.
	os.Remove(path)
	lf, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return true // fallback
	}
	lockFile = lf
	lf.WriteString(strconv.Itoa(os.Getpid()))
	lf.Sync()
	return true
}

func Unlock() {
	if lockFile != nil {
		lockFile.Close()
		os.Remove(lockFile.Name())
		lockFile = nil
	}
}
