//go:build linux && !qt

package ui

import (
	"image/color"
	"log"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/fonts"
	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/gui"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

// The 7-day forecast panel, GTK edition.
//
// The CONTENT is a plain five-column table: one header row, a rule, then seven
// data rows separated by hairlines. No summary header and no cards - that layout
// was tried and reverted. The CHROME around it is the panel work and stays: off
// the taskbar and the pager, above other windows, sticky across workspaces,
// draggable by its body, placed where the user last dragged it or else at the
// work-area corner nearest the click, and closed only deliberately: by the title
// bar's close button, or by the tray icon, which toggles it. Escape does not
// close it and neither does losing the focus - this is a window that stays where
// it was put until it is dismissed.
//
// The panel is an ordinary application window rather than a sheet floating over
// the desktop: the window manager's frame and title bar, opaque, square, and
// coloured by the desktop theme. That is why this file states no colour of its
// own - even the ink the weather symbols are tinted with is read back off the
// theme - and why the title bar's close button is routed through the panel's own
// dismissal, so that every exit reports the panel's position alike.
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

	forecastWindowID = "nimbus-forecast"

	// Clearance from the work-area edges the panel hugs.
	panelMargin = 12
)

// placeSettle is how long the panel waits before taking the window's position as
// the baseline a later title-bar drag is measured against. Long enough for a
// window manager to finish placing a window it has just been given, short enough
// that a user cannot drag the panel before it elapses.
const placeSettle = 400 * time.Millisecond

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

// Both are read and written only on the GTK thread.
//
// forecastDismiss is the open panel's own closer rather than a bare Destroy,
// because the panel's final position has to be read off the window before it is
// destroyed and only the closure built with the panel knows where to report it.
var (
	forecastWindow  gtk.Window
	forecastDismiss func()
	forecastPage    uintptr
	forecastReq  *gui.Forecast
	forecastLang i18n.Lang
	forecastStop chan struct{}
)

// closeOpenPanel makes the tray icon a toggle: if the panel is up, the click
// that would have opened it closes it instead. It reports whether it consumed
// the click, and must run on the GTK thread.
func closeOpenPanel() bool {
	if forecastWindow != 0 {
		if forecastDismiss != nil {
			forecastDismiss()
		} else {
			forecastWindow.Destroy()
		}
		return true
	}
	return false
}

// showForecast opens the 7-day forecast panel. It returns immediately: the
// forecast is fetched on its own goroutine because the caller is the tray's
// single menu-dispatch loop, and a blocking 10s HTTP call there would freeze
// Settings, About and Quit along with it.
func showForecast(req gui.Forecast) {
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

		l := i18n.ParseLang(req.Lang)

		_, cachedDaily := weather.Cached(req.Lat, req.Lon)
		if cachedDaily == nil {
			log.Print("forecast: no cached data, waiting for first fetch")
			select {
			case <-weather.UpdateCh:
				_, cachedDaily = weather.Cached(req.Lat, req.Lon)
			case <-time.After(5 * time.Second):
				log.Print("forecast: first fetch timed out, probing API directly")
				_, daily, err := weather.FetchAll(req.Lat, req.Lon)
				if err == nil && len(daily) > 0 {
					_, cachedDaily = weather.Cached(req.Lat, req.Lon)
				}
			}
			if cachedDaily == nil {
				gui.Current().Error("Nimbus", l.NetworkError())
				return
			}
		}

		if forecastWindow == 0 {
			gtk.Invoke(func() {
				if forecastWindow == 0 {
					buildForecast(cachedDaily, req, l, s.at)
				}
			})
		}

		if forecastStop != nil {
			close(forecastStop)
			forecastStop = nil
		}
		stop := make(chan struct{})
		forecastStop = stop
		go func() {
			for {
				select {
				case <-weather.UpdateCh:
					gtk.Invoke(func() {
						if forecastWindow == 0 {
							return
						}
						_, cachedDaily := weather.Cached(req.Lat, req.Lon)
						if cachedDaily != nil {
							updateForecast(cachedDaily)
						}
					})
				case <-stop:
					return
				}
			}
		}()
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

// updateForecast заменяет содержимое открытого окна новыми данными.
// Должна вызываться на GTK-треде.
func updateForecast(data []weather.DailyForecast) {
	if forecastWindow == 0 || forecastPage == 0 || forecastReq == nil {
		return
	}
	req := *forecastReq
	l := forecastLang

	gtk.ClearContainer(forecastPage)
	scale := forecastWindow.ScaleFactor()
	fg := forecastWindow.Foreground()
	grid := forecastTable(data, req.Units, req.WindUnit, l, scale, fg)
	gtk.PackStart(forecastPage, grid, false, false, 0)
	gtk.ShowAll(grid)
}

func buildForecast(data []weather.DailyForecast, req gui.Forecast, l i18n.Lang, at gtk.Rect) {
	if forecastWindow != 0 {
		forecastWindow.Present()
		return
	}

	// Останавливаем предыдущий тикер, если есть.
	if forecastStop != nil {
		close(forecastStop)
		forecastStop = nil
	}

	ensureAppIcon()
	// The theme option, applied to the process's own GtkSettings before the window
	// is built - "auto" leaves the property alone, so the desktop's preference
	// wins and the theme paints this window as it paints every other. The panel
	// states no colours of its own, so asking for the theme's dark variant is the
	// whole of what "dark" can mean here, and it is the same call About and the
	// settings window make.
	gtk.PreferDark(req.Theme)
	gtk.LoadCSS(forecastCSS)

	// closed says this panel is gone. It is declared before the window because the
	// destroy handler below is one of the things that writes it, and like the
	// package globals above it is read and written only on the GTK thread.
	//
	// It has two jobs. It makes dismiss idempotent, so a second exit cannot read a
	// position off a window that is no longer there or destroy it twice. And it is
	// what the placement timer further down asks before touching the window at
	// all: that timer fires a moment after the panel opens, which is easily long
	// enough for the user to have closed it again. It is set from the destroy
	// handler as well as from dismiss, so an exit that never went through
	// dismiss - the window manager's, or shutdown's - is recorded too.
	var closed bool

	win := gtk.NewFramedPanel(l.ForecastTitle(), forecastWidth, forecastHeight)
	win.OnDestroy(func() {
		closed = true
		forecastWindow = 0
		forecastDismiss = nil
		forecastPage = 0
		forecastReq = nil
		if forecastStop != nil {
			close(forecastStop)
			forecastStop = nil
		}
	})
	gtk.SetName(uintptr(win), forecastWindowID)

	// The panel states no palette of its own, which is what "coloured by the
	// desktop theme" is implemented as: the stylesheet names no colour anywhere,
	// so the theme paints the window, its labels and its separators.
	//
	// One class is added and it names no colour either. .system exists so the
	// header rule can outweigh the row hairlines: with the resets alone both fell
	// through to the theme's single separator colour, which collapsed two
	// deliberate weights into one - measured at 1.08:1 against the background
	// under Adwaita:dark, which is invisible. The sheet gives .system nothing but
	// a min-height, so the colour is still the theme's and only the thickness is
	// ours. The Win32 backend keeps the same hierarchy by giving the rule and the
	// hairlines two alphas of COLOR_3DSHADOW.
	gtk.AddClass(uintptr(win), "system")

	// fg is the ink the rasterised weather symbols are tinted with, and it is the
	// theme's own label colour rather than a literal of ours for the same reason:
	// the theme paints every label beside the symbols, so nothing else could match
	// them. req.Theme therefore does not reach the panel at all here - it still
	// drives the tray icon, and on Windows it still picks a palette.
	fg := win.Foreground()

	// What licenses reporting a position, and nothing less: a press was handed to
	// the window manager's move loop at least once during THIS showing, AND the
	// window is no longer where it was at that first handoff. handedOff and origin
	// are the two halves of that test.
	//
	// Neither half is enough alone. Reporting every close wrote down a corner
	// nobody chose on the first open-and-close of a user's very first forecast; the
	// tray reads a stored position as "put it back there", so pointer anchoring
	// never ran again for that user - permanently, and for every forecast
	// afterwards. But the handoff on its own is just as bad, because a bare click
	// on the panel body IS a handoff: the press goes to the window manager whether
	// or not the pointer then moves. So is a drag the user cancels with Escape,
	// which the window manager takes for itself and undoes by putting the window
	// back - and the position comparison is what makes that report nothing.
	//
	// Both positions are read the SAME way, through gtk.Window.Position, and never
	// against the placement placePanel asked for. Reading both through the same
	// call is what makes this correct on Wayland, where gtk_window_get_position
	// always answers 0,0: the two reads are then equal, so nothing is persisted,
	// which is the outcome we want there - a Wayland client cannot know its own
	// position, and 0,0 is not a position the user chose. forecast_windows.go
	// applies the identical rule through GetWindowRect.
	//
	// GTK thread only, like the package globals above: written from the
	// button-press handler, read from dismiss.
	handedOff := false
	origin := gui.Point{}

	// A second way to establish that origin is needed, because the most obvious
	// gesture - dragging the window by its title bar - never reaches the toolkit at
	// all. The window manager reparents a decorated window and handles the caption
	// itself, so no button-press-event is emitted, handedOff stays false, and a
	// panel the user dragged across the screen by its title bar reported nothing.
	// Measured under Marco: a title-bar drag from 13,43 to 163,153 wrote no
	// position, while a body drag of the same window did.
	//
	// So the window's settled placement is taken as the origin once, shortly after
	// it is mapped, and everything after that is a move. The delay is what makes it
	// a settled position rather than a transient one: the manager may adjust a
	// freshly mapped window, and a baseline read mid-adjustment would make the
	// adjustment itself look like something the user did.
	//
	// The Win32 backend has the same problem and solves it in the same spirit, by
	// capturing the origin in WM_ENTERSIZEMOVE, which is the caption drag's own
	// starting message.
	gtk.After(placeSettle, func() {
		if closed || handedOff {
			return
		}
		x, y := win.Position()
		handedOff, origin = true, gui.Point{X: x, Y: y}
	})

	// A press on the panel body starts a window-manager move, which is the second
	// way to drag the panel: the title bar the window manager draws is the first,
	// and it is the one this handler never sees - see the baseline above.
	//
	// Only the FIRST handoff records an origin. A later press happens wherever the
	// user has already dragged the panel to, so keeping the newest origin would
	// make a drag followed by one idle click report nothing at all. The callback
	// runs just before gtk_window_begin_move_drag, so this reads the position the
	// drag starts from.
	win.DragOnPress(func() {
		if handedOff {
			return
		}
		x, y := win.Position()
		handedOff, origin = true, gui.Point{X: x, Y: y}
	})

	// dismiss is the panel's single exit: the title bar's close button and the
	// tray toggle both close it through here, which is the only reason the
	// position gets reported at all.
	//
	// The order inside it is the whole point. The position has to be read while
	// the window still exists: gtk_widget_destroy takes the GdkWindow with it, and
	// gtk.Window.Position on what is left answers with garbage. That is why the
	// read cannot live in the destroy handler above - by the time that runs there
	// is nothing left to ask. A destroy that arrives from outside this file, from
	// the window manager or at shutdown, therefore reports no move, which is
	// correct: an unread position is better than a wrong one.
	//
	// The coordinates reported are the ones read HERE, at close, not the origin
	// recorded at the handoff: the origin exists only to be compared against.
	//
	// The read stays here; only the delivery moves off the GTK thread. OnMove
	// writes the configuration file, and a disk write inside the GTK main loop
	// stalls every window this process owns - on the tray-toggle path it would
	// stall inside a gtk.Invoke as well, with the panel still visibly on screen
	// for as long as the disk takes. The Win32 backend hands OnMove to a
	// goroutine the same way, so the callback has one contract on both platforms:
	// arbitrary goroutine, coordinates already read, free to take locks and do
	// I/O.
	//
	// The guard on the front is what makes it idempotent, and that is not
	// decoration: a second pass would read Position off a destroyed window -
	// garbage - and then report it as a position the user chose, or destroy the
	// window twice.
	dismiss := func() {
		if closed {
			return
		}
		closed = true
		if req.OnMove != nil && handedOff {
			if x, y := win.Position(); x != origin.X || y != origin.Y {
				onMove := req.OnMove
				go onMove(x, y)
			}
		}
		win.Destroy()
	}

	// The title bar's close button is one of the two ways this panel gets closed -
	// the tray icon is the other - so it has to run the panel's own dismissal
	// rather than GTK's default destroy: the default reports nothing, which would
	// mean a panel the user dragged somewhere almost never remembers where it was
	// dropped. It is handed dismiss bare, with no guard in front of it, because a
	// close button that could decline to close is not a close button.
	//
	// A close arriving from outside the window - a session shutdown, a wmctrl -c -
	// comes through here too and reports a position as well. That is sound where
	// the destroy handler is not: delete-event is delivered while the window is
	// still there to be asked.
	win.OnDeleteEvent(dismiss)

	scale := win.ScaleFactor()

	// One child, so there is nothing to space. The box is here because .page is
	// where the panel's padding is stated.
	page := gtk.NewVBox(0)
	gtk.AddClass(page, "page")
	win.Add(page)
	forecastPage = page
	forecastReq = &req
	forecastLang = l

	gtk.PackStart(page, forecastTable(data, req.Units, req.WindUnit, l, scale, fg), false, false, 0)

	placePanel(win, page, at, req.At)
	forecastWindow = win
	forecastDismiss = dismiss
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

		grid.Attach(dataCell(weather.TempRange(d, units, l), 2), 2, row, 1, 1)
		grid.Attach(dataCell(weather.WindSpeed(d, windUnit, l), 3), 3, row, 1, 1)
		grid.Attach(dataCell(weather.Precip(d, l), 4), 4, row, 1, 1)

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
// The symbol is monochrome and tinted with the theme's own ink on purpose: it
// was picked over the colour artwork, and a glyph drawn in the same ink as
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

// placePanel shows the panel where the user last dragged it, or at the work-area
// corner nearest the click.
//
// The order matters and is not the obvious one. A content-hugging layout has no
// size until its children are visible, and neither the preferred size of an
// unrealised window nor the size reported after a bare realise tells the truth -
// both under-report, and the window manager then quietly clamps the window to
// the screen edge, which hides the bad arithmetic behind a result that looks
// almost right. Showing the CONTENT while the toplevel is still unmapped gives
// GTK everything it needs to measure and leaves the window invisible and free
// to move. Both placements need that real size, so both come after ShowContent.
func placePanel(win gtk.Window, content uintptr, at gtk.Rect, want *gui.Point) {
	win.ShowContent(content)
	if x, y, ok := panelOrigin(win, at, want); ok {
		win.Move(x, y)
	}
	win.Show()
}

// panelOrigin decides the top-left corner: the remembered position when it is
// still on a monitor, otherwise the corner nearest the click. A false return
// means leave the window wherever the window manager put it, which is all that
// can be done when neither position is usable - on Wayland, for instance, where
// a client sees no global coordinates at all.
//
// It reads the window size, so it must be called after ShowContent.
func panelOrigin(win gtk.Window, at gtk.Rect, want *gui.Point) (int, int, bool) {
	w, h := win.Size()

	if want != nil {
		if area, ok := rememberedArea(want.X, want.Y); ok {
			x, y := clampToArea(want.X, want.Y, w, h, area)
			return x, y, true
		}
		// Worth a line: the panel is about to ignore a position the user chose
		// by dragging it, and the reason is invisible from the outside.
		log.Printf("forecast: remembered position %d,%d is on no current monitor, using the corner nearest the pointer", want.X, want.Y)
	}

	if at.W <= 0 || at.H <= 0 {
		return 0, 0, false
	}
	area, ok := gtk.WorkAreaAt(at.X, at.Y)
	if !ok {
		return 0, 0, false
	}
	x, y := corner(at.X, at.Y, w, h, area)
	return x, y, true
}

// rememberedArea answers the two questions a remembered position raises, in the
// order they have to be asked: is that point still on a monitor at all, and if it
// is, which usable rectangle should the panel be clamped into. False means no
// monitor contains it - the display it was saved on is gone, or the screens were
// rearranged - and the caller falls back to the pointer corner.
//
// Containment is tested against monitor GEOMETRY and clamping done against the
// WORK AREA, and the two being different rectangles is the point. Testing
// containment against the work area is what made the backends disagree: Win32
// asks MonitorFromRect, which is geometry with the taskbar included, so a panel
// dropped with its top-left corner over a dock or a top panel was clamped back
// into view on Windows and silently forgotten here.
//
// Must be called on the GTK thread - both calls reach into GDK.
func rememberedArea(x, y int) (gtk.Rect, bool) {
	mon, ok := gtk.GeometryAt(x, y)
	if !ok || !inside(mon, x, y) {
		return gtk.Rect{}, false
	}
	return gtk.WorkAreaAt(x, y)
}

// inside reports whether a point lies in a rectangle.
//
// It exists because gtk.GeometryAt cannot answer "no monitor here":
// gdk_display_get_monitor_at_point falls back to the NEAREST monitor when the
// point is off every screen, so a position remembered on a monitor that has
// since been unplugged comes back as a perfectly valid rectangle belonging to
// some other monitor. Testing the point against the rectangle we were given is
// what turns that into the "no monitor contains it" answer the caller needs.
func inside(r gtk.Rect, x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// clampToArea slides a remembered origin until the whole panel is inside the
// work area. The area is not necessarily the one the position was saved on: a
// resolution change, a dock appearing or a monitor being rearranged can all
// leave a position that was legitimate when it was written with most of the
// panel hanging off an edge, and a panel whose title bar is off-screen cannot be
// closed from the window at all - the close button on it is one of only two ways
// out.
//
// The lower bounds are applied last on purpose. When the panel is larger than
// the work area - a small screen with a large font scale - the upper bound comes
// out below the lower one, and the top-left corner is then the only answer worth
// having: the header row and the first days stay readable, whereas honouring the
// bottom edge would push exactly those off the top.
func clampToArea(x, y, w, h int, area gtk.Rect) (int, int) {
	if x+w > area.X+area.W {
		x = area.X + area.W - w
	}
	if y+h > area.Y+area.H {
		y = area.Y + area.H - h
	}
	if x < area.X {
		x = area.X
	}
	if y < area.Y {
		y = area.Y
	}
	return x, y
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
	// Clamped to the work area's own origin, which is what forecast_windows.go's
	// panelCorner does and this did not. A panel wider or taller than the work area
	// - a small screen, or a text-scaling factor that grows the table - otherwise
	// gets a negative origin from the arithmetic above and opens with its header row
	// and first days off the top or left edge, where nothing clamps it afterwards.
	// Showing the top-left corner of an oversized panel is the lesser evil.
	if x < area.X {
		x = area.X
	}
	if y < area.Y {
		y = area.Y
	}
	return x, y
}
