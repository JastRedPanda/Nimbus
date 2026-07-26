// Package logfile points the standard logger at a file next to the config.
//
// Nimbus is linked as a GUI application and closes its console at startup, so
// os.Stderr is an invalid handle for the whole life of the process and every
// log line the code writes goes nowhere. That is fine until something fails on
// a machine the developer cannot reach, at which point "the menu item does
// nothing" is the entire bug report.
package logfile

import (
	"log"
	"os"
	"path/filepath"
)

// maxSize is the point at which the log is started again. A weather applet
// writes a handful of lines a day; anything past this is a fault repeating,
// and keeping the newest of it matters more than keeping all of it.
const maxSize = 1 << 20

// Open redirects the standard logger to <config dir>/Nimbus/nimbus.log and
// reports the path, or an empty string when no log could be opened - in which
// case logging stays where it was and nothing else changes.
func Open() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "Nimbus")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	path := filepath.Join(dir, "nimbus.log")

	if fi, err := os.Stat(path); err == nil && fi.Size() > maxSize {
		os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return ""
	}
	// Deliberately never closed: it has to outlive main and stay writable for
	// the tray's whole session.
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
	return path
}
