package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// redirectConfigDir points the package at a throwaway directory for the rest of
// the test. configDir goes through os.UserConfigDir, which on Linux returns
// $XDG_CONFIG_HOME when it is set, so overriding that variable is what keeps
// these tests off the developer's own ~/.config/Nimbus/config.json.
//
// Other platforms ignore XDG_CONFIG_HOME entirely, so the test skips there
// rather than writing to a real config directory, and on Linux it hard-fails if
// the resolved path ever escapes the temporary directory - that check is what
// turns a future rewrite of configDir into a test failure instead of a silently
// clobbered user config.
func redirectConfigDir(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("config directory can only be redirected via XDG_CONFIG_HOME on Linux")
	}
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	if !strings.HasPrefix(path, dir+string(os.PathSeparator)) {
		t.Fatalf("config path %q is outside the test directory %q; refusing to touch the real config", path, dir)
	}
	return dir
}

// TestConfigDirIsRedirected asserts the guarantee the rest of this file relies
// on, rather than leaving it implicit in a helper: every path Save and Delete
// touch resolves under the throwaway directory, so no test here can write to the
// developer's own ~/.config/Nimbus/config.json.
func TestConfigDirIsRedirected(t *testing.T) {
	dir := redirectConfigDir(t)

	got, err := configDir()
	if err != nil {
		t.Fatalf("configDir: %v", err)
	}
	if got != filepath.Join(dir, "Nimbus") {
		t.Fatalf("configDir() = %q, want %q", got, filepath.Join(dir, "Nimbus"))
	}

	// Save is the operation that would do the damage, so check where it actually
	// landed rather than trusting the path computation alone.
	if err := Default().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(got, "config.json")); err != nil {
		t.Fatalf("Save did not write inside the test directory: %v", err)
	}
}

func TestDefaultForecastPanel(t *testing.T) {
	cfg := Default()
	if !cfg.ForecastPinned {
		t.Error("Default().ForecastPinned = false, want true")
	}
	if cfg.ForecastX != nil || cfg.ForecastY != nil {
		t.Errorf("Default() position = (%v, %v), want (nil, nil)", cfg.ForecastX, cfg.ForecastY)
	}
}

// TestForecastPinnedFalseRoundTrip is the regression test for the omitempty
// trap: with omitempty on the tag, false is dropped from the file and Load
// returns the true from Default(), so unchecking the box never sticks.
func TestForecastPinnedFalseRoundTrip(t *testing.T) {
	redirectConfigDir(t)

	cfg := Default()
	cfg.ForecastPinned = false
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"forecast_pinned"`) {
		t.Errorf("saved config has no forecast_pinned key, so false cannot survive Load:\n%s", data)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ForecastPinned {
		t.Error("ForecastPinned = true after saving false")
	}
}

func TestLoadConfigWithoutForecastKeys(t *testing.T) {
	dir := redirectConfigDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "Nimbus"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	// A file written by a version that predates the forecast panel settings.
	old := `{"latitude":50.4501,"longitude":30.5234,"city_name":"Kyiv","update_interval":10,` +
		`"units":"celsius","pressure_unit":"hpa","wind_unit":"ms","icon_theme":"auto",` +
		`"language":"en","font_scale":100}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ForecastPinned {
		t.Error("ForecastPinned = false for a config that predates the field, want the true from Default()")
	}
	if cfg.ForecastX != nil || cfg.ForecastY != nil {
		t.Errorf("position = (%v, %v) for a config that predates the field, want (nil, nil)", cfg.ForecastX, cfg.ForecastY)
	}
}

func TestDefaultAppearanceIsModern(t *testing.T) {
	// Modern is today's panel, so a fresh config must ask for it: an empty string
	// here would reach the backends as "no look chosen" and rely on every one of
	// them treating that as modern, instead of the config saying which it is.
	if got := Default().Appearance; got != "modern" {
		t.Errorf("Default().Appearance = %q, want \"modern\"", got)
	}
}

// TestAppearanceSystemRoundTrip pins the half that a struct tag can break: the
// non-default value has to reach the disk and come back. Load unmarshals over
// Default(), so anything that stops "system" being written - an omitempty added
// later, a renamed key - shows up as the panel quietly reverting to Modern on
// the next start, with nothing in the settings window to explain it.
func TestAppearanceSystemRoundTrip(t *testing.T) {
	redirectConfigDir(t)

	cfg := Default()
	cfg.Appearance = "system"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"appearance"`) {
		t.Errorf("saved config has no appearance key:\n%s", data)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Appearance != "system" {
		t.Errorf("Appearance = %q after saving \"system\"", got.Appearance)
	}
}

// TestLoadConfigWithoutAppearanceLoadsModern covers the upgrade path: a file
// written before the option existed has no appearance key, and the panel must
// look exactly as it did before rather than switching to the system look because
// the field arrived empty.
func TestLoadConfigWithoutAppearanceLoadsModern(t *testing.T) {
	dir := redirectConfigDir(t)
	if err := os.MkdirAll(filepath.Join(dir, "Nimbus"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	old := `{"latitude":50.4501,"longitude":30.5234,"city_name":"Kyiv","update_interval":10,` +
		`"units":"celsius","pressure_unit":"hpa","wind_unit":"ms","icon_theme":"auto",` +
		`"language":"en","font_scale":100,"forecast_pinned":true}`
	if err := os.WriteFile(path, []byte(old), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Appearance != "modern" {
		t.Errorf("Appearance = %q for a config that predates the field, want the \"modern\" from Default()", cfg.Appearance)
	}
}

func TestForecastPositionRoundTrip(t *testing.T) {
	redirectConfigDir(t)

	// Negative coordinates are legitimate on a multi-monitor layout and are the
	// reason the fields are pointers rather than plain ints with a sentinel.
	x, y := -120, 0
	cfg := Default()
	cfg.ForecastX, cfg.ForecastY = &x, &y
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ForecastX == nil || got.ForecastY == nil {
		t.Fatalf("position = (%v, %v), want (-120, 0)", got.ForecastX, got.ForecastY)
	}
	if *got.ForecastX != x || *got.ForecastY != y {
		t.Errorf("position = (%d, %d), want (%d, %d)", *got.ForecastX, *got.ForecastY, x, y)
	}
}

// TestSaveIsAtomic covers what the rename in Save is for. Two writers exist now
// - a Save click and a pinned panel close from a detached goroutine - and a
// truncated config file makes main.go log.Fatalf, so the app stops starting.
//
// The temp-file assertion is the part that would catch a rewrite that forgets to
// rename or to clean up: a leftover config.json.tmp* is invisible to Load and so
// would never show up as a behaviour bug.
func TestSaveIsAtomic(t *testing.T) {
	redirectConfigDir(t)

	cfg := Default()
	cfg.CityName = "Lviv"
	x, y := 640, 480
	cfg.ForecastX, cfg.ForecastY = &x, &y
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Twice, because the second Save is the one that renames over an existing
	// file rather than creating one.
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save (over existing file): %v", err)
	}

	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != filepath.Base(path) {
		t.Errorf("config directory contains %v, want only %q - Save left a temp file behind", names, filepath.Base(path))
	}

	// Complete and parseable, not merely present: a torn file usually still
	// unmarshals up to the point it was cut off.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved config is not valid JSON: %v\n%s", err, data)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.CityName != "Lviv" || got.ForecastX == nil || *got.ForecastX != x || got.ForecastY == nil || *got.ForecastY != y {
		t.Errorf("Load = city %q position (%v, %v), want \"Lviv\" (640, 480)", got.CityName, got.ForecastX, got.ForecastY)
	}
}

// TestSaveKeepsFileMode checks that Save does not hand the user's config the
// 0600 that os.CreateTemp gives its temp file, nor widen a mode the user
// tightened themselves.
func TestSaveKeepsFileMode(t *testing.T) {
	redirectConfigDir(t)

	if err := Default().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path, err := configPath()
	if err != nil {
		t.Fatalf("configPath: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// 0600, not 0644: this file holds the user's home coordinates, and the log file
	// written beside it has always been 0600.
	if fi.Mode().Perm() != 0600 {
		t.Errorf("new config mode = %v, want 0600 - the file holds the user's coordinates", fi.Mode().Perm())
	}

	// And a mode the user chose is preserved rather than reimposed. 0644 makes the
	// assertion meaningful now that 0600 is also the default.
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := Default().Save(); err != nil {
		t.Fatalf("Save (after chmod): %v", err)
	}
	fi, err = os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Mode().Perm() != 0644 {
		t.Errorf("config mode = %v after re-saving a 0644 file, want 0644 - an existing mode must survive", fi.Mode().Perm())
	}
}

// TestResetsCountsOnlyDeletes pins the signal the tray hangs both halves of
// "Delete configuration must stay deleted" off. It has to move on a delete and
// must NOT move on a save, because the tray reads it as "the configuration was
// discarded" and a save that happens to leave no file - a full disk, a read-only
// directory - used to read as a reset and silently threw away the panel position
// the user had chosen.
func TestResetsCountsOnlyDeletes(t *testing.T) {
	redirectConfigDir(t)

	start := Resets()
	cfg := Default()
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := Resets(); got != start {
		t.Errorf("Resets() moved on a Save: %d -> %d", start, got)
	}
	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := Resets(); got != start+1 {
		t.Errorf("Resets() = %d after Delete, want %d", got, start+1)
	}
	// A second delete of a file that is already gone still counts: the user pressed
	// the button, and what the counter records is the intent, not the syscall.
	_ = Delete()
	if got := Resets(); got != start+2 {
		t.Errorf("Resets() = %d after deleting an absent file, want %d", got, start+2)
	}
}

// TestDeleteDiscardsTheRememberedPosition is the regression test for the bug the
// counter exists to prevent: a config that had been dragged is deleted, and
// nothing must be able to bring those coordinates back.
func TestDeleteDiscardsTheRememberedPosition(t *testing.T) {
	redirectConfigDir(t)

	x, y := 1500, 900
	cfg := Default()
	cfg.ForecastX, cfg.ForecastY = &x, &y
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load after Delete: %v", err)
	}
	if got.ForecastX != nil || got.ForecastY != nil {
		t.Errorf("position survived the delete: %v, %v", got.ForecastX, got.ForecastY)
	}
}

func TestForecastPositionAbsentLoadsNil(t *testing.T) {
	redirectConfigDir(t)

	if err := Default().Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ForecastX != nil || got.ForecastY != nil {
		t.Errorf("position = (%v, %v) for a never-dragged panel, want (nil, nil)", got.ForecastX, got.ForecastY)
	}
}
