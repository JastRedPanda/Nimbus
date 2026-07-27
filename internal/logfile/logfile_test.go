package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The limit is exercised at a size a test can reach in a few writes. maxSize
// itself is not the thing under test - the bookkeeping around it is.
const testLimit = 200

func newTestRotator(t *testing.T) (*rotator, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nimbus.log")
	r, err := newRotator(path, testLimit)
	if err != nil {
		t.Fatalf("newRotator: %v", err)
	}
	return r, path
}

func size(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return fi.Size()
}

// TestWriteRestartsWithinTheSession is the whole point of the type: the file used
// to be checked only when it was opened, so a process that never restarted grew
// it without limit.
func TestWriteRestartsWithinTheSession(t *testing.T) {
	r, path := newTestRotator(t)

	line := strings.Repeat("x", 60) + "\n"
	for i := 0; i < 20; i++ {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
		if got := size(t, path); got > testLimit {
			t.Fatalf("after write %d the file is %d bytes, over the %d limit", i, got, testLimit)
		}
	}

	// And it really did restart rather than stop writing: the newest line has to
	// be there, and the oldest must not be.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "log restarted") {
		t.Error("no restart marker in the file, so nothing rotated")
	}
	if n := strings.Count(string(data), line); n == 0 {
		t.Error("the last lines written are not in the file")
	}
}

// TestWriteKeepsTheNewestLine pins the direction of the trade: what survives a
// restart is the newest content, because that is what a fault report needs.
func TestWriteKeepsTheNewestLine(t *testing.T) {
	r, path := newTestRotator(t)

	if _, err := r.Write([]byte(strings.Repeat("o", int(testLimit)) + "\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	newest := "the newest line\n"
	if _, err := r.Write([]byte(newest)); err != nil {
		t.Fatalf("write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasSuffix(string(data), newest) {
		t.Errorf("file does not end with the newest line, got %q", string(data))
	}
	if strings.Contains(string(data), "oooo") {
		t.Error("the oversized older content survived the restart")
	}
}

// TestOpenDiscardsAnOversizedFile keeps the behaviour Open always had: a log left
// over the limit by a previous session is not appended to.
func TestOpenDiscardsAnOversizedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nimbus.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("y", testLimit*2)), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r, err := newRotator(path, testLimit)
	if err != nil {
		t.Fatalf("newRotator: %v", err)
	}
	if r.size != 0 {
		t.Errorf("size = %d after opening an oversized log, want 0", r.size)
	}
	if got := size(t, path); got != 0 {
		t.Errorf("file is %d bytes after opening an oversized log, want 0", got)
	}
}

// TestOpenAppendsToASmallFile is the other half: a session that stayed under the
// limit keeps its tail, which is what makes the log useful across a restart.
func TestOpenAppendsToASmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nimbus.log")
	old := "from the previous session\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r, err := newRotator(path, testLimit)
	if err != nil {
		t.Fatalf("newRotator: %v", err)
	}
	if r.size != int64(len(old)) {
		t.Errorf("size = %d, want %d - the tracked size must match what is on disk or the limit drifts", r.size, len(old))
	}
	if _, err := r.Write([]byte("new\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.HasPrefix(string(data), old) {
		t.Error("the previous session's tail was discarded")
	}
}
