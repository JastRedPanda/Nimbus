//go:build linux

package ui

import (
	"image/color"
	"log"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/fonts"
	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

// The 7-day forecast panel, GTK edition.
//
// The CONTENT is a plain five-column table: one header row, a rule, then seven
// data rows separated by hairlines. No summary header and no cards - that layout
// was tried and reverted. The CHROME around it is the panel work and stays:
// undecorated, off the taskbar, above other windows, translucent only where a
// compositor can actually composite it, placed at the work-area corner nearest
// the click, and dismissed by Escape, by focus loss, or by its own × button.
//
// Every metric is shared with the Win32 backend value for value - see the
// constants below and the stylesheet in style_linux.go, whose numbers
// forecast_windows.go carries as named constants of its own. The point of the
// two files is that the two platforms look like one product.

const (
	forecastWidth = 620
	// -1 lets the panel hug its content. A fixed height would stretch the table
	// rows over whatever height was guessed.
	forecastHeight = -1

	// Weather-symbol size in layout points. Unlike the colour artwork this
	// replaced, the glyph is rasterised at whatever size is asked for, so a
	// HiDPI screen simply gets scale times as many pixels rather than the
	// nearest available asset.
	symbolPt = 20

	// Grid metrics. These live in Go rather than in the stylesheet because GTK
	// grid spacing is a widget property, not a style property - CSS cannot set
	// it. Counterparts in forecast_windows.go: rowGapY, colGapX.
	rowGapY = 6
	colGapX = 18

	// Page VBox spacing: the close-button row to the table. Counterpart:
	// pageGapY.
	pageGapY = 2

	forecastWindowID = "nimbus-forecast"

	// Clearance from the work-area edges the panel hugs.
	panelMargin = 12
)

// closeGlyph is the panel's own close affordance. With no title bar there is no
// system close button, so the panel supplies one, exactly as the Win32 panel
// does.
const closeGlyph = "×" // MULTIPLICATION SIGN

// colAlign is the horizontal alignment of each table column, applied to both the
// header caption and the cells under it so a column reads as one thing. Day
// reads as a label and sits left; the symbol is centred in its column; the three
// numeric columns are right aligned so their digits line up, which is the whole
// reason a table beats a row of cards.
var colAlign = [...]int{
	gtk.AlignStart,  // Day
	gtk.AlignCenter, // Condition
	gtk.AlignEnd,    // Temp
	gtk.AlignEnd,    // Wind
	gtk.AlignEnd,    // Precip
}

// Both are read and written only on the GTK thread. forecastClosedAt is when the
// panel last went away on its own, which the tray toggle needs to tell a closing
// click from an opening one.
var (
	forecastWindow   gtk.Window
	forecastClosedAt time.Time
)

// closeOpenPanel makes the tray icon a toggle: if the panel is up, the click
// that would have opened it closes it instead. It reports whether it consumed
// the click, and must run on the GTK thread.
func closeOpenPanel() bool {
	if forecastWindow != 0 {
		forecastWindow.Destroy()
		return true
	}
	if !forecastClosedAt.IsZero() && time.Since(forecastClosedAt) < toggleGrace {
		// Worth a line: to the user this click did nothing at all.
		log.Print("forecast: click within the toggle grace period, treated as the closing click")
		return true
	}
	return false
}

// showForecast opens the 7-day forecast panel. It returns immediately: the
// forecast is fetched on its own goroutine because the caller is the tray's
// single menu-dispatch loop, and a blocking 10s HTTP call there would freeze
// Settings, About and Quit along with it.
func showForecast(lat, lon float64, units, lang, theme, windUnit string) {
	// The pointer is sampled now, while the user's click is still fresh, rather
	// than when the window is finally built: the fetch in between can take up
	// to ten seconds, by which time the pointer may be on another monitor
	// entirely. GDK is not thread-safe, so even this read goes through the GTK
	// loop - it costs a fraction of a millisecond.
	type start struct {
		at       gtk.Rect
		consumed bool
	}
	begin := make(chan start, 1)
	if err := gtk.Invoke(func() {
		if closeOpenPanel() {
			begin <- start{consumed: true}
			return
		}
		begin <- start{at: pointerAnchor()}
	}); err != nil {
		begin <- start{}
	}

	go func() {
		s := <-begin
		if s.consumed {
			return
		}
		data, err := weather.FetchDaily(lat, lon)
		schedErr := gtk.Invoke(func() {
			l := i18n.ParseLang(lang)
			if err != nil {
				log.Printf("forecast: fetch failed: %v", err)
			} else if len(data) == 0 {
				log.Print("forecast: fetch returned no days")
			}
			if err != nil || len(data) == 0 {
				ensureAppIcon()
				gtk.ShowError(appName, forecastFailed(l), "", closeLabel(l))
				return
			}
			buildForecast(data, units, windUnit, theme, l, s.at)
		})
		if schedErr != nil {
			// Nothing will ever draw this. Say so rather than leaving the user
			// clicking a menu item that silently does nothing.
			log.Printf("forecast: cannot reach the GTK loop: %v", schedErr)
		}
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

func buildForecast(data []weather.DailyForecast, units, windUnit, theme string, l i18n.Lang, at gtk.Rect) {
	if forecastWindow != 0 {
		forecastWindow.Present()
		return
	}

	ensureAppIcon()
	gtk.LoadCSS(forecastCSS)

	win := gtk.NewPanel(l.ForecastTitle(), forecastWidth, forecastHeight)
	win.OnDestroy(func() {
		forecastWindow = 0
		forecastClosedAt = time.Now()
	})
	gtk.SetName(uintptr(win), forecastWindowID)

	// The visual has to be chosen before the window is realised, and the
	// stylesheet must follow what was actually granted rather than what was
	// asked for: with no compositor the alpha would composite against black and
	// every rounded corner would come out as a black notch.
	fill := "solid"
	if win.SetTranslucent() {
		fill = "translucent"
	}
	palette := paletteFor(win, theme)
	gtk.AddClass(uintptr(win), palette, fill)

	// With no title bar there is no close button, so the panel supplies its own
	// exits. Escape always works; losing focus is the convenience path.
	//
	// Focus loss is armed only after the panel has actually held focus once. An
	// undecorated window is not guaranteed to be given focus when it is mapped,
	// and without this the first focus-out - which can arrive before the user
	// has seen anything - closes the panel again immediately. The symptom is a
	// panel that flickers and vanishes, intermittently, depending on what held
	// focus at the moment it opened. The Win32 backend has always armed this.
	armed := false
	win.OnEscape(func() { win.Destroy() })
	win.OnFocusIn(func() { armed = true })
	win.OnFocusOut(func() {
		if !armed {
			return
		}
		// Any window taking focus lands here, not just one the user clicked -
		// a notification or a background window will close the panel too. The
		// line is here because an unexplained disappearance is otherwise
		// indistinguishable from a crash.
		log.Print("forecast: closing, focus lost")
		win.Destroy()
	})

	scale := win.ScaleFactor()

	page := gtk.NewVBox(pageGapY)
	gtk.AddClass(page, "page")
	win.Add(page)

	gtk.PackStart(page, closeRow(win), false, false, 0)
	gtk.PackStart(page, forecastTable(data, units, windUnit, l, scale, paletteForeground(palette)), false, false, 0)

	placePanel(win, page, at)
	forecastWindow = win
}

// closeRow is the panel's title-bar substitute: nothing but the × button,
// pushed to the trailing edge of the sheet.
//
// The button is packed expanding and filling with its own halign set, rather
// than packed non-expanding after a spacer label. A spacer would be one more
// label for the desktop theme to state a colour and a padding on, and there is
// no summary text left for it to hide behind.
func closeRow(win gtk.Window) uintptr {
	row := gtk.NewHBox(0)
	btn := gtk.NewButton(closeGlyph, func() { win.Destroy() })
	gtk.AddClass(btn, "close")
	gtk.SetHAlign(btn, gtk.AlignEnd)
	gtk.PackStart(row, btn, true, true, 0)
	return row
}

// forecastTable builds the table: captions, a rule, then one row per day with a
// hairline between neighbours.
//
// The grid is deliberately NOT column-homogeneous - forcing the columns equal
// makes every one of them as wide as the widest caption ("Температура") and puts
// a large floor under the panel width. Every cell is a gtk.NewCell, which does
// not wrap: a wrapping label reports a near-zero minimum width, and its column
// then collapses to nothing the moment the panel is narrower than its content.
func forecastTable(data []weather.DailyForecast, units, windUnit string, l i18n.Lang, scale int, fg color.NRGBA) uintptr {
	grid := gtk.NewGrid(rowGapY, colGapX)
	cols := len(colAlign)

	for i, caption := range l.ForecastHeaders() {
		if i >= cols {
			break
		}
		cell := gtk.NewCell(caption, colAlign[i])
		gtk.AddClass(cell, "thead")
		grid.Attach(cell, i, 0, 1, 1)
	}

	rule := gtk.NewHSeparator()
	gtk.AddClass(rule, "rule")
	grid.Attach(rule, 0, 1, cols, 1)

	row := 2
	for i, d := range data {
		if i > 0 {
			sep := gtk.NewHSeparator()
			gtk.AddClass(sep, "rowsep")
			grid.Attach(sep, 0, row, cols, 1)
			row++
		}

		// The ISO date exactly as Open-Meteo returned it. No weekday name and no
		// localised month: the column is a date, and a sortable one reads the
		// same in both languages.
		grid.Attach(dataCell(d.Date, 0), 0, row, 1, 1)

		// A missing glyph leaves the slot empty rather than substituting a
		// symbol that means something else.
		if sym := symbolWidget(d.WeatherCode, symbolPt, scale, fg); sym != 0 {
			gtk.SetHAlign(sym, gtk.AlignCenter)
			gtk.SetVAlign(sym, gtk.AlignCenter)
			grid.Attach(sym, 1, row, 1, 1)
		}

		grid.Attach(dataCell(tempRange(d, units, l), 2), 2, row, 1, 1)
		grid.Attach(dataCell(windSpeed(d, windUnit, l), 3), 3, row, 1, 1)
		grid.Attach(dataCell(precip(d, l), 4), 4, row, 1, 1)

		row++
	}
	return uintptr(grid)
}

func dataCell(text string, col int) uintptr {
	cell := gtk.NewCell(text, colAlign[col])
	gtk.AddClass(cell, "cell")
	return cell
}

// symbolWidget rasterises the Weather Icons glyph for a condition code into a
// GtkImage.
//
// The symbol is monochrome and tinted with the palette's foreground on purpose:
// it was picked over the colour artwork, and a glyph drawn in the same ink as
// the numbers beside it belongs to the table rather than sitting on top of it.
// The glyph is rasterised at scale times the layout size and the GtkImage is
// told the scale, so a HiDPI screen draws real pixels instead of an upscale.
func symbolWidget(code, pt, scale int, col color.NRGBA) uintptr {
	if scale < 1 {
		scale = 1
	}
	img := fonts.Glyph(fonts.RuneForCode(code), pt*scale, col)
	if img == nil {
		return 0
	}
	b := img.Bounds()
	return gtk.NewImageRGBAScaled(img.Pix, b.Dx(), b.Dy(), img.Stride, scale)
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

// paletteFor picks the panel palette. For an explicit setting it honours the
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

// paletteForeground is the ink the palette draws cell text in, and therefore the
// colour a rasterised symbol has to be tinted with to look like part of the
// table.
//
// The value is taken from the stylesheet rather than from win.Foreground(),
// which is what the DESKTOP theme draws text in. Those two disagree whenever the
// user forces a palette the desktop does not share - a dark panel on a light
// theme would otherwise get a near-black symbol on a near-black row. Both
// literals below are the `label { color: ... }` declarations in style_linux.go
// and the text members of the Win32 palettes.
func paletteForeground(palette string) color.NRGBA {
	if palette == "dark" {
		return color.NRGBA{R: 0xf2, G: 0xf4, B: 0xf7, A: 0xff} // .dark label
	}
	return color.NRGBA{R: 0x14, G: 0x16, B: 0x1a, A: 0xff} // .light label
}

func luminance(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
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
