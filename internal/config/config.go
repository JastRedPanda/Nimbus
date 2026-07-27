package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

type Config struct {
	Latitude       float64 `json:"latitude"`
	Longitude      float64 `json:"longitude"`
	CityName       string  `json:"city_name"`
	UpdateInterval int     `json:"update_interval"`
	Units          string  `json:"units"`
	PressureUnit   string  `json:"pressure_unit"`
	WindUnit       string  `json:"wind_unit"`
	IconTheme      string  `json:"icon_theme"`
	Language       string  `json:"language"`
	FontScale      int     `json:"font_scale"`

	// Appearance chooses how the forecast panel is dressed: "modern" is the
	// translucent, round-cornered, undecorated sheet with its own close button,
	// and "system" is an ordinary application window - opaque, square, with the
	// window manager's title bar and its colours taken from the desktop theme.
	// Everything else about the panel is the same either way.
	//
	// The tag has no omitempty for the same reason ForecastPinned's has none, even
	// though the failure looks different: neither legal value is the empty string,
	// so omitempty would never fire today, which is exactly what makes it a trap -
	// it would sit there stating that an empty value may be dropped, and the next
	// edit that gives this field an empty-meaning-default would silently stop
	// persisting the user's choice with nothing in the diff to point at.
	//
	// Readers must treat an unrecognised value as "modern" rather than trusting
	// the string: it is a hand-editable file and a downgrade can leave a value this
	// build has never heard of. Switch on "system" with a default arm - never
	// compare against "modern", which would make every typo mean the system look.
	Appearance string `json:"appearance"`

	// ForecastPinned keeps the forecast panel on screen until the tray icon or
	// the close button dismisses it: while it is set, Escape and losing focus do
	// nothing.
	//
	// The tag deliberately has no omitempty, and adding one would break the
	// setting. The field defaults to true and Load unmarshals the file over
	// Default(), so a false that omitempty had dropped from the file comes back
	// as true on the next start - unchecking the box would never stick, and the
	// symptom would look like the panel ignoring the option rather than like a
	// struct tag.
	ForecastPinned bool `json:"forecast_pinned"`

	// ForecastX and ForecastY remember where the panel was last dragged to.
	// They are pointers because there is no free sentinel value: (0,0) is a
	// legitimate top-left corner and negative coordinates are legitimate on a
	// multi-monitor layout. nil means the panel has never been MOVED - a drag the
	// user cancels does not count, and neither does a Wayland session, where a
	// client cannot know its own position - and the backend then anchors it at the
	// corner nearest the pointer instead.
	ForecastX *int `json:"forecast_x,omitempty"`
	ForecastY *int `json:"forecast_y,omitempty"`
}

func Default() *Config {
	return &Config{
		Latitude:       50.4501,
		Longitude:      30.5234,
		CityName:       "Kyiv",
		UpdateInterval: 10,
		Units:          "celsius",
		PressureUnit:   "hpa",
		WindUnit:       "ms",
		IconTheme:      "auto",
		Language:       "en",
		FontScale:      100,
		Appearance:     "modern",
		ForecastPinned: true,
	}
}

func configDir() (string, error) {
	cd, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cd, "Nimbus"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), err
	}
	cfg := Default()
	if err := json.Unmarshal(data, cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}

// Save writes the config atomically: a temp file in the target's own directory,
// then a rename over the target.
//
// A plain os.WriteFile truncates first and can leave a half-written file behind,
// and there are now two unsynchronised writers - a Save click in the settings
// window, and the forecast panel reporting its position from a detached
// goroutine as it closes. main.go calls log.Fatalf when Load fails, so a torn
// file is not cosmetic: the app refuses to start until someone deletes the
// config by hand. The rename is what makes a concurrent reader see either the
// old file or the new one and never a truncated one.
//
// The temp file goes in the same directory on purpose - os.Rename cannot cross
// filesystems, and os.TempDir often is one.
func (c *Config) Save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	// Preserve whatever mode the file already has; 0644 only decides the mode of
	// a config being created for the first time. CreateTemp makes the temp file
	// 0600, so the mode has to be set explicitly either way.
	mode := os.FileMode(0644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}

	tmp, err := os.CreateTemp(dir, "config.json.tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Every failure past this point has to unlink the temp file, or a directory
	// full of config.json.tmp* is what a user with a full disk ends up with.
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	// Sync before the rename, not just Close. The rename gives a concurrent
	// reader old-or-new, but it says nothing about what survives a power loss:
	// on ext4 and xfs the rename can reach the disk before the data does, and
	// what is left after the reboot is the new name over a zero-length or
	// partially written file - the exact torn config that makes main.go
	// log.Fatalf and the app refuse to start until someone deletes the file by
	// hand.
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	// Close before renaming: on Windows a rename over an open file fails, and
	// the close is also where a deferred write error surfaces.
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

func ConfigPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Delete removes the configuration file and advances the reset counter - see
// Resets, which is the half of this that callers must not skip.
func Delete() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err == nil || os.IsNotExist(err) {
		// Counted even when the file was already gone: what the counter records is
		// "the configuration was discarded", and the user pressed the button either
		// way.
		resets.Add(1)
	}
	return err
}

// resets counts how many times the configuration has been discarded rather than
// edited. Atomic because Delete runs on whichever thread owns the settings
// window while Resets is read from the tray's goroutines.
var resets atomic.Uint64

// Resets reports how many times the configuration has been discarded. It is a
// counter, not a flag: callers capture it, do something slow, and compare.
//
// It exists because "was the configuration reset?" cannot be answered by looking
// at the disk, and two bugs came from trying. The "Delete configuration" button
// removes the file and hands back Default(), which deliberately carries no
// remembered panel position; the tray then carries the live position of a panel
// that is still on screen forward onto the returned config and saves it, which
// recreated the file that had just been deleted with the coordinates that had
// just been discarded. Asking whether the file exists closed the ordinary case
// and left two holes: the answer is only correct in the window between the delete
// and the next write, so a panel closing in that window put the file back; and a
// Save that FAILED also leaves no file, which read as a reset and silently threw
// away a position the user had chosen. A counter has neither hole - it moves when
// and only when the configuration is actually discarded, and it moves inside
// Delete rather than at some later point that a racing writer can slip past.
func Resets() uint64 { return resets.Load() }

func (c *Config) Interval() time.Duration {
	d := time.Duration(c.UpdateInterval) * time.Minute
	if d < time.Minute {
		d = time.Minute
	}
	return d
}

func (c *Config) String() string {
	return fmt.Sprintf("City: %s (%.4f, %.4f) | Interval: %d min | Temp: %s | Pressure: %s | Theme: %s | Lang: %s",
		c.CityName, c.Latitude, c.Longitude, c.UpdateInterval, c.Units, c.PressureUnit, c.IconTheme, c.Language)
}
