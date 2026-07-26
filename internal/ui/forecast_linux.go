//go:build linux

package ui

import (
	"fmt"
	"image/color"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/JastRedPanda/Nimbus/internal/webui"
	"github.com/JastRedPanda/Nimbus/internal/wicons"
)

const (
	forecastWidth = 620
	// -1 lets the panel hug its content. A fixed height would stretch the day
	// cards and leave their contents clinging to the top edge.
	forecastHeight = -1

	// Icon sizes in layout points. Both double cleanly into an embedded asset
	// on a HiDPI screen (64 -> 128, 32 -> 64), which is why the header icon is
	// not larger: there is no 256px artwork to draw it sharply at scale 2.
	heroIconPt = 64
	dayIconPt  = 32

	forecastWindowID = "nimbus-forecast"

	// Clearance from the work-area edges the panel hugs.
	panelMargin = 12
)

// forecastWindow is read and written only on the GTK thread.
var forecastWindow gtk.Window

// ShowForecast opens the 7-day forecast panel. It returns immediately: the
// forecast is fetched on its own goroutine because the caller is the tray's
// single menu-dispatch loop, and a blocking 10s HTTP call there would freeze
// Settings, About and Quit along with it.
func ShowForecast(lat, lon float64, units, lang, theme, windUnit string) {
	if !gtk.Ready() {
		webui.ShowForecast(lat, lon, units, lang, theme, windUnit)
		return
	}

	// The pointer is sampled now, while the user's click is still fresh, rather
	// than when the window is finally built: the fetch in between can take up
	// to ten seconds, by which time the pointer may be on another monitor
	// entirely. GDK is not thread-safe, so even this read goes through the GTK
	// loop - it costs a fraction of a millisecond.
	anchor := make(chan gtk.Rect, 1)
	if err := gtk.Invoke(func() { anchor <- pointerAnchor() }); err != nil {
		anchor <- gtk.Rect{}
	}

	go func() {
		data, err := weather.FetchDaily(lat, lon)
		at := <-anchor
		gtk.Invoke(func() {
			l := i18n.ParseLang(lang)
			if err != nil || len(data) == 0 {
				ensureAppIcon()
				gtk.ShowError(appName, forecastFailed(l), "", closeLabel(l))
				return
			}
			buildForecast(data, units, theme, l, at)
		})
	}()
}

// pointerAnchor captures where the panel should be anchored. A zero rect means
// the position could not be determined - on Wayland, for instance, where a
// client cannot see global pointer coordinates - and the panel is then left
// wherever the window manager puts it.
func pointerAnchor() gtk.Rect {
	x, y, ok := gtk.PointerPosition()
	if !ok {
		return gtk.Rect{}
	}
	area, ok := gtk.WorkAreaAt(x, y)
	if !ok {
		return gtk.Rect{}
	}
	return gtk.Rect{X: x, Y: y, W: area.W, H: area.H}
}

func buildForecast(data []weather.DailyForecast, units, theme string, l i18n.Lang, at gtk.Rect) {
	if forecastWindow != 0 {
		forecastWindow.Present()
		return
	}

	ensureAppIcon()
	gtk.LoadCSS(forecastCSS)

	win := gtk.NewPanel(l.ForecastTitle(), forecastWidth, forecastHeight)
	win.OnDestroy(func() { forecastWindow = 0 })
	gtk.SetName(uintptr(win), forecastWindowID)

	// The visual has to be chosen before the window is realised, and the
	// stylesheet must follow what was actually granted rather than what was
	// asked for: with no compositor the alpha would composite against black and
	// every rounded corner would come out as a black notch.
	fill := "solid"
	if win.SetTranslucent() {
		fill = "translucent"
	}
	gtk.AddClass(uintptr(win), paletteFor(win, theme), fill)

	// With no title bar there is no close button, so the panel supplies its own
	// exits. Escape always works; losing focus is the convenience path.
	win.OnEscape(func() { win.Destroy() })
	win.OnFocusOut(func() { win.Destroy() })

	scale := win.ScaleFactor()

	page := gtk.NewVBox(12)
	gtk.AddClass(page, "page")
	win.Add(page)

	gtk.PackStart(page, header(data[0], units, l, scale, win), false, false, 0)
	gtk.PackStart(page, dayStrip(data, units, l, scale), false, false, 0)

	placePanel(win, page, at)
	forecastWindow = win
}

// placePanel shows the panel at the work-area corner nearest the click.
//
// The order matters and is not the obvious one. A content-hugging layout has no
// size until its children are visible, and neither the preferred size of an
// unrealised window nor the size reported after a bare realise tells the truth -
// both under-report, and the window manager then quietly clamps the window to
// the screen edge, which hides the bad arithmetic behind a result that looks
// almost right. Showing the CONTENT while the toplevel is still unmapped gives
// GTK everything it needs to measure and leaves the window invisible and free
// to move.
func placePanel(win gtk.Window, content uintptr, at gtk.Rect) {
	win.ShowContent(content)

	if at.W > 0 && at.H > 0 {
		if area, ok := gtk.WorkAreaAt(at.X, at.Y); ok {
			w, h := win.Size()
			x, y := corner(at.X, at.Y, w, h, area)
			win.Move(x, y)
		}
	}
	win.Show()
}

// corner picks the work-area corner on the same side as the pointer, so the
// panel opens towards the middle of the screen rather than off its edge. The
// work area is deliberately not the monitor geometry: the difference is exactly
// the desktop panels, and ignoring it puts the forecast underneath them.
func corner(px, py, w, h int, area gtk.Rect) (int, int) {
	x := area.X + panelMargin
	if px > area.X+area.W/2 {
		x = area.X + area.W - w - panelMargin
	}
	y := area.Y + panelMargin
	if py > area.Y+area.H/2 {
		y = area.Y + area.H - h - panelMargin
	}
	return x, y
}

// paletteFor picks the card palette. For an explicit setting it honours the
// user; for "auto" it asks the desktop theme which way round it draws text - a
// light foreground means a dark theme. That is more reliable than the GTK
// prefer-dark flag, which stays false under themes that are simply dark, such
// as BlackMATE.
func paletteFor(win gtk.Window, theme string) string {
	switch theme {
	case "dark":
		return "dark"
	case "light":
		return "light"
	}
	if luminance(win.Foreground()) > 0.5 {
		return "dark"
	}
	return "light"
}

func luminance(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// header is the summary card: today's date, its high and low in large type
// beside the weather icon, the condition in words, and the panel's close button.
func header(today weather.DailyForecast, units string, l i18n.Lang, scale int, win gtk.Window) uintptr {
	card := gtk.NewVBox(6)
	gtk.AddClass(card, "card")

	top := gtk.NewHBox(8)
	date := gtk.NewCell(headerDate(today.Date, l), gtk.AlignStart)
	gtk.AddClass(date, "muted")
	gtk.PackStart(top, date, true, true, 0)

	closeBtn := gtk.NewButton("×", func() { win.Destroy() })
	gtk.AddClass(closeBtn, "close")
	gtk.SetVAlign(closeBtn, gtk.AlignCenter)
	gtk.PackStart(top, closeBtn, false, false, 0)
	gtk.PackStart(card, top, false, false, 0)

	row := gtk.NewHBox(10)

	temps := gtk.NewCell(tempRange(today, units), gtk.AlignStart)
	gtk.AddClass(temps, "hero")
	gtk.PackStart(row, temps, false, false, 0)

	if icon := iconWidget(today.WeatherCode, heroIconPt, scale); icon != 0 {
		gtk.SetVAlign(icon, gtk.AlignCenter)
		gtk.PackStart(row, icon, false, false, 0)
	}

	cond := gtk.NewCell(l.Condition(today.WeatherCode), gtk.AlignEnd)
	gtk.AddClass(cond, "cond")
	gtk.SetVAlign(cond, gtk.AlignCenter)
	gtk.PackStart(row, cond, true, true, 0)

	gtk.PackStart(card, row, false, false, 0)
	return card
}

// dayStrip is the row of equal-width day cards. Index 0 is today: Open-Meteo's
// daily series starts at the current date.
func dayStrip(data []weather.DailyForecast, units string, l i18n.Lang, scale int) uintptr {
	strip := gtk.NewHBox(8)
	gtk.SetHomogeneous(strip)

	for i, day := range data {
		card := gtk.NewVBox(6)
		gtk.AddClass(card, "day")
		if i == 0 {
			gtk.AddClass(card, "today")
		}

		name := gtk.NewCell(shortDay(day.Date, l), gtk.AlignCenter)
		gtk.AddClass(name, "muted")
		gtk.PackStart(card, name, false, false, 0)

		if icon := iconWidget(day.WeatherCode, dayIconPt, scale); icon != 0 {
			gtk.SetHAlign(icon, gtk.AlignCenter)
			gtk.PackStart(card, icon, false, false, 0)
		}

		temps := gtk.NewCell(tempRange(day, units), gtk.AlignCenter)
		gtk.AddClass(temps, "daytemp")
		gtk.PackStart(card, temps, false, false, 0)

		gtk.PackStart(strip, card, true, true, 0)
	}
	return strip
}

// iconWidget renders the weather artwork at pt layout points, using the asset
// that matches the display scale so a HiDPI screen gets real pixels rather than
// an upscaled blur. It returns 0 when no artwork is available; every caller
// treats that as "leave the slot empty" rather than substituting a wrong
// symbol.
func iconWidget(code, pt, scale int) uintptr {
	if scale < 1 {
		scale = 1
	}
	img := wicons.Icon(code, wicons.Size(pt*scale))
	if img == nil && scale > 1 {
		// No artwork at the doubled size; fall back to 1x rather than nothing.
		scale = 1
		img = wicons.Icon(code, wicons.Size(pt))
	}
	if img == nil {
		return 0
	}
	b := img.Bounds()
	return gtk.NewImageRGBAScaled(img.Pix, b.Dx(), b.Dy(), img.Stride, scale)
}

func tempRange(d weather.DailyForecast, units string) string {
	hi, lo := d.TempMax, d.TempMin
	if units == "fahrenheit" {
		hi = hi*9/5 + 32
		lo = lo*9/5 + 32
	}
	return fmt.Sprintf("%.0f°/%.0f°", hi, lo)
}

func forecastFailed(l i18n.Lang) string {
	if l == i18n.UK {
		return "Не вдалося завантажити прогноз."
	}
	return "Failed to load forecast."
}

func closeLabel(l i18n.Lang) string {
	if l == i18n.UK {
		return "Закрити"
	}
	return "Close"
}
