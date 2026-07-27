//go:build linux

package ui

import (
	"fmt"
	"log"
	"strconv"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

const (
	settingsWidth   = 460
	resultsMinH     = 92
	settingsSpacing = 10
	// settingsChromeH is what the window needs around the scrolling page: the button
	// row, the 14px border top and bottom, the box spacing between them and a title
	// bar. Deliberately generous - a ceiling a few pixels too low costs a scrollbar
	// nobody needed, one too high costs the buttons.
	settingsChromeH = 120
)

// Update intervals in minutes, with the captions the dropdown shows.
var intervals = []struct {
	minutes int
	label   string
}{
	{5, "5 min"},
	{30, "30 min"},
	{60, "1 hour"},
	{720, "12 hours"},
	{1440, "24 hours"},
}

// settingsOpen is read and written only on the GTK thread.
var settingsOpen bool

// showSettings opens the settings window and blocks until the user is done,
// returning the configuration to adopt or nil to change nothing.
//
// The caller is a goroutine the tray spawned for exactly this, so blocking here
// is correct - but it must never block forever. Every way out of the window,
// including the window manager's own close button, resolves the result.
func showSettings(cfg *config.Config, onFontScale func(int)) *config.Config {
	result := make(chan *config.Config, 1)
	if err := gtk.Invoke(func() { buildSettings(cfg, onFontScale, result) }); err != nil {
		return nil
	}
	return <-result
}

func buildSettings(cfg *config.Config, onFontScale func(int), result chan<- *config.Config) {
	if settingsOpen {
		// A second window would leave the first caller blocked for good.
		result <- nil
		return
	}
	settingsOpen = true

	l := i18n.ParseLang(cfg.Language)
	ensureAppIcon()

	win := gtk.NewWindow(l.SettingsTitle(), settingsWidth, -1, false)
	win.SetBorder(14)

	// finish resolves the caller exactly once. Every exit routes through it -
	// Save, Cancel, Delete, Escape and the window manager's own close button - so
	// the goroutine waiting on the channel can never be stranded.
	//
	// The guard is a plain bool and MUST NOT become a sync.Once. It was one, and
	// that deadlocked the entire GTK main loop, permanently, on every Save:
	// gtk_widget_destroy emits "destroy" SYNCHRONOUSLY, the handler below calls
	// finish again on this same goroutine, and sync.Once.Do is not reentrant - it
	// holds a mutex until f returns and only marks itself done afterwards, so the
	// nested call blocks on that mutex for the life of the process. It blocks the
	// GTK thread, so nothing scheduled with Invoke ever runs again: the forecast
	// panel stops opening and Quit stops working, while the tray icon keeps
	// answering because it is pure Go on its own goroutines. No panic, no deadlock
	// detector - the runtime still sees runnable goroutines.
	//
	// Everything here runs on the GTK thread and nothing else touches it, so a
	// bool is not merely sufficient, it is the correct tool: re-entrancy is the
	// normal case for this function, not a race to be excluded.
	//
	// The window manager's close button was the one exit that never showed the
	// bug, which is why it survived testing: there "destroy" is emitted from
	// outside finish, so the outer call runs first and the nested Destroy is the
	// no-op gtk_widget_destroy's own in_destruction guard makes it.
	done := false
	finish := func(nc *config.Config) {
		if done {
			return
		}
		done = true
		settingsOpen = false
		result <- nc
		win.Destroy()
	}
	win.OnDestroy(func() { finish(nil) })
	win.OnEscape(func() { finish(nil) })

	// The page scrolls; the buttons do not. A non-resizable window around an
	// unscrolled page has its content's natural height as its only height, so every
	// setting added to this window pushed the button row further down until, on a
	// 1366x768 screen, Save, Cancel and Delete sat below the bottom edge with no way
	// to reach them - measured at 758 pixels of content against a 768 pixel screen.
	// Keeping the buttons in the outer box means the window can run out of room for
	// the settings without ever running out of room for the way to accept them.
	//
	// The ceiling is the work area the pointer is on, less what the buttons, the
	// window border and the title bar need. A page that fits is unaffected: the
	// scroller asks for its child's natural height, so the window looks and measures
	// exactly as it did before this was here.
	outer := gtk.NewVBox(settingsSpacing)
	win.Add(outer)

	page := gtk.NewVBox(settingsSpacing)
	gtk.PackStart(outer, gtk.NewScrolledPage(page, settingsMaxPageH()), true, true, 0)

	city := gtk.NewEntry(cfg.CityName)
	lat := gtk.NewEntry(fmt.Sprintf("%.4f", cfg.Latitude))
	lon := gtk.NewEntry(fmt.Sprintf("%.4f", cfg.Longitude))

	gtk.PackStart(page, cityFrame(l, city, lat, lon), false, false, 0)

	temp := radioFrame(page, l.TemperatureGroup(), []string{"°C", "°F"},
		index(cfg.Units, "celsius", "fahrenheit"))
	pressure := radioFrame(page, l.PressureGroup(), []string{l.HPa(), l.MmHg(), l.InHg()},
		index(cfg.PressureUnit, "hpa", "mmhg", "inhg"))
	wind := radioFrame(page, l.WindGroup(), []string{l.WindMS(), l.WindKMH()},
		index(cfg.WindUnit, "ms", "kmh"))
	theme := radioFrame(page, l.ThemeGroup(), []string{l.ThemeAuto(), l.ThemeDark(), l.ThemeLight()},
		index(cfg.IconTheme, "auto", "dark", "light"))
	lang := radioFrame(page, l.LanguageGroup(), []string{"English", "Українська"},
		index(cfg.Language, "en", "uk"))

	scale := sliderFrame(page, l, cfg.FontScale, onFontScale)

	// Directly above the pin checkbox: both settings are about the forecast panel
	// and nothing else, so they belong next to each other. Modern is first because
	// it is what index falls back to, which is what an unrecognised value in the
	// file must show.
	//
	// A row with a plain caption, NOT a titled frame like every other group in this
	// window, and the reason is arithmetic rather than taste. This window is
	// gtk.NewWindow(..., resizable=false) around an unscrolled page, so its natural
	// height IS its height and nothing can shrink it. Measured on a 1366x768 screen:
	// 723 client pixels under BlackMATE before this option existed, already past a
	// 720 work area, and a frame for these two words took it to 785 - the whole
	// button row below the bottom edge of the screen, with no way left to reach
	// Save. A frame costs its border and its title on top of the row it holds; a
	// caption in the row costs the row. The pin checkbox below is frameless for the
	// same reason and says so.
	appearance := radioRow(page, l.AppearanceGroup(), []string{l.LookModern(), l.LookSystem()},
		index(cfg.Appearance, "modern", "system"))

	// Straight into the page, with no frame around it either. A titled border
	// holding a single checkbox is chrome for nothing, and the checkbox's own label
	// is its title.
	pin := gtk.NewCheck(l.PinForecast(), cfg.ForecastPinned)
	gtk.PackStart(page, uintptr(pin), false, false, 0)

	interval := gtk.NewCombo(intervalLabels(), intervalIndex(cfg.UpdateInterval))
	gtk.PackStart(page, gtk.NewFrame(l.UpdateInterval(), uintptr(interval)), false, false, 0)

	save := gtk.NewButton(l.SaveBtn(), func() {
		nc := *cfg
		nc.CityName = city.Text()
		nc.Latitude = parseCoord(lat.Text(), cfg.Latitude)
		nc.Longitude = parseCoord(lon.Text(), cfg.Longitude)
		nc.Units = pick(temp.Active(), "celsius", "fahrenheit")
		nc.PressureUnit = pick(pressure.Active(), "hpa", "mmhg", "inhg")
		nc.WindUnit = pick(wind.Active(), "ms", "kmh")
		nc.IconTheme = pick(theme.Active(), "auto", "dark", "light")
		nc.Language = pick(lang.Active(), "en", "uk")
		nc.FontScale = scale.Value()
		// Only when the buttons were actually built. Active answers -1 for a group
		// with no members, and pick turns any out-of-range index into the first
		// option - so a group that could not be created would write "modern" over a
		// user who had chosen the system look and was never shown either button.
		// nc carries the stored value forward instead.
		if i := appearance.Active(); i >= 0 {
			nc.Appearance = pick(i, "modern", "system")
		}
		// Only when the box was actually built. A Check that could not be
		// created reads as unticked, and writing that would turn off an option
		// the user was never shown; nc carries the stored value forward instead.
		if pin != 0 {
			nc.ForecastPinned = pin.Active()
		}
		if i := interval.Active(); i >= 0 && i < len(intervals) {
			nc.UpdateInterval = intervals[i].minutes
		}
		// The error is logged rather than swallowed: Save has eight ways to fail
		// now that it writes through a temp file, and a silent failure here means
		// the user's settings are gone on the next start with nothing to explain it.
		if err := nc.Save(); err != nil {
			log.Printf("settings: could not save the configuration: %v", err)
		}
		finish(&nc)
	})
	cancel := gtk.NewButton(l.CancelBtn(), func() { finish(nil) })
	del := gtk.NewButton(l.DeleteCfgBtn(), func() {
		config.Delete()
		finish(config.Default())
	})

	buttons := gtk.NewHBox(8)
	gtk.PackStart(buttons, save, true, true, 0)
	gtk.PackStart(buttons, cancel, true, true, 0)
	gtk.PackStart(buttons, del, true, true, 0)
	// Into outer, NOT page: these must stay reachable however tall the settings get.
	gtk.PackStart(outer, buttons, false, false, 0)

	win.ShowAll()
}

// cityFrame is the city name, its search button, the result list and the two
// coordinate fields. Picking a result fills all three fields at once.
func cityFrame(l i18n.Lang, city, lat, lon gtk.Entry) uintptr {
	box := gtk.NewVBox(8)

	row := gtk.NewHBox(6)
	gtk.SetHExpand(uintptr(city))
	gtk.PackStart(row, uintptr(city), true, true, 0)

	results := gtk.NewVBox(0)
	var search uintptr
	search = gtk.NewButton(l.SearchBtn(), func() {
		query := city.Text()
		if query == "" {
			return
		}
		// The lookup is a network call, so it cannot run on the GTK thread.
		// The button greys out meanwhile: without that the user cannot tell a
		// slow search from one that never started.
		gtk.SetSensitive(search, false)
		go func() {
			found, err := weather.SearchCity(query, l.String())
			gtk.Invoke(func() {
				gtk.SetSensitive(search, true)
				gtk.ClearContainer(results)
				if err != nil || len(found) == 0 {
					gtk.PackStart(results, gtk.NewText(l.NoResults()), false, false, 0)
					gtk.ShowAll(results)
					return
				}
				for _, g := range found {
					g := g
					label := fmt.Sprintf("%s, %s (%.4f, %.4f)", g.Name, g.Country, g.Latitude, g.Longitude)
					gtk.PackStart(results, gtk.NewListRow(label, func() {
						city.SetText(g.Name)
						lat.SetText(fmt.Sprintf("%.4f", g.Latitude))
						lon.SetText(fmt.Sprintf("%.4f", g.Longitude))
					}), false, false, 0)
				}
				gtk.ShowAll(results)
			})
		}()
	})
	gtk.PackStart(row, search, false, false, 0)
	gtk.PackStart(box, row, false, false, 0)
	gtk.PackStart(box, gtk.NewScrolled(results, resultsMinH), false, false, 0)

	coords := gtk.NewHBox(6)
	gtk.PackStart(coords, gtk.NewCell(l.LatLabel(), gtk.AlignStart), false, false, 0)
	gtk.SetHExpand(uintptr(lat))
	gtk.PackStart(coords, uintptr(lat), true, true, 0)
	gtk.PackStart(coords, gtk.NewCell(l.LonLabel(), gtk.AlignStart), false, false, 0)
	gtk.SetHExpand(uintptr(lon))
	gtk.PackStart(coords, uintptr(lon), true, true, 0)
	gtk.PackStart(box, coords, false, false, 0)

	return gtk.NewFrame(l.CityLabel(), box)
}

// radioFrame lays a group out in a row inside its own frame and returns it so
// the Save handler can read which option won.
func radioFrame(page uintptr, title string, labels []string, active int) gtk.RadioGroup {
	group := gtk.NewRadioGroup(labels, active)
	row := gtk.NewHBox(12)
	for _, b := range group {
		gtk.PackStart(row, b, false, false, 0)
	}
	gtk.PackStart(page, gtk.NewFrame(title, row), false, false, 0)
	return group
}

// radioRow is radioFrame without the frame: a caption and the buttons on one
// line. It exists because this window has no vertical room for another titled
// group - see the comment at its call site, which has the measurements.
// settingsMaxPageH is the tallest the scrolling part of the window may be: the
// work area the pointer is on, less the room the button row, the window border and
// the window manager's title bar need. Zero when the work area cannot be read, in
// which case the scroller imposes no ceiling and the window behaves exactly as it
// did before it scrolled at all.
func settingsMaxPageH() int {
	x, y, ok := gtk.PointerPosition()
	if !ok {
		return 0
	}
	area, ok := gtk.WorkAreaAt(x, y)
	if !ok {
		return 0
	}
	h := area.H - settingsChromeH
	if h < resultsMinH {
		// A work area this small makes any ceiling absurd; let the window be its
		// natural height and let the window manager deal with it.
		return 0
	}
	return h
}

func radioRow(page uintptr, caption string, labels []string, active int) gtk.RadioGroup {
	group := gtk.NewRadioGroup(labels, active)
	row := gtk.NewHBox(12)
	gtk.PackStart(row, gtk.NewText(caption+":"), false, false, 0)
	for _, b := range group {
		gtk.PackStart(row, b, false, false, 0)
	}
	gtk.PackStart(page, row, false, false, 0)
	return group
}

// sliderFrame is the tray font size, which previews live: dragging regenerates
// the tray icon immediately, without saving, so the user can judge the size
// against the real notification area rather than a number.
func sliderFrame(page uintptr, l i18n.Lang, value int, onFontScale func(int)) gtk.Slider {
	slider := gtk.NewSlider(1, 100, value)
	readout := gtk.NewCell(fmt.Sprintf("%d%%", value), gtk.AlignEnd)

	slider.OnChange(func(v int) {
		gtk.SetLabel(readout, fmt.Sprintf("%d%%", v))
		if onFontScale != nil {
			onFontScale(v)
		}
	})

	row := gtk.NewHBox(8)
	gtk.SetHExpand(uintptr(slider))
	gtk.PackStart(row, uintptr(slider), true, true, 0)
	gtk.PackStart(row, readout, false, false, 0)
	gtk.PackStart(page, gtk.NewFrame(l.FontScaleGroup(), row), false, false, 0)
	return slider
}

func intervalLabels() []string {
	out := make([]string, len(intervals))
	for i, iv := range intervals {
		out[i] = iv.label
	}
	return out
}

func intervalIndex(minutes int) int {
	for i, iv := range intervals {
		if iv.minutes == minutes {
			return i
		}
	}
	return 0
}

// index maps a stored config value to its position among the options. An
// unknown value falls back to the first, which is what the config defaults to.
func index(value string, options ...string) int {
	for i, o := range options {
		if o == value {
			return i
		}
	}
	return 0
}

func pick(i int, options ...string) string {
	if i < 0 || i >= len(options) {
		return options[0]
	}
	return options[i]
}

// parseCoord keeps the previous value when the field holds nonsense, rather
// than silently moving the user to the Gulf of Guinea.
func parseCoord(s string, fallback float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fallback
	}
	return v
}
