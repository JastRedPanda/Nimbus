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
	"sync"
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

	w, err := newRotator(path, maxSize)
	if err != nil {
		return ""
	}
	// The file inside is deliberately never closed: it has to outlive main and
	// stay writable for the tray's whole session.
	log.SetOutput(w)
	log.SetFlags(log.LstdFlags)
	return path
}

// rotator is the log file plus the size bookkeeping that keeps it bounded.
//
// The size used to be checked once, in Open, which bounded the file across
// restarts and not at all within a session. That is the wrong way round for this
// program: a tray applet is started once and left running for weeks, so the
// session that matters is the one that never ends. Anything that repeats - a
// weather service answering 503 every few minutes, a window that logs each time
// it opens - grew the file without limit, and the check at startup would only
// notice after the user had restarted the app, which is exactly when they no
// longer need the log.
type rotator struct {
	// mu makes a restart atomic against a concurrent write. The standard logger
	// serialises its own calls, but log.SetOutput hands this writer out and
	// nothing guarantees it stays the only caller.
	mu    sync.Mutex
	path  string
	limit int64
	f     *os.File
	// size is tracked rather than re-stat'ed: a syscall per log line for a number
	// this side already knows.
	size int64
}

func newRotator(path string, limit int64) (*rotator, error) {
	r := &rotator{path: path, limit: limit}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

// open attaches to the file, discarding what is there if it is already over the
// limit. Append rather than truncate otherwise, so a session that stays under the
// limit keeps the previous one's tail for context.
func (r *rotator) open() error {
	if fi, err := os.Stat(r.path); err == nil && fi.Size() > r.limit {
		os.Remove(r.path)
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	size := int64(0)
	if fi, err := f.Stat(); err == nil {
		size = fi.Size()
	}
	r.f, r.size = f, size
	return nil
}

func (r *rotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Checked before the write, not after, so the limit is a ceiling on the file
	// rather than a line the last write is allowed to cross.
	if r.size+int64(len(p)) > r.limit {
		if err := r.restart(); err != nil {
			// Nowhere to report this: logging it would recurse straight back into
			// here. The line goes to the oversized file instead, because a log that
			// is too big beats a line that is lost.
			n, werr := r.f.Write(p)
			r.size += int64(n)
			return n, werr
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

// restart empties the file in place rather than renaming it aside. There is one
// log and no rotation scheme to name generations in: what a fault report needs is
// the newest lines, and a second file that also has to be found and sent is worse
// than none. Truncating rather than reopening keeps the same inode, so anything
// tailing the file follows it across the restart.
func (r *rotator) restart() error {
	if _, err := r.f.Seek(0, 0); err != nil {
		return err
	}
	if err := r.f.Truncate(0); err != nil {
		return err
	}
	r.size = 0
	// Written straight to the file: going through log would deadlock on mu.
	n, _ := r.f.WriteString("--- log restarted: the previous contents reached the size limit ---\n")
	r.size = int64(n)
	return nil
}
