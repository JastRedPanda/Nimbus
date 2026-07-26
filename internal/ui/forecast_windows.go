//go:build windows

package ui

// The 7-day forecast panel, Win32 edition.
//
// It is the same design as the GTK panel in forecast_linux.go and style_linux.go:
// a borderless translucent sheet of rounded cards that opens at the work-area
// corner nearest the pointer and closes on Escape, on losing focus, or on its
// own close button. Everything the two backends share - the 620pt width, the
// 14pt page padding, the card radii, the type sizes, the palettes, the corner
// arithmetic - is carried across value for value, because the point of this
// file is that the two platforms look like one product.
//
// How it is built, in one line: the whole panel is composed in pure Go into a
// premultiplied image.RGBA, GDI is used for exactly one thing (turning strings
// into glyph coverage, in a scratch bitmap), the result is copied into a
// top-down 32bpp DIB section, and that is handed to UpdateLayeredWindow. GDI
// never touches the panel surface, so GDI can never destroy its alpha. See
// panelpaint_windows.go for why that rule exists.
//
// Window shape:   WS_POPUP. No WS_CAPTION, no WS_BORDER, no WS_THICKFRAME.
// Extended style: WS_EX_LAYERED | WS_EX_TOOLWINDOW | WS_EX_TOPMOST.
//                 NOT WS_EX_NOACTIVATE, which is the tempting wrong flag: it
//                 stops the window becoming foreground, so it never holds the
//                 keyboard focus, so Escape never arrives.
// Content:        one UpdateLayeredWindow call. There is no WM_PAINT handler
//                 and none is wanted - the system keeps the surface and
//                 repaints it itself.

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/lxn/win"
)

// Layout, in the same units the GTK stylesheet uses: logical pixels at 96 DPI,
// scaled to the window's real DPI at layout time. Every value here has a
// counterpart in style_linux.go or forecast_linux.go and is listed beside it.
const (
	panelWidthPt = 620 // forecastWidth
	panelEdgeGap = 12  // panelMargin: clearance from the work-area edges
	pagePad      = 14  // .page padding
	pageGapY     = 12  // page VBox spacing, header card to day strip

	cardRadiusPt = 14 // .card border-radius
	cardPadX     = 18 // .card padding, horizontal
	cardPadY     = 16 // .card padding, vertical
	cardGapY     = 6  // header card VBox spacing
	topRowGap    = 8  // header top HBox spacing, date to close button
	heroRowGap   = 10 // header hero HBox spacing

	dayRadiusPt = 12 // .day border-radius
	dayPadX     = 6  // .day padding, horizontal
	dayPadY     = 12 // .day padding, vertical
	dayGapY     = 6  // day card VBox spacing
	stripGap    = 8  // day strip HBox spacing
	dayBorderPt = 1  // .day border-width

	closePadX     = 8 // .close padding
	closePadY     = 2
	closeRadiusPt = 8 // .close border-radius

	// Icon sizes in layout points. Both double cleanly into an embedded asset
	// on a HiDPI screen (64 -> 128, 32 -> 64), which is why the header icon is
	// not larger: internal/wicons has no 256px artwork.
	heroIconPt = 64
	dayIconPt  = 32

	// Type sizes, in points, from style_linux.go.
	mutedPt   = 11 // .muted
	condPt    = 13 // .cond
	heroPt    = 34 // .hero
	dayTempPt = 11 // .daytemp
	closePt   = 13 // .close
)

// closeGlyph is the panel's own close affordance. With no title bar there is no
// system close button, so the header card supplies one, exactly as the GTK
// panel does.
const closeGlyph = "×" // MULTIPLICATION SIGN

const panelClassName = "NimbusForecastPanel"

// ---------------------------------------------------------------------------
// Palette
// ---------------------------------------------------------------------------

// panelPalette is the translucent half of the GTK stylesheet. Only the
// translucent variants are ported: a layered window always composites against
// the desktop, so there is no equivalent of the "no compositor" case the GTK
// sheet keeps a solid palette for.
type panelPalette struct {
	card      color.RGBA // .card / .day background, premultiplied
	today     color.RGBA // .day.today background
	border    color.RGBA // .day.today border-color
	hoverFill color.RGBA // .close:hover background
	text      [3]uint8   // label color
	muted     [3]uint8   // .muted color, also .close
	cond      [3]uint8   // .cond color
}

func darkPanelPalette() panelPalette {
	return panelPalette{
		card:      premul(28, 31, 38, 209),    // rgba(28,31,38,0.82)
		today:     premul(42, 47, 57, 224),    // rgba(42,47,57,0.88)
		border:    premul(111, 123, 141, 255), // #6f7b8d
		hoverFill: premul(255, 255, 255, 26),  // rgba(255,255,255,0.10)
		text:      [3]uint8{0xf2, 0xf4, 0xf7},
		muted:     [3]uint8{0x9a, 0xa3, 0xb0},
		cond:      [3]uint8{0xd6, 0xdb, 0xe3},
	}
}

func lightPanelPalette() panelPalette {
	return panelPalette{
		card:      premul(255, 255, 255, 224), // rgba(255,255,255,0.88)
		today:     premul(226, 233, 242, 240), // rgba(226,233,242,0.94)
		border:    premul(154, 166, 182, 255), // #9aa6b6
		hoverFill: premul(0, 0, 0, 20),        // rgba(0,0,0,0.08)
		text:      [3]uint8{0x14, 0x16, 0x1a},
		muted:     [3]uint8{0x5b, 0x64, 0x72},
		cond:      [3]uint8{0x38, 0x41, 0x4f},
	}
}

// ---------------------------------------------------------------------------
// Entry points
// ---------------------------------------------------------------------------

// showForecast opens the forecast panel. It returns immediately.
//
// The fetch runs on its own goroutine because the caller is the tray's single
// menu-dispatch loop, and a blocking ten-second HTTP call there would freeze
// Settings, About and Quit along with it. That is the same reason
// forecast_linux.go gives; the Win32 side simply never had the fix.
func showForecast(lat, lon float64, units, lang, theme, windUnit string) {
	// The pointer is sampled NOW, while the user's click is still fresh, rather
	// than when the window is finally built: the fetch in between can take up
	// to ten seconds, by which time the pointer may be on another monitor
	// entirely. GetCursorPos has no thread affinity, so reading it here costs
	// nothing.
	at, haveAt := pointerAnchor()

	go func() {
		l := i18n.ParseLang(lang)

		// Cheap early out, so a second click while the panel is open does not
		// spend ten seconds on a fetch whose result is going to be discarded.
		if presentPanel() {
			return
		}

		data, err := weather.FetchDaily(lat, lon)
		if err != nil || len(data) == 0 {
			if err != nil {
				log.Printf("forecast: fetch failed: %v", err)
			} else {
				log.Printf("forecast: fetch returned no days")
			}
			showError(forecastFailed(l))
			return
		}

		p := newPanel(data, units, windUnit, theme, l)
		p.run(at, haveAt)
	}()
}

// showError reports a failure with the only window that is guaranteed to work
// when something has already gone wrong.
//
// MessageBox runs its own modal message loop inside the call, so the goroutine
// stays on its OS thread for the whole of it and needs no LockOSThread.
func showError(msg string) {
	title := utf16Of("Nimbus")
	t := utf16Of(msg)
	win.MessageBox(0, &t[0], &title[0], win.MB_OK|win.MB_ICONERROR)
}

func forecastFailed(l i18n.Lang) string {
	if l == i18n.UK {
		return "Не вдалося завантажити прогноз."
	}
	return "Failed to load forecast."
}

// tempRange is the high/low pair the header and every day card show. Identical
// to the GTK backend's, including the unit conversion: Open-Meteo is asked for
// Celsius and Fahrenheit is derived here.
func tempRange(d weather.DailyForecast, units string) string {
	hi, lo := d.TempMax, d.TempMin
	if units == "fahrenheit" {
		hi = hi*9/5 + 32
		lo = lo*9/5 + 32
	}
	return fmt.Sprintf("%.0f°/%.0f°", hi, lo)
}

// ---------------------------------------------------------------------------
// The panel
// ---------------------------------------------------------------------------

type dayItem struct {
	name string
	temp string
	code int
	icon *image.RGBA
}

type dayGeom struct {
	card win.RECT
	name win.RECT
	icon win.POINT
	temp win.RECT
}

type panelGeom struct {
	header   win.RECT
	date     win.RECT
	closeBox win.RECT
	hero     win.RECT
	heroIcon win.POINT
	cond     win.RECT
	days     []dayGeom
}

type panelFonts struct {
	muted   win.HFONT
	cond    win.HFONT
	hero    win.HFONT
	dayTemp win.HFONT
	close   win.HFONT
}

type panel struct {
	hwnd  win.HWND
	inst  win.HINSTANCE
	title string

	pal panelPalette
	dpi int32

	date     string
	hero     string
	cond     string
	heroCode int
	heroIcon *image.RGBA
	days     []dayItem

	fonts panelFonts
	geom  panelGeom

	x, y, w, h int32

	img  *image.RGBA
	surf *surface
	mask *surface

	// armed is set by the first real activation. Until then a WA_INACTIVE must
	// be ignored: SetForegroundWindow can be refused, and an unarmed panel
	// would then vanish before the user ever saw it.
	armed bool
	// closing collapses the several ways the panel can be dismissed into one
	// WM_CLOSE, so a second trigger cannot destroy the window twice.
	closing  bool
	hover    bool
	tracking bool
}

func newPanel(data []weather.DailyForecast, units, windUnit, theme string, l i18n.Lang) *panel {
	// windUnit is part of the contract but the card design has no wind field:
	// the GTK panel dropped the wind and precipitation columns when it became a
	// card layout, and this is the same design. Named rather than blank so the
	// signature keeps documenting itself.
	_ = windUnit

	pal := lightPanelPalette()
	if resolveDark(theme) {
		pal = darkPanelPalette()
	}

	p := &panel{
		title:    l.ForecastTitle(),
		pal:      pal,
		date:     headerDate(data[0].Date, l),
		hero:     tempRange(data[0], units),
		cond:     l.Condition(data[0].WeatherCode),
		heroCode: data[0].WeatherCode,
	}
	for _, d := range data {
		p.days = append(p.days, dayItem{
			name: shortDay(d.Date, l),
			temp: tempRange(d, units),
			code: d.WeatherCode,
		})
	}
	return p
}

// ---------------------------------------------------------------------------
// Placement
// ---------------------------------------------------------------------------

// pointerAnchor samples where the user clicked. A false return - the pointer
// position could not be read at all - leaves the panel wherever Windows puts
// it, which is the same fallback the GTK backend takes on Wayland.
func pointerAnchor() (win.POINT, bool) {
	var pt win.POINT
	if !win.GetCursorPos(&pt) {
		return win.POINT{}, false
	}
	return pt, true
}

// pointerWork is the WORK AREA of the monitor under a sampled pointer.
//
// rcWork, not rcMonitor. The difference is exactly the taskbar, and ignoring it
// puts the panel underneath it - the same lesson forecast_linux.go records
// about desktop panels.
//
// It is read here, when the window is about to be built, rather than alongside
// the pointer: the fetch in between can take ten seconds, and a monitor can be
// plugged in or a taskbar moved in that time.
func pointerWork(pt win.POINT) (win.RECT, bool) {
	rc := win.RECT{Left: pt.X, Top: pt.Y, Right: pt.X + 1, Bottom: pt.Y + 1}
	mon := monitorFromRect(&rc, win.MONITOR_DEFAULTTONEAREST)
	if mon == 0 {
		return win.RECT{}, false
	}
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if !win.GetMonitorInfo(mon, &mi) {
		return win.RECT{}, false
	}
	return mi.RcWork, true
}

// panelCorner picks the work-area corner on the same side as the pointer, so
// the panel opens towards the middle of the screen rather than off its edge.
// Identical arithmetic to forecast_linux.go's corner(), with W/H swapped for
// Right-Left and Bottom-Top.
func panelCorner(px, py, w, h, margin int32, area win.RECT) (int32, int32) {
	aw := area.Right - area.Left
	ah := area.Bottom - area.Top

	x := area.Left + margin
	if px > area.Left+aw/2 {
		x = area.Right - w - margin
	}
	y := area.Top + margin
	if py > area.Top+ah/2 {
		y = area.Bottom - h - margin
	}
	// A panel that has been shrunk as far as it will go can still be larger
	// than the work area on a very small screen. Prefer showing its top-left.
	if x < area.Left {
		x = area.Left
	}
	if y < area.Top {
		y = area.Top
	}
	return x, y
}

// ---------------------------------------------------------------------------
// Singleton
// ---------------------------------------------------------------------------

// Only one panel exists at a time, the way the GTK backend presents the window
// it already has instead of stacking a second one.
var (
	panelMu   sync.Mutex
	panelBusy bool
	panelHWND win.HWND

	// live maps HWND to panel instead of parking a Go pointer in
	// GWLP_USERDATA. The usual idiom - pass the struct as CreateWindowEx's
	// lpParam, stash cs.CreateParams, cast it back on every message - hides a
	// Go heap pointer from the garbage collector and is what `go vet` calls
	// "possible misuse of unsafe.Pointer". A map costs one lookup per message
	// and needs no unsafe at all.
	live    = map[win.HWND]*panel{}
	pending *panel // set immediately before CreateWindowEx, claimed by WM_NCCREATE
)

func claimPanel() bool {
	panelMu.Lock()
	defer panelMu.Unlock()
	if panelBusy {
		return false
	}
	panelBusy = true
	return true
}

func releasePanel() {
	panelMu.Lock()
	panelBusy = false
	panelHWND = 0
	panelMu.Unlock()
}

// presentPanel raises the panel that is already open, if there is one, and
// reports whether it did.
func presentPanel() bool {
	panelMu.Lock()
	hwnd := panelHWND
	busy := panelBusy
	panelMu.Unlock()
	if !busy {
		return false
	}
	if hwnd != 0 {
		forceForeground(hwnd)
	}
	return true
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

var (
	panelClassOnce sync.Once
	panelClassOK   bool
)

func (p *panel) run(at win.POINT, haveAt bool) {
	// The claim comes before the thread is locked, so a duplicate open costs
	// nothing: a goroutine that exits while its thread is locked takes the
	// thread down with it.
	if !claimPanel() {
		presentPanel()
		return
	}
	defer releasePanel()

	// MANDATORY. A window's message queue belongs to the thread that created
	// the window. An unlocked goroutine can be rescheduled onto another OS
	// thread between loop iterations, after which GetMessage services a queue
	// that will never receive this window's messages and the panel is frozen.
	// PostQuitMessage has the same thread affinity. The thread is deliberately
	// never unlocked, so it is destroyed with the goroutine rather than handed
	// back to the scheduler carrying a message queue nobody will read.
	runtime.LockOSThread()

	p.inst = win.GetModuleHandle(nil)
	if p.inst == 0 {
		log.Printf("forecast: GetModuleHandle failed")
		return
	}

	panelClassOnce.Do(func() {
		cn := utf16Of(panelClassName)
		wc := &win.WNDCLASSEX{
			CbSize:      uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			LpfnWndProc: syscall.NewCallback(panelWndProc),
			HInstance:   p.inst,
			HCursor:     win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			// No class background brush and no CS_HREDRAW/CS_VREDRAW: nothing
			// about this window is painted by the class. Note also that
			// WS_EX_LAYERED is incompatible with CS_OWNDC and CS_CLASSDC.
			HbrBackground: 0,
			LpszClassName: &cn[0],
		}
		panelClassOK = win.RegisterClassEx(wc) != 0
	})
	if !panelClassOK {
		log.Printf("forecast: RegisterClassEx failed")
		return
	}

	// Create INVISIBLE. A layered window shows nothing until
	// UpdateLayeredWindow has been called for it, so creating it with
	// WS_VISIBLE only flashes an empty frame.
	panelMu.Lock()
	pending = p
	panelMu.Unlock()

	title := utf16Of(p.title)
	cn := utf16Of(panelClassName)
	p.hwnd = win.CreateWindowEx(
		win.WS_EX_LAYERED|win.WS_EX_TOOLWINDOW|win.WS_EX_TOPMOST,
		&cn[0], &title[0],
		win.WS_POPUP,
		0, 0, 1, 1,
		0, 0, p.inst, nil,
	)
	if p.hwnd == 0 {
		panelMu.Lock()
		pending = nil
		panelMu.Unlock()
		log.Printf("forecast: CreateWindowEx failed")
		return
	}

	work, haveWork := win.RECT{}, false
	if haveAt {
		work, haveWork = pointerWork(at)
	}

	if !p.build(work, haveWork) {
		// DestroyWindow first: its WM_NCDESTROY is what frees the GDI objects,
		// and release() is idempotent, so the second call is only insurance
		// against a window procedure that somehow never ran.
		win.DestroyWindow(p.hwnd)
		p.release()
		return
	}

	p.x, p.y = 0, 0
	if haveWork {
		p.x, p.y = panelCorner(at.X, at.Y, p.w, p.h, scaleDPI(panelEdgeGap, p.dpi), work)
	}

	p.paint()
	if !p.push() {
		win.DestroyWindow(p.hwnd)
		p.release()
		return
	}

	panelMu.Lock()
	panelHWND = p.hwnd
	panelMu.Unlock()

	win.ShowWindow(p.hwnd, win.SW_SHOW)
	if !forceForeground(p.hwnd) {
		// Worth saying out loud: without the foreground the panel holds no
		// keyboard focus, so Escape does nothing and it will never be told it
		// lost focus either. The close button is then the only way out, which
		// is precisely why the panel draws one.
		log.Printf("forecast: could not take the foreground; Escape and click-away will not work")
	}

	var msg win.MSG
	for {
		switch win.GetMessage(&msg, 0, 0, 0) {
		case 0: // WM_QUIT
			return
		case -1:
			log.Printf("forecast: GetMessage failed")
			return
		}
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
}

// push hands the finished surface to the compositor. The one call moves,
// resizes and repaints the window, which is why no SetWindowPos is needed.
func (p *panel) push() bool {
	if p.surf == nil {
		return false
	}
	ptDst := win.POINT{X: p.x, Y: p.y}
	ptSrc := win.POINT{X: 0, Y: 0}
	size := win.SIZE{CX: p.w, CY: p.h}
	blend := win.BLENDFUNCTION{
		BlendOp:    blendOpSrcOver,
		BlendFlags: 0,
		// 255 is mandatory, not merely sensible: "Set the SourceConstantAlpha
		// value to 255 (opaque) when you only want to use per-pixel alpha
		// values." Anything less multiplies the whole panel down again.
		SourceConstantAlpha: 255,
		AlphaFormat:         win.AC_SRC_ALPHA,
	}
	ok, errno := updateLayeredWindow(p.hwnd, 0, &ptDst, &size,
		p.surf.dc, &ptSrc, 0, &blend, ulwAlpha)
	if !ok {
		// A silent failure here produces an invisible window, which reads to
		// the user as "the menu item does nothing".
		log.Printf("forecast: UpdateLayeredWindow failed: %v", errno)
	}
	return ok
}

// release frees every GDI object the panel owns. Fonts are only ever selected
// into the mask DC for the duration of one drawTextGroup call and are always
// deselected afterwards, so none of them is still selected here - DeleteObject
// refuses to free a selected object, and a font that fails to delete leaks for
// the life of the process.
func (p *panel) release() {
	p.freeFonts()
	p.surf.dispose()
	p.mask.dispose()
	p.surf = nil
	p.mask = nil
	p.img = nil
}

func (p *panel) requestClose() {
	if p.closing {
		return
	}
	p.closing = true
	// PostMessage rather than a direct DestroyWindow: the dismissal triggers
	// include WM_ACTIVATE, and destroying a window from inside the system's own
	// activation handling is re-entrant in a way nothing documents as safe.
	win.PostMessage(p.hwnd, win.WM_CLOSE, 0, 0)
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

// build measures the content, sizes the window and allocates the surfaces.
//
// The loop exists because the layout is fixed-width: 620 units at 200% is 1240
// pixels, which does not fit a 1280-wide screen once the edge margins are paid
// for. Rather than let the panel run off the edge it is re-laid out at a lower
// effective DPI, which is the same trick layoutDPI performs for the settings
// window. Two corrections are enough for any real screen; the floor stops it
// shrinking the text into illegibility.
func (p *panel) build(work win.RECT, haveWork bool) bool {
	dpi := dpiOf(p.hwnd)

	for attempt := 0; ; attempt++ {
		p.dpi = dpi
		if !p.makeFonts() {
			log.Printf("forecast: CreateFontIndirect failed")
			return false
		}
		p.measure()

		if !haveWork || attempt >= 2 {
			break
		}
		gap := 2 * scaleDPI(panelEdgeGap, p.dpi)
		availW := work.Right - work.Left - gap
		availH := work.Bottom - work.Top - gap
		if availW <= 0 || availH <= 0 || (p.w <= availW && p.h <= availH) {
			break
		}
		next := dpi
		if p.w > availW {
			next = min(next, dpi*availW/p.w)
		}
		if p.h > availH {
			next = min(next, dpi*availH/p.h)
		}
		if next < minLayoutDPI {
			next = minLayoutDPI
		}
		if next >= dpi {
			break // no progress to be made; take what we have
		}
		p.freeFonts()
		dpi = next
	}

	p.img = image.NewRGBA(image.Rect(0, 0, int(p.w), int(p.h)))
	p.surf = newSurface(int(p.w), int(p.h))
	p.mask = newSurface(int(p.w), int(p.h))
	if p.surf == nil || p.mask == nil {
		log.Printf("forecast: CreateDIBSection failed for %dx%d", p.w, p.h)
		return false
	}
	return true
}

func (p *panel) makeFonts() bool {
	p.fonts = panelFonts{
		muted:   panelFont(mutedPt, win.FW_NORMAL, p.dpi),
		cond:    panelFont(condPt, win.FW_NORMAL, p.dpi),
		hero:    panelFont(heroPt, win.FW_LIGHT, p.dpi),
		dayTemp: panelFont(dayTempPt, win.FW_SEMIBOLD, p.dpi),
		close:   panelFont(closePt, win.FW_NORMAL, p.dpi),
	}
	f := p.fonts
	return f.muted != 0 && f.cond != 0 && f.hero != 0 && f.dayTemp != 0 && f.close != 0
}

func (p *panel) freeFonts() {
	for _, f := range []win.HFONT{p.fonts.muted, p.fonts.cond, p.fonts.hero, p.fonts.dayTemp, p.fonts.close} {
		if f != 0 {
			win.DeleteObject(win.HGDIOBJ(f))
		}
	}
	p.fonts = panelFonts{}
}

// measure computes every rectangle in the panel and the panel's own size.
//
// It follows the GTK box model exactly: a page with padding, a header card and
// a homogeneous strip of day cards, each card a vertical box of fixed-height
// rows separated by its spacing. Heights come from the font metrics rather than
// from constants, which is what stops a locale with taller glyphs from clipping.
func (p *panel) measure() {
	s := func(v int) int32 { return scaleDPI(v, p.dpi) }

	m := newMeasureDC()
	defer m.dispose()

	mutedH := m.lineHeight(p.fonts.muted)
	condH := m.lineHeight(p.fonts.cond)
	heroH := m.lineHeight(p.fonts.hero)
	tempH := m.lineHeight(p.fonts.dayTemp)
	closeH := m.lineHeight(p.fonts.close)

	heroW := m.width(p.hero, p.fonts.hero)
	closeW := m.width(closeGlyph, p.fonts.close)

	// Artwork. The icons are built here rather than once at construction
	// because their pixel size follows the DPI, and the DPI can still change
	// while build() is fitting the layout to the screen.
	heroIconPx := s(heroIconPt)
	p.heroIcon = panelIcon(p.heroCode, heroIconPx)
	if p.heroIcon == nil {
		heroIconPx = 0
	}
	dayIconPx := s(dayIconPt)
	for i := range p.days {
		p.days[i].icon = panelIcon(p.days[i].code, dayIconPx)
	}
	if len(p.days) > 0 && p.days[0].icon == nil {
		dayIconPx = 0
	}

	pad := s(pagePad)
	p.w = s(panelWidthPt)
	innerW := p.w - 2*pad
	contentL := pad + s(cardPadX)
	contentR := p.w - pad - s(cardPadX)

	closeBoxW := min(closeW+2*s(closePadX), contentR-contentL)
	closeBoxH := closeH + 2*s(closePadY)

	topH := max(mutedH, closeBoxH)
	heroRowH := max(heroH, heroIconPx, condH)
	headerH := 2*s(cardPadY) + topH + s(cardGapY) + heroRowH

	dayH := 2*s(dayPadY) + mutedH + s(dayGapY) + dayIconPx + s(dayGapY) + tempH
	if dayIconPx == 0 {
		dayH -= s(dayGapY)
	}

	p.h = 2*pad + headerH + s(pageGapY) + dayH

	g := panelGeom{}
	g.header = rectAt(pad, pad, innerW, headerH)

	topY := pad + s(cardPadY)
	g.closeBox = rectAt(contentR-closeBoxW, topY+(topH-closeBoxH)/2, closeBoxW, closeBoxH)
	g.date = win.RECT{
		Left:   contentL,
		Top:    topY,
		Right:  max(contentL, g.closeBox.Left-s(topRowGap)),
		Bottom: topY + topH,
	}

	rowY := topY + topH + s(cardGapY)
	g.hero = win.RECT{Left: contentL, Top: rowY, Right: min(contentL+heroW, contentR), Bottom: rowY + heroRowH}

	condL := g.hero.Right
	if heroIconPx > 0 {
		iconX := g.hero.Right + s(heroRowGap)
		if iconX+heroIconPx > contentR {
			// No room beside the temperature. Dropping the artwork is better
			// than overlapping it with the numbers.
			p.heroIcon = nil
			heroIconPx = 0
		} else {
			g.heroIcon = win.POINT{X: iconX, Y: rowY + (heroRowH-heroIconPx)/2}
			condL = iconX + heroIconPx
		}
	}
	g.cond = win.RECT{Left: min(condL+s(heroRowGap), contentR), Top: rowY, Right: contentR, Bottom: rowY + heroRowH}

	// The day strip is homogeneous. Positions are derived from a single
	// division so the rounding error is spread instead of accumulating into a
	// wider last card.
	stripY := pad + headerH + s(pageGapY)
	n := int32(len(p.days))
	gap := s(stripGap)
	for i := int32(0); i < n; i++ {
		x := pad + (innerW+gap)*i/n
		w := (innerW+gap)*(i+1)/n - gap - (innerW+gap)*i/n

		var d dayGeom
		d.card = rectAt(x, stripY, w, dayH)

		ty := stripY + s(dayPadY)
		inner := rectAt(x+s(dayPadX), ty, max(0, w-2*s(dayPadX)), mutedH)
		d.name = inner

		iy := ty + mutedH + s(dayGapY)
		if dayIconPx > 0 {
			d.icon = win.POINT{X: x + (w-dayIconPx)/2, Y: iy}
			iy += dayIconPx + s(dayGapY)
		}
		d.temp = rectAt(x+s(dayPadX), iy, max(0, w-2*s(dayPadX)), tempH)

		g.days = append(g.days, d)
	}

	p.geom = g
}

func rectAt(x, y, w, h int32) win.RECT {
	return win.RECT{Left: x, Top: y, Right: x + w, Bottom: y + h}
}

func rectContains(rc win.RECT, x, y int32) bool {
	return x >= rc.Left && x < rc.Right && y >= rc.Top && y < rc.Bottom
}

// ---------------------------------------------------------------------------
// Painting
// ---------------------------------------------------------------------------

func (p *panel) paint() {
	if p.img == nil || p.surf == nil {
		return
	}
	// Start fully transparent. Every pixel the design does not claim stays at
	// alpha 0, which is also what makes the gaps between the cards pass mouse
	// clicks through to the desktop.
	for i := range p.img.Pix {
		p.img.Pix[i] = 0
	}

	g := &p.geom
	s := func(v int) int32 { return scaleDPI(v, p.dpi) }

	roundRect(p.img, g.header.Left, g.header.Top,
		g.header.Right-g.header.Left, g.header.Bottom-g.header.Top,
		float32(s(cardRadiusPt)), p.pal.card)

	for i := range g.days {
		card := g.days[i].card
		w, h := card.Right-card.Left, card.Bottom-card.Top
		r := float32(s(dayRadiusPt))
		fill := p.pal.card
		if i == 0 {
			fill = p.pal.today
		}
		roundRect(p.img, card.Left, card.Top, w, h, r, fill)
		if i == 0 {
			// Today is outlined. The ring goes ON TOP of the finished fill; see
			// roundRing for why the other order ruins the translucency.
			roundRing(p.img, card.Left, card.Top, w, h, r, float32(s(dayBorderPt)), p.pal.border)
		}
		drawImage(p.img, p.days[i].icon, g.days[i].icon.X, g.days[i].icon.Y)
	}

	drawImage(p.img, p.heroIcon, g.heroIcon.X, g.heroIcon.Y)

	if p.hover {
		roundRect(p.img, g.closeBox.Left, g.closeBox.Top,
			g.closeBox.Right-g.closeBox.Left, g.closeBox.Bottom-g.closeBox.Top,
			float32(s(closeRadiusPt)), p.pal.hoverFill)
	}

	const centred = win.DT_CENTER | win.DT_VCENTER | win.DT_SINGLELINE | win.DT_NOPREFIX

	muted := []textRun{{
		text:  p.date,
		rect:  g.date,
		flags: win.DT_LEFT | win.DT_VCENTER | win.DT_SINGLELINE | win.DT_NOPREFIX | win.DT_END_ELLIPSIS,
		font:  p.fonts.muted,
	}}
	strong := []textRun{{
		text:  p.hero,
		rect:  g.hero,
		flags: win.DT_LEFT | win.DT_VCENTER | win.DT_SINGLELINE | win.DT_NOPREFIX,
		font:  p.fonts.hero,
	}}
	cond := []textRun{{
		text:  p.cond,
		rect:  g.cond,
		flags: win.DT_RIGHT | win.DT_VCENTER | win.DT_SINGLELINE | win.DT_NOPREFIX | win.DT_END_ELLIPSIS,
		font:  p.fonts.cond,
	}}
	for i := range g.days {
		muted = append(muted, textRun{
			text: p.days[i].name, rect: g.days[i].name, flags: centred, font: p.fonts.muted,
		})
		strong = append(strong, textRun{
			text: p.days[i].temp, rect: g.days[i].temp, flags: centred, font: p.fonts.dayTemp,
		})
	}

	drawTextGroup(p.img, p.mask, muted, p.pal.muted[0], p.pal.muted[1], p.pal.muted[2])
	drawTextGroup(p.img, p.mask, cond, p.pal.cond[0], p.pal.cond[1], p.pal.cond[2])
	drawTextGroup(p.img, p.mask, strong, p.pal.text[0], p.pal.text[1], p.pal.text[2])

	closeColour := p.pal.muted
	if p.hover {
		closeColour = p.pal.text
	}
	drawTextGroup(p.img, p.mask, []textRun{{
		text: closeGlyph, rect: g.closeBox, flags: centred, font: p.fonts.close,
	}}, closeColour[0], closeColour[1], closeColour[2])

	p.surf.blitFrom(p.img)
}

// repaint redraws and re-pushes the surface. UpdateLayeredWindow always updates
// the entire window, so there is no partial-invalidate path and no WM_PAINT.
func (p *panel) repaint() {
	p.paint()
	p.push()
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func panelWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	panelMu.Lock()
	if msg == win.WM_NCCREATE && pending != nil {
		live[hwnd] = pending
		pending = nil
	}
	p := live[hwnd]
	panelMu.Unlock()

	if p == nil || msg == win.WM_NCCREATE {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case win.WM_KEYDOWN:
		// A WS_POPUP top-level window with no child controls holds the focus
		// itself, so VK_ESCAPE arrives here directly. IsDialogMessage is
		// deliberately absent from the message loop: there are no controls to
		// navigate and it would swallow keys.
		if wParam == win.VK_ESCAPE {
			p.requestClose()
			return 0
		}

	case win.WM_ACTIVATE:
		if win.LOWORD(uint32(wParam)) == win.WA_INACTIVE {
			if p.armed {
				p.requestClose()
			}
			return 0
		}
		p.armed = true
		// MUST fall through to DefWindowProc. From the WM_ACTIVATE
		// documentation: "If the window is being activated and is not
		// minimized, the DefWindowProc function sets the keyboard focus to the
		// window." Returning 0 here - which the message's own "return zero if
		// you process this" line invites - leaves the panel focusless, so no
		// WM_KEYDOWN ever arrives and Escape silently does nothing while the
		// window still looks perfect.
		return win.DefWindowProc(hwnd, msg, wParam, lParam)

	case win.WM_MOUSEMOVE:
		x, y := clientPoint(lParam)
		if !p.tracking {
			// Without a leave notification the close button stays highlighted
			// after the pointer exits through a transparent gap, where no
			// further WM_MOUSEMOVE is ever delivered.
			tme := win.TRACKMOUSEEVENT{
				CbSize:    uint32(unsafe.Sizeof(win.TRACKMOUSEEVENT{})),
				DwFlags:   win.TME_LEAVE,
				HwndTrack: hwnd,
			}
			p.tracking = win.TrackMouseEvent(&tme)
		}
		if over := rectContains(p.geom.closeBox, x, y); over != p.hover {
			p.hover = over
			p.repaint()
		}
		return 0

	case win.WM_MOUSELEAVE:
		p.tracking = false
		if p.hover {
			p.hover = false
			p.repaint()
		}
		return 0

	case win.WM_LBUTTONUP:
		// Hit testing on a layered window follows the alpha channel: pixels
		// whose alpha is zero - the gaps between the cards, and the page
		// padding around them - let the click through to whatever is
		// underneath, which deactivates this window and closes it through
		// WM_ACTIVATE above. Only card pixels ever arrive here.
		x, y := clientPoint(lParam)
		if rectContains(p.geom.closeBox, x, y) {
			p.requestClose()
		}
		return 0

	case win.WM_ERASEBKGND:
		return 1

	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
		return 0

	case win.WM_DESTROY:
		win.PostQuitMessage(0)
		return 0

	case win.WM_NCDESTROY:
		// Last message the window ever gets, and the only point at which
		// nothing can still be referencing the GDI objects.
		p.release()
		panelMu.Lock()
		delete(live, hwnd)
		panelMu.Unlock()
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

// clientPoint unpacks the mouse coordinates from lParam. They are signed 16-bit
// and can be negative on a multi-monitor desktop, so the cast through int16
// matters.
func clientPoint(lParam uintptr) (int32, int32) {
	return int32(int16(win.LOWORD(uint32(lParam)))), int32(int16(win.HIWORD(uint32(lParam))))
}

// ---------------------------------------------------------------------------
// Foreground
// ---------------------------------------------------------------------------

// forceForeground gets the keyboard focus onto a panel opened from a tray menu.
//
// SetForegroundWindow is refused unless the caller satisfies one of a short
// list of conditions, the relevant one being "the calling process received the
// last input event". The user's click landed on the notification area, which
// belongs to Explorer, not to Nimbus - and by the time fyne.io/systray has
// delivered that click down a channel to the menu-dispatch goroutine, and the
// forecast has been fetched over the network, any grace the shell handed out
// has certainly lapsed. When it is refused the documented consolation is that
// Windows flashes the taskbar button, and a WS_EX_TOOLWINDOW has no taskbar
// button, so the refusal is completely silent.
//
// The AttachThreadInput dance is the standard workaround: while two threads
// share an input queue, one may set focus within the other's windows.
func forceForeground(hwnd win.HWND) bool {
	if win.SetForegroundWindow(hwnd) {
		win.SetFocus(hwnd)
		return true
	}

	fg := win.GetForegroundWindow()
	if fg == 0 {
		return false
	}
	other := win.GetWindowThreadProcessId(fg, nil)
	self := win.GetCurrentThreadId()
	if other == 0 || other == self {
		return false
	}

	// lxn/win types AttachThreadInput's thread ids as int32 where the API says
	// DWORD. The values are identical; only the Go type differs.
	win.AttachThreadInput(int32(self), int32(other), true)
	win.BringWindowToTop(hwnd)
	ok := win.SetForegroundWindow(hwnd)
	win.SetActiveWindow(hwnd)
	win.SetFocus(hwnd)
	win.AttachThreadInput(int32(self), int32(other), false)
	return ok
}
