//go:build linux && qt

package qt

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

// The settings window, Qt edition.
//
// It is a FORM rather than a window here: this file names the fields, their
// captions and their options, and the shim turns that into widgets. Which is why
// there is no layout code in it at all - the one thing a settings window needs
// that a general widget abstraction would have to reimplement per backend, and
// the reason the gui.Backend seam is drawn at the level of tasks rather than
// widgets.
//
// The vocabulary is Go's. Every option list below pairs a caption with the
// string the configuration stores, and config.Index/config.Pick map between the
// two - the same helpers the GTK and Win32 windows use, so an unrecognised
// stored value means the same thing in all three.

// The field keys. They are handed to the shim as opaque integers and come back
// with the values, so the numbers matter only in that they are distinct.
const (
	keyCity = iota + 1
	keyLat
	keyLon
	keyResults
	keyUnits
	keyPressure
	keyWind
	keyTheme
	keyLang
	keyScale
	keyAppearance
	keyPinned
	keyInterval
)

// resultsMinH is how tall the search-result list is, matching the GTK window.
const resultsMinH = 92

// fontScaleSettle is how long the font-scale slider waits before regenerating
// the tray icon. Short enough to read as live feedback, long enough that a drag
// across the whole range costs a handful of regenerations instead of a hundred -
// each one rasterises two frames, PNG-encodes them and emits a D-Bus signal.
const fontScaleSettle = 150 * time.Millisecond

// The option lists. Caption first, stored value second, in the same order, and
// the stored order is what config.Pick indexes into.
var (
	unitValues     = []string{"celsius", "fahrenheit"}
	pressureValues = []string{"hpa", "mmhg", "inhg"}
	windValues     = []string{"ms", "kmh"}
	themeValues    = []string{"auto", "dark", "light"}
	langValues     = []string{"en", "uk"}
	// Modern first, and this is load-bearing: config.Index answers 0 for anything
	// it does not recognise, so putting the system look first would make a
	// hand-edited or downgraded file mean "system" - the opposite of the rule the
	// panel applies to the same string.
	lookValues = []string{"modern", "system"}
)

// settingsOpen is read and written only on the Qt thread.
var settingsOpen bool

// Settings opens the window and blocks until the user is done, returning the
// configuration to adopt or nil to change nothing.
//
// The caller is a goroutine the tray spawned for exactly this, so blocking here
// is correct - but it must never block forever. Every way out of the window,
// including the window manager's own close button, resolves the result, and so
// does a Qt that never started.
func (backend) Settings(cfg *config.Config, onFontScale func(int)) *config.Config {
	result := make(chan *config.Config, 1)
	if !invoke(func() { buildForm(cfg, onFontScale, result) }) {
		return nil
	}
	return <-result
}

func buildForm(cfg *config.Config, onFontScale func(int), result chan<- *config.Config) {
	if settingsOpen {
		// A second window would leave the first caller blocked for good.
		result <- nil
		return
	}
	settingsOpen = true

	l := i18n.ParseLang(cfg.Language)

	// Everything below is touched only on the Qt thread: the field values arrive
	// through the field callback, the search results through an invoke, and the
	// font-scale coalescing through a timer that goes back through one.
	values := map[int64]string{}
	var found []weather.GeoResult
	scalePending, scaleSettling := cfg.FontScale, false

	var id uint64
	id = register(&window{
		field: func(key int64, value string) { values[key] = value },
		event: func(code, a, b int64) {
			switch code {
			case evSearch:
				// The query has to be read out of the field before the lookup can
				// start, and reading it is a round trip through the same callback
				// that delivers values on save.
				qtFormReport(keyCity)
				query := values[keyCity]
				if query == "" {
					return
				}
				// The button greys out meanwhile: without that the user cannot tell
				// a slow search from one that never started.
				qtFormEnable(keyCity, 0)
				go func() {
					res, err := weather.SearchCity(query, l.String())
					if err != nil {
						log.Printf("qt: city search failed: %v", err)
					}
					invoke(func() {
						qtFormEnable(keyCity, 1)
						qtFormListClear(keyResults)
						found = res
						if err != nil || len(res) == 0 {
							found = nil
							qtFormListAdd(keyResults, l.NoResults())
							return
						}
						for _, g := range res {
							qtFormListAdd(keyResults, fmt.Sprintf("%s, %s (%.4f, %.4f)",
								g.Name, g.Country, g.Latitude, g.Longitude))
						}
					})
				}()

			case evPick:
				// Picking a result fills all three fields at once, which is the
				// point of the list: the coordinates are what the program actually
				// uses and nobody types them by hand.
				if int(b) < 0 || int(b) >= len(found) {
					return
				}
				g := found[b]
				qtFormSet(keyCity, g.Name)
				qtFormSet(keyLat, fmt.Sprintf("%.4f", g.Latitude))
				qtFormSet(keyLon, fmt.Sprintf("%.4f", g.Longitude))

			case evSlide:
				if onFontScale == nil {
					return
				}
				// Coalesced, and the preview runs off the Qt thread. Regenerating
				// the tray icon costs a millisecond or two and about a megabyte per
				// call, and the slider reports every integer the thumb crosses - a
				// drag across the range is some two hundred of them.
				scalePending = int(b)
				if scaleSettling {
					return
				}
				scaleSettling = true
				time.AfterFunc(fontScaleSettle, func() {
					invoke(func() {
						scaleSettling = false
						v := scalePending
						go onFontScale(v)
					})
				})

			case evAction:
				settingsOpen = false
				drop(id)
				result <- adopt(cfg, values, int(a))
			}
		},
	})

	qtFormBegin(l.SettingsTitle())

	qtFormGroup(l.CityLabel())
	qtFormText(keyCity, "", cfg.CityName, l.SearchBtn())
	qtFormList(keyResults, resultsMinH)
	qtFormText(keyLat, l.LatLabel(), fmt.Sprintf("%.4f", cfg.Latitude), "")
	qtFormText(keyLon, l.LonLabel(), fmt.Sprintf("%.4f", cfg.Longitude), "")
	qtFormGroup("")

	choice(keyUnits, l.TemperatureGroup(), []string{l.Celsius(), l.Fahrenheit()},
		config.Index(cfg.Units, unitValues...))
	choice(keyPressure, l.PressureGroup(), []string{l.HPa(), l.MmHg(), l.InHg()},
		config.Index(cfg.PressureUnit, pressureValues...))
	choice(keyWind, l.WindGroup(), []string{l.WindMS(), l.WindKMH()},
		config.Index(cfg.WindUnit, windValues...))
	choice(keyTheme, l.ThemeGroup(), []string{l.ThemeAuto(), l.ThemeDark(), l.ThemeLight()},
		config.Index(cfg.IconTheme, themeValues...))
	choice(keyLang, l.LanguageGroup(), []string{"English", "Українська"},
		config.Index(cfg.Language, langValues...))

	qtFormSlider(keyScale, l.FontScaleGroup(), 1, 100, int32(cfg.FontScale))

	// Directly above the pin checkbox: both settings are about the forecast panel
	// and nothing else, so they belong next to each other.
	choice(keyAppearance, l.AppearanceGroup(), []string{l.LookModern(), l.LookSystem()},
		config.Index(cfg.Appearance, lookValues...))
	check(keyPinned, l.PinForecast(), cfg.ForecastPinned)

	combo(keyInterval, l.UpdateInterval(), config.IntervalLabels(),
		config.IntervalIndex(cfg.UpdateInterval))

	qtFormButtons(l.SaveBtn(), l.CancelBtn(), l.DeleteCfgBtn())
	qtFormShow(id, eventTramp, fieldTramp)
}

func choice(key int32, label string, options []string, active int) {
	qtFormChoice(key, label)
	for i, o := range options {
		qtFormOption(o, b2i(i == active))
	}
}

func combo(key int32, label string, options []string, active int) {
	qtFormCombo(key, label)
	for i, o := range options {
		qtFormOption(o, b2i(i == active))
	}
}

func check(key int32, label string, on bool) { qtFormCheck(key, label, b2i(on)) }

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// adopt turns what the form reported into the configuration to keep, or nil for
// "change nothing".
//
// Every value arrives as text, because that is all the field callback can carry.
// The parsing is deliberately forgiving in one direction only: a field that
// cannot be read keeps what the config already had, rather than writing a
// default over a setting the user never touched.
func adopt(cfg *config.Config, values map[int64]string, action int) *config.Config {
	switch action {
	case actionReset:
		// Logged the way the other two backends log it: Delete advances the reset
		// counter either way, so a failure here leaves the user believing the
		// configuration is gone when the file is still on disk.
		if err := config.Delete(); err != nil {
			log.Printf("qt: deleting the configuration failed: %v", err)
		}
		return config.Default()

	case actionSave:
		nc := *cfg
		if v, ok := values[keyCity]; ok {
			nc.CityName = v
		}
		nc.Latitude = config.ParseCoord(values[keyLat], cfg.Latitude)
		nc.Longitude = config.ParseCoord(values[keyLon], cfg.Longitude)
		nc.Units = choose(values[keyUnits], cfg.Units, unitValues)
		nc.PressureUnit = choose(values[keyPressure], cfg.PressureUnit, pressureValues)
		nc.WindUnit = choose(values[keyWind], cfg.WindUnit, windValues)
		nc.IconTheme = choose(values[keyTheme], cfg.IconTheme, themeValues)
		nc.Language = choose(values[keyLang], cfg.Language, langValues)
		nc.Appearance = choose(values[keyAppearance], cfg.Appearance, lookValues)
		if v, err := strconv.Atoi(values[keyScale]); err == nil {
			nc.FontScale = v
		}
		if v, ok := values[keyPinned]; ok {
			nc.ForecastPinned = v == "1"
		}
		if v, err := strconv.Atoi(values[keyInterval]); err == nil && v >= 0 && v < len(config.Intervals) {
			nc.UpdateInterval = config.Intervals[v].Minutes
		}
		// The error is logged rather than swallowed: Save has several ways to fail
		// now that it writes through a temp file, and a silent failure means the
		// user's settings are gone on the next start with nothing to explain it.
		if err := nc.Save(); err != nil {
			log.Printf("qt: could not save the configuration: %v", err)
		}
		return &nc
	}
	return nil
}

// choose maps a reported option index back to the stored value, keeping the old
// one when the group reported no selection at all.
//
// That last part is the reason this is not just config.Pick. A group that could
// not be built reports -1, and config.Pick turns any out-of-range index into the
// FIRST option - which would write "celsius" over a user who had chosen
// Fahrenheit and was never shown either button. The GTK window guards the same
// case the same way.
func choose(reported, current string, options []string) string {
	i, err := strconv.Atoi(reported)
	if err != nil || i < 0 || i >= len(options) {
		return current
	}
	return config.Pick(i, options...)
}
