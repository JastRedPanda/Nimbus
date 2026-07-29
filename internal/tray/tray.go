package tray

import (
	"fmt"
	"log"
	"sync"

	"fyne.io/systray"
	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gui"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/icons"
	_ "github.com/JastRedPanda/Nimbus/internal/ui" // registers the GUI backends
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

type app struct {
	// cfgMu guards exactly two things, and nothing else in this struct: the cfg
	// POINTER, and the disk write that persists the result. An installed config is
	// never mutated in place - saveForecastPos copies it and swaps the pointer,
	// and openSettings publishes the struct the backend built while it is still
	// private - so a holder of the old pointer keeps reading a consistent
	// snapshot instead of a struct changing under it. That is what lets
	// showForecast copy the fields it needs and let go of the lock before it calls
	// into a backend that may block for as long as the panel is open.
	//
	// The lock does NOT cover most reads of a.cfg's fields, and this comment is
	// not an assurance that it does:
	//   - updateIcon reads Units, IconTheme and PressureUnit unlocked, and runs
	//     both on the settings backend's font-scale callback (the toolkit thread)
	//     and on the settings goroutine via fetchAndUpdate,
	//   - fetchAndUpdate reads Latitude and Longitude and writes lastData on the
	//     settings goroutine, while updateIcon reads lastData from the toolkit
	//     thread,
	//   - handleMenu reads IconTheme on the menu goroutine for the about box,
	//   - a.lang is written on the settings goroutine and read from both of the
	//     others.
	// Every one of those races the pointer swap below, because only the swapping
	// side takes the lock. Widening the locking is a decision about the whole
	// file - which accesses belong on which goroutine at all - and has not been
	// made yet; do not read the narrow scope here as a claim that it is unneeded.
	cfgMu sync.Mutex
	cfg   *config.Config

	lang     i18n.Lang
	lastData *weather.WeatherData

	mForecast *systray.MenuItem
	mSettings *systray.MenuItem
	mAbout    *systray.MenuItem
	mQuit     *systray.MenuItem
}

func newApp(cfg *config.Config) *app {
	return &app{cfg: cfg, lang: i18n.ParseLang(cfg.Language)}
}

func (a *app) ready() {
	icon := icons.GenerateScale(20, 0, "auto", a.cfg.FontScale)
	if icon != nil {
		systray.SetIcon(icon)
	}
	systray.SetTooltip("Nimbus — loading...")

	a.mForecast = systray.AddMenuItem("7-day Forecast", "Open 7-day forecast")
	systray.AddSeparator()
	a.mSettings = systray.AddMenuItem("Settings...", "Configure Nimbus")
	systray.AddSeparator()
	a.mAbout = systray.AddMenuItem("About", "About Nimbus")
	a.mQuit = systray.AddMenuItem("Quit", "Quit Nimbus")

	a.fetchAndUpdate()
	go a.handleMenu()
}

// handleMenu dispatches the tray menu, one item at a time.
//
// Each item logs before it acts, and the reason is diagnostic rather than
// decorative. This loop and the toolkit are separate: the tray speaks D-Bus from
// pure Go, so the menu keeps opening and these channels keep delivering even if
// the toolkit has stopped dispatching entirely. When that happened, the log went
// completely silent - every code path that logged was on the far side of the
// toolkit - and there was no way to tell a wedged loop from a tray that had
// stopped receiving. One line per user action is what makes that distinguishable
// afterwards: "forecast requested" with nothing following it says the request was
// received and the toolkit never answered.
func (a *app) handleMenu() {
	for {
		select {
		case <-a.mForecast.ClickedCh:
			log.Print("tray: forecast requested from the menu")
			a.showForecast()
		case <-a.mSettings.ClickedCh:
			log.Print("tray: settings requested")
			a.openSettings()
		case <-a.mAbout.ClickedCh:
			log.Print("tray: about requested")
			gui.Current().About(a.cfg.IconTheme)
		case <-a.mQuit.ClickedCh:
			log.Print("tray: quit requested from the menu")
			quit()
			return
		}
	}
}

func (a *app) fetchAndUpdate() {
	data, err := weather.Fetch(a.cfg.Latitude, a.cfg.Longitude)
	if err != nil {
		log.Printf("Weather fetch error: %v", err)
		systray.SetTooltip(fmt.Sprintf("Nimbus — error: %v", err))
		return
	}
	a.lastData = data
	a.updateIcon(a.cfg.FontScale)
}

func (a *app) updateIcon(fontScale int) {
	if a.lastData == nil {
		return
	}
	data := a.lastData
	temp := data.Temperature
	apparent := data.ApparentTemp
	if a.cfg.Units == "fahrenheit" {
		temp = temp*9/5 + 32
		apparent = apparent*9/5 + 32
	}

	icon := icons.GenerateScale(temp, data.WeatherCode, a.cfg.IconTheme, fontScale)
	if icon != nil {
		systray.SetIcon(icon)
	}

	detail := a.lang.Tooltip("", data.WeatherCode, temp, apparent,
		int(data.Humidity), data.WindSpeed, data.SurfacePressure,
		a.cfg.Units, a.cfg.PressureUnit, a.cfg.WindUnit)
	systray.SetTooltip(detail)
}

func (a *app) showForecast() {
	// Logged before the lock, not after: if this ever blocks, the log has to show
	// that the request arrived. The tray icon's own tap comes through here too,
	// and that path has no menu line above it.
	log.Print("tray: opening the forecast")
	a.cfgMu.Lock()
	// The reset count the panel is opened against. The panel stays up until it is
	// closed deliberately, so it outlives the settings window and a report can
	// arrive after the configuration it was opened under has been thrown away.
	// See saveForecastPos.
	gen := config.Resets()
	req := gui.Forecast{
		Lat:      a.cfg.Latitude,
		Lon:      a.cfg.Longitude,
		Units:    a.cfg.Units,
		Lang:     a.cfg.Language,
		Theme:    a.cfg.IconTheme,
		WindUnit: a.cfg.WindUnit,
		OnMove:   func(x, y int) { a.saveForecastPos(gen, x, y) },
	}
	// The pointer anchor is what happens when there is nothing remembered yet; a
	// remembered position is handed back whenever there is one.
	if a.cfg.ForecastX != nil && a.cfg.ForecastY != nil {
		req.At = &gui.Point{X: *a.cfg.ForecastX, Y: *a.cfg.ForecastY}
	}
	a.cfgMu.Unlock()

	gui.Current().Forecast(req)
}

// saveForecastPos records where the user dragged the panel to. The backend calls
// it on a throwaway goroutine, which is why blocking on the lock and writing the
// file from here is allowed: nothing about this path runs on the thread that owns
// the windows.
//
// It is only ever called after a real drag - see gui.Forecast.OnMove - so
// arriving here means the position genuinely changed at least once during this
// showing.
//
// gen is config.Resets() as it stood when the panel opened, and it is the second
// half of keeping "Delete configuration" deleted. The carry-forward in
// openSettings covers the config the settings window returns; this covers the
// panel, which is the other writer and the harder one: the panel stays on
// screen until it is closed deliberately, so the settings window can come and
// go while it is still open - the user can drag it, then delete the
// configuration, then close the panel - and without this check the close would
// write the discarded coordinates back into a file that no longer exists,
// recreating it.
func (a *app) saveForecastPos(gen uint64, x, y int) {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	if gen != config.Resets() {
		log.Print("tray: dropping a forecast position from a discarded configuration")
		return
	}
	if a.cfg.ForecastX != nil && a.cfg.ForecastY != nil && *a.cfg.ForecastX == x && *a.cfg.ForecastY == y {
		// Dragging the panel back to where it started must not touch the disk, or
		// the cycle would cost a rewrite of the config file for no change at all.
		return
	}

	// Copy on write. Writing the coordinates into a.cfg in place would mutate the
	// very struct the settings window is holding a pointer to and reading field by
	// field to populate its controls, so a drag while that window is open could
	// hand it a half-old, half-new config. Nobody else may observe this struct
	// until the pointer swap publishes it.
	cfg := *a.cfg
	cfg.ForecastX, cfg.ForecastY = &x, &y
	a.cfg = &cfg
	if err := cfg.Save(); err != nil {
		log.Printf("tray: could not save forecast position: %v", err)
	}
}

func (a *app) openSettings() {
	go func() {
		a.cfgMu.Lock()
		cur := a.cfg
		a.cfgMu.Unlock()

		// Captured before the window opens: the Delete button inside it advances
		// this, and that is how the reset is recognised afterwards.
		resetsAtOpen := config.Resets()

		nc := gui.Current().Settings(cur, func(fs int) { a.updateIcon(fs) })
		if nc == nil {
			return
		}

		a.cfgMu.Lock()
		// Carry the panel's live position forward. The settings window has been
		// editing a snapshot taken when it opened, and the panel owns the position:
		// a user who drags the panel and then clicks Save in a window that was
		// already open would otherwise have the drag silently reverted - and on
		// Windows the panel and the settings window really are different threads,
		// so the drag can land at any point during the edit. The settings backend
		// has already written its own copy to disk, so a re-save is what makes the
		// carried position survive a restart too; it is guarded so the common case
		// (nothing moved) stays a pure in-memory swap.
		//
		// None of that applies when the configuration was DISCARDED rather than
		// edited: "Delete configuration" removes the file and returns Default(),
		// and carrying the live position onto that and saving it recreated the file
		// that was just deleted, holding the coordinates that were just thrown
		// away. config.Resets tells the two cases apart, and skipping the block
		// leaves stale false so the re-save below is skipped with it. A reset means
		// a reset, position included.
		//
		// The counter is captured before the settings window opens and compared
		// after it closes. Asking whether the file exists instead - which is what
		// this did first - is wrong twice over: the answer is only correct in the
		// window between the delete and the next write, so a panel closing in that
		// window put the file back and the delete was undone; and a Save that
		// FAILED also leaves no file, so an ordinary Save read as a reset and
		// silently discarded a position the user had chosen.
		stale := false
		if config.Resets() == resetsAtOpen {
			if a.cfg.ForecastX != nil && a.cfg.ForecastY != nil {
				x, y := *a.cfg.ForecastX, *a.cfg.ForecastY
				if nc.ForecastX == nil || nc.ForecastY == nil || *nc.ForecastX != x || *nc.ForecastY != y {
					nc.ForecastX, nc.ForecastY = &x, &y
					stale = true
				}
			}
		}
		a.cfg = nc
		if stale {
			if err := nc.Save(); err != nil {
				log.Printf("tray: could not save carried-forward forecast position: %v", err)
			}
		}
		a.cfgMu.Unlock()

		if nc.Language != string(a.lang) {
			a.lang = i18n.ParseLang(nc.Language)
		}
		a.fetchAndUpdate()
	}()
}
