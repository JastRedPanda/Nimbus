//go:build windows

package ui

// The 7-day forecast panel, Win32 edition.
//
// The CONTENT is a plain five-column table: one header row, a rule, then seven
// data rows separated by hairlines. No summary header and no cards - that layout
// was tried and reverted.
//
// The WINDOW is an ordinary application window, coloured by the desktop, kept
// above the others, off the taskbar, placed at the work-area corner nearest the
// click - or at the position the caller remembers - and draggable by its body as
// well as by its caption.
//
//	Window style:   WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU. A title bar with
//	                the window manager's own close button, square corners,
//	                opaque. WS_SYSMENU is what puts the close button in the
//	                caption; WS_CAPTION alone draws a bar with nothing in it.
//	Extended style: WS_EX_TOPMOST, and deliberately NOT WS_EX_TOOLWINDOW, which
//	                was here and was wrong. That flag draws the thin
//	                palette-window caption with a small close button, so the one
//	                control this window offers did not look like the one every
//	                other window on the desktop offers. The caption is now the
//	                ordinary one, the same the About box gets. WS_EX_TOOLWINDOW
//	                was also what kept the panel off the taskbar and out of
//	                Alt+Tab; an invisible owner window does that now instead -
//	                see makeOwner.
//	Colours:        GetSysColor, with one documented exception that is a Windows
//	                wart rather than a choice - see panelPaletteSystem.
//	Closing:        the caption's close button, or another click on the tray
//	                icon, and NOTHING ELSE. Both arrive as WM_CLOSE, which is
//	                what makes the position report a single path. Escape and
//	                focus loss deliberately do nothing: this is a window that
//	                stays until it is closed on purpose, not a popup that
//	                evaporates when the user looks away.
//
// Every metric is shared with the GTK backend value for value: the constants
// below carry the same names and numbers as the ones in forecast_linux.go and
// the stylesheet in style_linux.go, and the column widths are distributed the
// way a GtkGrid of expanding cells distributes them. The point of the two files
// is that the two platforms look like one product.
//
// HOW THE PIXELS GET THERE. Ordinary GDI, the same calls about_windows.go makes:
// FillRect for the sheet, the header rule and the row hairlines, DrawTextEx for
// every caption, cell and weather symbol. It happens ONCE, in p.paint, into an
// off-screen buffer exactly the size of the client area; WM_PAINT is a single
// BitBlt of that buffer and nothing else. The content is a snapshot of one fetch
// in a window that cannot be resized, so there is never anything to redraw.
//
// The panel used to be a WS_EX_LAYERED sheet fed to UpdateLayeredWindow, whose
// premultiplied alpha any GDI call would have destroyed, and was therefore
// composed by hand in Go - see panelpaint_windows.go for what that took. Both
// are gone. What outlives them is the palette: two of its colours are stated as
// white at a low alpha, and GDI has no alpha, so they are flattened against the
// sheet colour once, where the palette is built. See panelPaletteSystem.
//
// Dragging:       WM_LBUTTONDOWN hands a press on the body to the system's own
//                 move loop. NOT a WM_NCHITTEST that answers HTCAPTION, which
//                 looks like the tidier trick - panelWndProc spells the reason
//                 out. The caption starts the very same loop on its own, without
//                 this code seeing a press at all, which is why the origin the
//                 position report compares against is recorded from
//                 WM_ENTERSIZEMOVE as well as from WM_LBUTTONDOWN.

import (
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/JastRedPanda/Nimbus/internal/fonts"
	"github.com/JastRedPanda/Nimbus/internal/gui"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/lxn/win"
)

// Layout, in the same units the GTK side uses: logical pixels at 96 DPI, scaled
// to the window's real DPI at layout time. The counterpart of each value in
// forecast_linux.go or style_linux.go is named beside it.
const (
	panelWidthPt = 620 // forecastWidth
	panelEdgeGap = 12  // panelMargin: clearance from the work-area edges
	pagePad      = 14  // .page padding: sides and bottom in the stylesheet
	// The stylesheet's fourth value, the smaller padding it puts at the top of
	// the page. layoutTable spends it at the BOTTOM instead and starts the table
	// pagePad down from the top of the client area, so the split is the mirror of
	// the stylesheet's - while the panel's height, which is what the shared
	// metrics are really about, is the same sum.
	pagePadTop = 6

	rowGapY = 6  // grid row spacing
	colGapX = 18 // grid column spacing

	theadPt = 11 // .thead font-size, weight 600
	cellPt  = 11 // .cell font-size

	// symbolPt is the size of the weather symbol in layout PIXELS, not in
	// typographic points - it is the value fonts.Glyph is handed on the GTK
	// side, where it sizes the rasterised ink. Here it is the font's em height,
	// because these glyphs draw at very close to one em (0.75 to 1.11 in this
	// typeface, 1.02 on average), so an em of symbolPt puts the ink at about
	// symbolPt and the two backends agree.
	symbolPt = 20

	// rulePt is the thickness of the header rule and of the hairlines between
	// data rows: `separator { min-height: 1px }`.
	rulePt = 1

	// numCols is the width of i18n.ForecastHeaders(). It is a constant rather
	// than len(headers) because the geometry is an array: a locale that
	// returned a different number of captions would otherwise index past it.
	numCols = 5
	// colCond is the column drawn in the symbol face instead of the text face.
	colCond = 1
)

// colAlign is the horizontal alignment of each table column, applied to both the
// header caption and the cells under it so a column reads as one thing. Day
// reads as a label and sits left; the symbol is centred in its column; the three
// numeric columns are right aligned so their digits line up, which is the whole
// reason a table beats a row of cards.
var colAlign = [numCols]uint32{
	win.DT_LEFT,   // Day
	win.DT_CENTER, // Condition
	win.DT_RIGHT,  // Temp
	win.DT_RIGHT,  // Wind
	win.DT_RIGHT,  // Precip
}

// weatherIconsFace is the family name inside internal/fonts/weathericons.ttf
// (name ID 1), which is what CreateFontIndirect matches lfFaceName against.
// internal/fonts registers the file with GDI privately; it does not name the
// family, so the name is repeated here and verified at runtime - see makeFonts.
const weatherIconsFace = "Weather Icons"

const panelClassName = "NimbusForecastPanel"

// wmRefresh — пользовательское сообщение для обновления данных панели.
const wmRefresh = win.WM_APP + 1

// The window's shape, named rather than spelled out at the CreateWindowEx call,
// because panel.frame has to be asked about the very styles the window was
// created with - BOTH of them. Ask about different ones and the client area comes
// out the wrong size, which shows up either as a caption eating the header row or
// as a strip of uninitialised memory along the bottom, neither of which looks
// like the sizing bug it is.
//
// Deliberately NOT resizable: no WS_THICKFRAME and no WS_MAXIMIZEBOX. The layout
// is fixed-width by design, and those flags would also hand the window Aero Snap,
// which would fling a 620-unit panel across half the screen the first time a user
// dragged it near an edge.
const (
	panelStyle   = uint32(win.WS_OVERLAPPED | win.WS_CAPTION | win.WS_SYSMENU)
	panelExStyle = uint32(win.WS_EX_TOPMOST)
)

// ---------------------------------------------------------------------------
// Palette
// ---------------------------------------------------------------------------

// panelPalette is the panel's colours, ready for CreateSolidBrush and
// SetTextColor. They are the desktop's, not the app's - see panelPaletteSystem.
//
// EVERY ONE OF THEM IS A SOLID COLORREF, and two of them were not always. The
// header rule and the row hairlines are stated in the design, and on the GTK
// side, as a colour at a fraction of full strength - white at alpha 71 and alpha
// 26 over the sheet. GDI has no alpha: FillRect takes a solid brush and nothing
// else, and there is no compositing step left to apply one. So the two are
// blended against the sheet colour once, in panelPaletteSystem, and what is
// stored here is the flat result. Drawing them as stated would paint a white
// line across the table where a whisper of one belongs.
//
// There is no separate card colour because there is no card. The window IS the
// sheet: one background with pagePad of padding inside it, which is what `.page`
// states on the GTK side.
type panelPalette struct {
	sheet win.COLORREF // the client area's background
	rule  win.COLORREF // .rule background, under the captions
	sep   win.COLORREF // separator background, between data rows
	text  win.COLORREF // label color: the cells and the symbols
	thead win.COLORREF // .thead color
}

// highContrastOn reports whether a high contrast scheme is active.
//
// It is asked before the dark-apps switch: a high contrast scheme is expressed
// entirely in the classic system colours, so it is the one case where GetSysColor
// is the right answer even though Windows also reports the apps as dark.
func highContrastOn() bool {
	hc := win.HIGHCONTRAST{CbSize: uint32(unsafe.Sizeof(win.HIGHCONTRAST{}))}
	if !win.SystemParametersInfo(win.SPI_GETHIGHCONTRAST, hc.CbSize, unsafe.Pointer(&hc), 0) {
		// Not an error worth a log line: every Windows this runs on answers it, and
		// a machine that does not is not in high contrast either.
		return false
	}
	return hc.DwFlags&win.HCF_HIGHCONTRASTON != 0
}

// panelPaletteSystem builds the panel's palette out of the desktop's own
// colours, so it matches the settings and About windows and everything else on
// the screen instead of carrying the app's own design into a window whose frame
// the desktop paints.
//
// THE ONE WART, and the reason this is not four lines of GetSysColor.
// GetSysColor predates dark mode entirely: it answers from the classic colour
// scheme, the one SetSysColors and High Contrast drive, and it was never wired
// to the Settings > Personalisation > Colours "Choose your mode" switch. On a
// machine set to dark apps it still reports white for COLOR_WINDOW and black for
// COLOR_WINDOWTEXT, so a panel built from it would be a white rectangle in the
// middle of a dark desktop. Microsoft's own guidance for dark mode in Win32 says
// as much: use DwmSetWindowAttribute for the frame and substitute your own
// colours for the client area. This package already reached that conclusion once
// - createDarkBrush and handleCtlColor hardcode their greys for the settings
// window rather than ask the system - and this is the same workaround in the same
// package. Do not "simplify" it into a bare GetSysColor.
//
// So: when Windows says dark apps, the app's own solid dark palette is used, and
// the caption is darkened with DWMWA_USE_IMMERSIVE_DARK_MODE to match. Otherwise
// GetSysColor is trusted, which is not merely the light case.
//
// High contrast is the exception to the exception, and it is checked FIRST. A high
// contrast scheme - Black, White, or one the user built - lives entirely in the
// classic colours, so GetSysColor is exactly right there and substituting the
// app's greys is exactly wrong: it would be the one configuration where a user has
// explicitly told the system what colours they need in order to read the screen,
// and the app would answer with its own. High Contrast Black also sets apps to
// dark, so without this check the dark branch would swallow it.
func panelPaletteSystem(dark bool) panelPalette {
	if dark {
		// The colours the rest of the program paints on a dark Windows. The panel
		// is supposed to disappear into the desktop, and on a dark desktop the
		// closest thing to the desktop this program can honestly claim is what it
		// paints its other windows with. A palette of its own is what used to be
		// here, and a screenshot showed it up at once: the panel sat visibly
		// darker and cooler than the About window next to it.
		//
		// Asked from darkmode_windows.go rather than written out here, because
		// About paints from the same constants - see the note there for why they
		// have to be constants at all rather than a GetSysColor call.
		sheet := win.COLORREF(darkSurface)
		return panelPalette{
			sheet: sheet,
			// 0.28 and 0.10 of white, which is a RELATIVE statement about which
			// line is the header rule and which is a row hairline rather than a
			// colour of the app's own. The pair reads the same over any dark
			// surface, so it needs nothing from the theme.
			//
			// Alpha 71 and alpha 26 are those two fractions of 255, and they are
			// SPENT HERE: what the table is actually painted with is the two greys
			// they come to over darkSurface. See panelPalette.
			rule:  blendOver(win.RGB(255, 255, 255), 71, sheet),
			sep:   blendOver(win.RGB(255, 255, 255), 26, sheet),
			text:  win.COLORREF(darkText),
			thead: win.COLORREF(darkTextDim),
		}
	}

	sheet := win.COLORREF(win.GetSysColor(win.COLOR_WINDOW))
	// COLOR_3DSHADOW is the theme's own divider shade - the line an etched border
	// is drawn with - which is what the header rule is, at full strength.
	shadow := win.COLORREF(win.GetSysColor(win.COLOR_3DSHADOW))

	return panelPalette{
		sheet: sheet,
		rule:  shadow,
		// The hairlines between data rows keep the same shade at the share of it
		// the dark branch above gives them, 0.10 of 0.28, so the two weights stay
		// distinguishable instead of collapsing into one flat grid. That share is
		// alpha 91, and like the dark branch's it is blended into the sheet here
		// rather than carried into the painting.
		sep:   blendOver(shadow, 91, sheet),
		text:  win.COLORREF(win.GetSysColor(win.COLOR_WINDOWTEXT)),
		thead: win.COLORREF(win.GetSysColor(win.COLOR_GRAYTEXT)),
	}
}

// blendOver flattens a colour the design states with an alpha into the solid one
// GDI can draw: src laid over the opaque colour beneath it at alpha/255.
//
// It is all that survives of the premultiplied compositing the panel used to do:
// three colours once per panel, rather than a million pixels once per paint. It
// is exact for the only case that reaches it, because the backdrop really is
// opaque - it is the sheet the line is painted on.
func blendOver(src win.COLORREF, alpha uint8, under win.COLORREF) win.COLORREF {
	sr, sg, sb := refRGB(uint32(src))
	ur, ug, ub := refRGB(uint32(under))
	mix := func(s, u uint8) byte {
		return byte((uint32(s)*uint32(alpha) + uint32(u)*uint32(255-alpha)) / 255)
	}
	return win.RGB(mix(sr, ur), mix(sg, ug), mix(sb, ub))
}

// refRGB unpacks a COLORREF, which is 0x00bbggrr: the byte order is the opposite
// of the way the value is usually written down - which is invisible for the greys
// in darkmode_windows.go and would bite the first time one of them is not grey.
func refRGB(c uint32) (r, g, b uint8) {
	return uint8(c), uint8(c >> 8), uint8(c >> 16)
}

// ---------------------------------------------------------------------------
// Entry points
// ---------------------------------------------------------------------------

// showForecast opens the forecast panel. It returns immediately.
//
// The fetch runs on its own goroutine because the caller is the tray's single
// menu-dispatch loop, and a blocking ten-second HTTP call there would freeze
// Settings, About and Quit along with it.
func showForecast(req gui.Forecast) {
	// The pointer is sampled NOW, while the user's click is still fresh, rather
	// than when the window is finally built: the fetch in between can take up
	// to ten seconds, by which time the pointer may be on another monitor
	// entirely. GetCursorPos has no thread affinity, so reading it here costs
	// nothing.
	//
	// It is sampled even when req.At says where the panel goes, because At is
	// only honoured if a monitor still contains it: the pointer is the fallback
	// for the display that was unplugged since the position was remembered.
	at, haveAt := pointerAnchor()

	go func() {
		l := i18n.ParseLang(req.Lang)

		if closeOpenPanel() {
			return
		}

		_, cachedDaily := weather.Cached(req.Lat, req.Lon)
		if cachedDaily == nil {
			log.Print("forecast: no cached data, waiting for first fetch")
			select {
			case <-weather.UpdateCh:
				_, cachedDaily = weather.Cached(req.Lat, req.Lon)
			case <-time.After(5 * time.Second):
				log.Print("forecast: timed out waiting for first weather data")
				return
			}
			if cachedDaily == nil {
				return
			}
			if closeOpenPanel() {
				return
			}
		}

		p := newPanel(cachedDaily, req, l)
		go p.run(at, haveAt)
	}()
}

// showError reports a failure with the only window that is guaranteed to work
// when something has already gone wrong.
//
// MessageBox runs its own modal message loop inside the call, so the goroutine
// stays on its OS thread for the whole of it and needs no LockOSThread.
// showError puts up one error box at a time, and drops the rest.
//
// MessageBox blocks the thread that calls it, so one thread cannot stack two -
// but the panel, the settings window and the tray all run on threads of their own
// and each can call this. Errors here arrive in bursts, one per failed fetch, and
// a weather service that is down fails every attempt, so without the guard a
// handful of clicks leaves a pile of identical boxes to dismiss one by one. The
// GTK backend keeps a single dialog for the same reason.
//
// Owner is 0 deliberately: an owned box would be modal for a window on another
// thread, which is a deadlock waiting to be found.
func showError(msg string) {
	if !errorBoxFree.CompareAndSwap(true, false) {
		log.Printf("forecast: an error box is already on screen, dropping: %s", msg)
		return
	}
	defer errorBoxFree.Store(true)
	title := utf16Of("Nimbus")
	t := utf16Of(msg)
	win.MessageBox(0, &t[0], &title[0], win.MB_OK|win.MB_ICONERROR)
}

// errorBoxFree is true when no error box is on screen. It starts true, which is
// what the init below is for - the zero value of atomic.Bool is false.
var errorBoxFree atomic.Bool

func init() { errorBoxFree.Store(true) }

// ---------------------------------------------------------------------------
// The panel
// ---------------------------------------------------------------------------

// tableRow is one day, already formatted. cell[colCond] holds the Weather Icons
// codepoint rather than a word, and is the one cell drawn in the symbol face.
type tableRow struct {
	cell [numCols]string
}

type rowGeom struct {
	cell [numCols]win.RECT
	// sep is the hairline BELOW this row, and is the zero RECT for the last one:
	// the hairlines go between the data rows, not after them.
	sep win.RECT
}

type panelGeom struct {
	sheet win.RECT
	head  [numCols]win.RECT
	rule  win.RECT
	rows  []rowGeom
}

type panelFonts struct {
	thead  win.HFONT
	cell   win.HFONT
	symbol win.HFONT // 0 when the Weather Icons face is not usable
}

// panelBrushes are the three fills the table is made of, created once with the
// panel and destroyed with it rather than made and thrown away per WM_PAINT.
// This program sits in a tray for weeks: a brush leaked per repaint is a handle
// count that climbs all afternoon.
type panelBrushes struct {
	sheet win.HBRUSH
	rule  win.HBRUSH
	sep   win.HBRUSH
}

// panelMetrics is everything the layout needs to know about the fonts and the
// strings, so that the arithmetic in layoutTable is a pure function of it.
type panelMetrics struct {
	theadH int32
	cellH  int32
	// symbolPx is the symbol's box, and 0 when there is no symbol font. It is
	// the row's floor the way the GtkImage's height is on the GTK side.
	symbolPx int32
	// natural is the widest caption-or-cell in each column, which is what a
	// GtkGrid column requests before any slack is handed out.
	natural [numCols]int32
}

type panel struct {
	hwnd  win.HWND
	inst  win.HINSTANCE
	title string

	// req is kept whole rather than unpacked into fields because one of its
	// members is a callback that must be called LATER: OnMove, as the window
	// closes. The rest of it - the coordinates, the units, the remembered
	// position - is read during construction and placement.
	req gui.Forecast

	// dark says draw dark, and it is the DESKTOP's answer rather than the theme
	// option's - see newPanel.
	dark bool
	pal  panelPalette
	dpi  int32

	// frameW and frameH are the caption and borders, measured once in build.
	frameW, frameH int32

	heads []string
	rows  []tableRow

	// haveSymbols records whether the embedded typeface was registered with
	// GDI. Without it the symbol column stays blank rather than filling with
	// whatever glyphs a substitute face happens to have at those codepoints.
	haveSymbols bool

	fonts   panelFonts
	brushes panelBrushes
	geom    panelGeom

	x, y, w, h int32

	// buf is the client area, drawn once and blitted on every WM_PAINT.
	buf *backBuffer

	// reported keeps OnMove to one call. Two WM_CLOSEs can be in flight at once -
	// closeOpenPanel posts one per tray click, and the singleton is not released
	// until this thread's pump has drained - and the caller writes a config file
	// for each report it gets.
	reported bool
	// handedOff records that a press was handed to the system's modal move loop at
	// least once during this showing, and origin is where GetWindowRect said the
	// window was at that FIRST handoff. Together they gate the position report, and
	// neither half is enough on its own - see reportMove for the whole rule.
	//
	// Only the first handoff writes origin. A later press happens wherever the user
	// has already dragged the panel to, so keeping the newest origin would make a
	// drag followed by one idle click report nothing at all.
	handedOff bool
	origin    win.POINT
	// moveRectFailed keeps the WM_MOVE diagnostic to one line per panel.
	moveRectFailed bool
	// paintFailed does the same for the WM_PAINT diagnostic, and for a stronger
	// reason: a window whose BeginPaint fails is asked to paint again immediately,
	// so an ungated line there is not a log entry but a log flood.
	paintFailed bool

	refreshStop chan struct{}
}

func newPanel(data []weather.DailyForecast, req gui.Forecast, l i18n.Lang) *panel {
	// The theme option chooses this palette, exactly as it does for the About and
	// settings windows, and "auto" is what defers to Windows.
	//
	// It briefly did not, on the argument that the desktop paints this window's
	// caption so the client area had to agree with the desktop rather than with a
	// preference. The premise was wrong: the caption is not the desktop's to
	// choose either. This program asks for the one it wants through
	// DwmSetWindowAttribute, here and in both other windows, so honouring the
	// option costs nothing and leaving one window out of it produced exactly what
	// it sounds like - two light windows and a dark panel, on a desktop where the
	// user had asked for light.
	//
	// High contrast wins over all of it - see panelPaletteSystem for why the
	// substitution that is right for dark mode is wrong there.
	dark := resolveDark(req.Theme) && !highContrastOn()

	p := &panel{
		title: l.ForecastTitle(),
		req:   req,
		dark:  dark,
		pal:   panelPaletteSystem(dark),
		heads: l.ForecastHeaders(),
		rows:  buildRows(data, req, l),
	}
	return p
}

// makeOwner creates the panel's owner: an invisible, zero-sized window that is
// never shown and never painted.
//
// It exists because of what dropping WS_EX_TOOLWINDOW cost. That flag was doing
// two jobs at once - the thin palette caption, which was wrong and is gone, and
// keeping the window out of the taskbar and out of Alt+Tab, which the panel
// promises on every platform and the other two backends deliver. Windows leaves
// an OWNED top-level window out of both as well, and an owner is invisible, so
// the caption stays exactly the one an ordinary application window gets.
//
// ONE OWNER PER PANEL, ON THE PANEL'S OWN THREAD, and that is the whole point of
// this function rather than the sync.Once it replaces. A window belongs to the
// thread that created it, and Windows destroys every window a thread owns when
// that thread terminates. run locks its goroutine to an OS thread and never
// unlocks it, so the thread dies with the panel - taking a process-wide owner
// with it and leaving a stale HWND behind. CreateWindowEx then fails for every
// panel after the first, which is exactly what it did: the first tray click
// opened the forecast and no click afterwards opened anything.
//
// Created before pending is set, because it uses this same class: a WM_NCCREATE
// arriving while pending is non-nil would hand the owner the panel that is about
// to be built.
//
// A failure is logged and survivable - the panel is then created unowned, which
// is to say with a taskbar button.
func makeOwner(inst win.HINSTANCE) win.HWND {
	cn := utf16Of(panelClassName)
	name := utf16Of("")
	owner := win.CreateWindowEx(
		0,
		&cn[0], &name[0],
		win.WS_POPUP,
		0, 0, 0, 0,
		0, 0, inst, nil,
	)
	if owner == 0 {
		log.Printf("forecast: the owner window could not be created (%v); the panel will show a taskbar button",
			syscall.GetLastError())
	}
	return owner
}

// frame is how much larger the window is than the image it shows: the caption
// and the borders.
//
// It is asked about the SAME styles CreateWindowEx was given, both of them, and
// through the Ex form of the call. Passing anything else - or dropping the
// extended style, or using the non-Ex AdjustWindowRect - answers for a frame this
// window does not have, and the difference lands as client area below the back
// buffer, which WM_PAINT's BitBlt does not cover and WM_ERASEBKGND deliberately
// does not erase. See adjustWindowRectEx.
//
// A failure answers 0,0 and says so. That is the safe direction to be wrong in:
// too small a window clips the bottom of the last row, where too large a one
// shows uninitialised memory, and a panel missing a hairline is easier to live
// with than one with a garbage stripe across it.
func (p *panel) frame() (w, h int32) {
	var rc win.RECT
	if !adjustWindowRectEx(&rc, panelStyle, false, panelExStyle) {
		log.Print("forecast: AdjustWindowRectEx failed; the panel will be sized without its frame")
		return 0, 0
	}
	return rc.Right - rc.Left, rc.Bottom - rc.Top
}

// windowSize is the size to give the WINDOW.
//
// p.w and p.h are always the CLIENT size: they size the back buffer, and the
// back buffer is exactly what the client area shows. The caption and borders
// have to be added on top - hand p.w/p.h straight to CreateWindowEx and the
// caption eats the top of the table, because the client area then comes out
// shorter than the buffer by exactly the caption's height.
//
// EVERY placement decision has to use this and not p.w/p.h: panelCorner and
// clampToWork both reason about the window's edges against the work area, and
// SetWindowPos is given a window rect. build's shrink-to-fit has to account for
// it too - see build.
//
// Everything this file remembers as "the position" is the FRAME's top-left, which
// is what GetWindowRect answers and therefore what reportMove hands back: saved
// and restored the same way, so the panel reopens where it was left.
func (p *panel) windowSize() (int32, int32) {
	return p.w + p.frameW, p.h + p.frameH
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
	return workAreaAt(pt, win.MONITOR_DEFAULTTONEAREST)
}

// rememberedWork is the work area of the monitor that CONTAINS a remembered
// position, and false when no monitor does.
//
// MONITOR_DEFAULTTONULL, not TONEAREST, and the difference is the whole point:
// the position was saved on a desktop that may since have lost a display, and
// the nearest monitor to a point on a monitor that no longer exists is a
// perfectly reachable answer that would drag the panel to a corner of a screen
// the user never put it on. A null answer is how the caller learns to fall back
// to the pointer instead.
func rememberedWork(pt win.POINT) (win.RECT, bool) {
	return workAreaAt(pt, win.MONITOR_DEFAULTTONULL)
}

func workAreaAt(pt win.POINT, flags uint32) (win.RECT, bool) {
	rc := win.RECT{Left: pt.X, Top: pt.Y, Right: pt.X + 1, Bottom: pt.Y + 1}
	mon := monitorFromRect(&rc, flags)
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

// clampToWork nudges a remembered position until the whole panel is inside the
// work area, and answers where the panel should go.
//
// The right and bottom edges are pulled in first and the left and top second, so
// that a panel too big for the work area - a remembered position carried onto a
// far smaller screen, after the shrink-to-fit in build has already done what it
// can - shows its top-left corner rather than its bottom-right. panelCorner
// makes the same choice for the same reason.
//
// The caller must pass the size the panel WILL be, which is only known after
// build: the shrink-to-fit retries can lower the effective DPI and with it every
// dimension, and clamping against the first attempt's size would leave a panel
// pushed further from the edge than it needs to be.
func clampToWork(x, y, w, h int32, area win.RECT) (int32, int32) {
	if r := area.Right - w; x > r {
		x = r
	}
	if b := area.Bottom - h; y > b {
		y = b
	}
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

// releasePanel frees the singleton, and is deferred by run so that it happens
// however the panel's message pump ends.
func releasePanel() {
	panelMu.Lock()
	panelBusy = false
	panelHWND = 0
	panelMu.Unlock()
	// Останавливаем автообновление.
	// panel здесь не передаётся, поэтому идём через map через hwnd.
	// Но проще: panel сам закроет refreshStop при завершении.
}

// raisePanel brings an already-open panel to the front. Used only when two
// opens race, never as a response to a user click.
func raisePanel() {
	panelMu.Lock()
	hwnd := panelHWND
	panelMu.Unlock()
	if hwnd != 0 {
		forceForeground(hwnd)
	}
}

// closeOpenPanel makes the tray icon a toggle: if the panel is up, the click
// that would have opened it closes it instead. It reports whether it consumed
// the click.
//
// The question it asks is simply whether a panel is on screen right now, and
// that is trustworthy because the only other way out is the caption's close
// button: a click on the tray icon can no longer dismiss the panel as a side
// effect of taking the foreground, so a click can no longer arrive to find the
// panel already gone.
func closeOpenPanel() bool {
	panelMu.Lock()
	hwnd := panelHWND
	busy := panelBusy
	panelMu.Unlock()

	if busy {
		if hwnd != 0 {
			win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
		}
		return true
	}
	return false
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
		// Not the toggle: this is two opens racing, so the one that lost raises
		// the winner rather than closing it.
		raisePanel()
		return
	}
	defer releasePanel()
	defer func() {
		if p.refreshStop != nil {
			close(p.refreshStop)
			p.refreshStop = nil
		}
	}()

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
			// The application icon, resource id 1, the same one the About and
			// settings windows ask for. Without it the caption and Alt+Tab draw
			// the system's default application icon - which is what they did.
			HIcon:   win.LoadIcon(p.inst, win.MAKEINTRESOURCE(1)),
			HCursor: win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			// No class background brush and no CS_HREDRAW/CS_VREDRAW: nothing
			// about this window is painted by the class. The WM_PAINT BitBlt
			// covers every pixel of the client area, so a class brush would only
			// paint the same pixels twice and flicker while doing it.
			HbrBackground: 0,
			LpszClassName: &cn[0],
		}
		panelClassOK = win.RegisterClassEx(wc) != 0
	})
	if !panelClassOK {
		log.Printf("forecast: RegisterClassEx failed")
		return
	}

	owner := makeOwner(p.inst)
	if owner != 0 {
		// Deferred rather than destroyed at each exit, because there are four of
		// them. It runs before the deferred releasePanel above it, and by then the
		// panel window is already gone on every path - which matters, since
		// destroying an owner destroys what it owns.
		defer win.DestroyWindow(owner)
	}

	// Create INVISIBLE, at 1x1. Its size and position are not known until build
	// and show have run, so WS_VISIBLE here would flash a 1x1 title bar in the
	// top-left corner of the screen first.
	panelMu.Lock()
	pending = p
	panelMu.Unlock()

	// The title is the caption's text, and also what the window reports to
	// anything that asks its name.
	title := utf16Of(p.title)
	cn := utf16Of(panelClassName)
	p.hwnd = win.CreateWindowEx(
		panelExStyle,
		&cn[0], &title[0],
		panelStyle,
		0, 0, 1, 1,
		owner, 0, p.inst, nil,
	)
	if p.hwnd == 0 {
		panelMu.Lock()
		pending = nil
		panelMu.Unlock()
		// With the reason, because the last time this line fired it said only
		// that something had gone wrong, and the cause - an owner window
		// belonging to a thread that had already exited - had to be reasoned out
		// from the symptom instead of read here.
		log.Printf("forecast: CreateWindowEx failed: %v", syscall.GetLastError())
		return
	}

	if p.dark {
		// The caption is the one part of this window the app does not paint, and
		// DwmSetWindowAttribute is the only way to ask for a dark one. Without it
		// the client area drawn from the dark palette sits under a white title bar,
		// which is exactly the mismatch panelPaletteSystem's dark branch exists to
		// avoid.
		setDarkTitleBar(p.hwnd, true)
	}

	// The symbol column is drawn with the OS-registered face, so the typeface
	// has to be handed to GDI before any font is created from it. Load is
	// idempotent and process-wide; the panel is a singleton, so this is the
	// only caller that can be running.
	p.haveSymbols = fonts.Load()
	if !p.haveSymbols {
		log.Printf("forecast: could not register the weather typeface; the symbol column will be blank")
	}

	// Which work area the layout is fitted to has to be settled BEFORE build,
	// because build shrinks the layout to fit it. A remembered position can name
	// a different monitor from the one the pointer is on, and fitting to the
	// pointer's screen and then placing the window on a smaller one is how a
	// panel ends up hanging off an edge.
	work, haveWork := win.RECT{}, false
	remembered := win.POINT{}
	haveRemembered := false
	if p.req.At != nil {
		remembered = win.POINT{X: int32(p.req.At.X), Y: int32(p.req.At.Y)}
		work, haveRemembered = rememberedWork(remembered)
		haveWork = haveRemembered
		if !haveRemembered {
			log.Print("forecast: the remembered position is on no current monitor; anchoring at the pointer instead")
		}
	}
	if !haveWork && haveAt {
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

	// The WINDOW's size, not the image's: p.x and p.y are the window's top-left
	// corner - what SetWindowPos is given and what GetWindowRect answers - so the
	// size weighed against the work area has to be the window's too.
	winW, winH := p.windowSize()

	p.x, p.y = 0, 0
	switch {
	case haveRemembered:
		// No edge margin here, unlike the corner anchor: the user put the panel
		// exactly there, and moving it by panelEdgeGap would be the program
		// second-guessing a position it was asked to reproduce.
		p.x, p.y = clampToWork(remembered.X, remembered.Y, winW, winH, work)
	case haveWork:
		p.x, p.y = panelCorner(at.X, at.Y, winW, winH, scaleDPI(panelEdgeGap, p.dpi), work)
	}

	if !p.show() {
		win.DestroyWindow(p.hwnd)
		p.release()
		return
	}

	panelMu.Lock()
	panelHWND = p.hwnd
	panelMu.Unlock()

	win.ShowWindow(p.hwnd, win.SW_SHOW)
	if !forceForeground(p.hwnd) {
		// Worth saying out loud: the panel is on screen and topmost, but it is
		// not the active window, so it draws an inactive caption and the first
		// click on it will be spent on activating it.
		log.Printf("forecast: could not take the foreground; the panel is up but not activated")
	}

	p.refreshStop = make(chan struct{})
	go p.refreshLoop()

	var msg win.MSG
	for {
		switch win.GetMessage(&msg, 0, 0, 0) {
		case 0: // WM_QUIT
			return
		case -1:
			// Wound down exactly like the two failure exits above, and for the same
			// reason: the window is still up, so nothing has delivered the
			// WM_NCDESTROY that frees the fonts, the brushes and the back buffer.
			// Returning bare would strand all of them for the life of the process
			// and leave the *panel pinned in live, while the deferred releasePanel
			// lets the next tray click build a second panel on top of the first.
			log.Printf("forecast: GetMessage failed; closing the panel")
			win.DestroyWindow(p.hwnd)
			p.release()
			return
		}
		win.TranslateMessage(&msg)
		win.DispatchMessage(&msg)
	}
}

// show draws the panel and puts the window where it belongs. It is called
// while the window is still hidden; run's ShowWindow is what reveals it.
func (p *panel) show() bool {
	p.paint()

	// One call moves and resizes. It is given the WINDOW rect, and the window is
	// sized so its CLIENT area is exactly the back buffer - see windowSize.
	//
	// SWP_NOZORDER because WS_EX_TOPMOST already put the window at the top of the
	// topmost band and this call has no business reordering it; SWP_NOACTIVATE
	// because the window is still hidden and activation is forceForeground's job,
	// after ShowWindow.
	winW, winH := p.windowSize()
	if !win.SetWindowPos(p.hwnd, 0, p.x, p.y, winW, winH,
		win.SWP_NOZORDER|win.SWP_NOACTIVATE) {
		// Silence here would leave a 1x1 window in the corner of the screen, which
		// reads to the user as "the tray icon does nothing".
		log.Print("forecast: SetWindowPos failed; the panel cannot be placed")
		return false
	}
	// No InvalidateRect: the window is not visible yet, and the ShowWindow in run
	// generates the first WM_PAINT.
	return true
}

// release frees every GDI object the panel owns: the back buffer's DC and
// bitmap, the three fonts and the three brushes.
//
// The DC goes FIRST and the fonts and brushes second, which is the order that
// cannot leak. DeleteObject refuses to free a GDI object that is still selected
// into a DC, and a font or brush that fails to delete leaks for the life of the
// process; deleting the DC discards whatever was selected into it, so by the time
// the other two run there is provably no DC left to hold anything. paint does
// select its original font back before it returns, and the brushes are never
// selected at all - FillRect takes a brush as an argument rather than from the DC
// - but this way the invariant does not rest on either fact.
//
// All three steps are idempotent and nil-safe, so release can be called twice -
// which it is, on the failure paths in run().
func (p *panel) release() {
	p.buf.dispose()
	p.buf = nil
	p.freeFonts()
	p.freeBrushes()
}

// reportMove hands the panel's final position to the caller.
//
// NOTHING IS REPORTED UNLESS THE USER ACTUALLY MOVED THE PANEL, which takes BOTH
// of these: a press was handed to the system's move loop at least once during
// this showing, and the window is no longer where GetWindowRect said it was at
// that first handoff.
//
// The handoff alone is not enough, and that was the bug. A bare click on the
// panel body is a handoff too - the press goes to the move loop whether or not the
// pointer then moves - so a single stray click on a user's first ever forecast
// reported the corner this code chose for it as though the user had picked it.
// The caller then stops anchoring at the pointer, permanently, since it reads a
// stored position as "put it back there".
//
// The comparison is also what makes an ESCAPE-CANCELLED DRAG report nothing: the
// system's move loop restores the window to where the drag began, so the two
// reads come out equal even though a real drag happened.
//
// Both positions are read through GetWindowRect, never against the placement
// computed in run(), and the GTK backend reads both of its positions through
// gtk_window_get_position for the same reason: reading both the same way is what
// makes the rule correct on Wayland, where that call always answers 0,0 and the
// two reads are therefore equal, so nothing is persisted.
//
// It reads the position on the UI thread, where the window still exists -
// GetWindowRect on a destroyed HWND answers nothing - and then delivers it from a
// throwaway goroutine. The hand-off is deliberate: OnMove reaches into the
// tray's configuration and writes a file, and doing that inline would hold up
// WM_CLOSE, leaving the panel visibly on screen for as long as the disk takes.
// A callback slow enough to matter can then never stall this window's queue, and
// nothing it does can deadlock against a lock held by whoever is closing us. The
// GTK backend promises the same thing, because gui.Forecast.OnMove states one
// threading contract for every backend rather than one per backend.
//
// What the callback may assume, therefore: it is called at most once per panel
// and only after the panel actually changed position under the user's hand, with
// the window's screen position in the same coordinate space it handed over in At,
// from an ARBITRARY goroutine, possibly after the panel has gone. It may not
// touch this panel or any window; it may take locks and do I/O.
func (p *panel) reportMove() {
	if p.req.OnMove == nil || p.reported || !p.handedOff {
		return
	}
	var rc win.RECT
	if !win.GetWindowRect(p.hwnd, &rc) {
		log.Print("forecast: GetWindowRect failed; the position was not remembered")
		return
	}
	if rc.Left == p.origin.X && rc.Top == p.origin.Y {
		// A handoff that moved nothing: a bare click on the body, or a drag the
		// user cancelled with Escape and the system put back.
		return
	}
	p.reported = true
	onMove, x, y := p.req.OnMove, int(rc.Left), int(rc.Top)
	go onMove(x, y)
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

// build measures the content, sizes the window, and creates everything the panel
// draws with: its fonts, its brushes and its back buffer.
//
// The loop exists because the layout is fixed-width: 620 units at 200% is 1240
// pixels, which does not fit a 1280-wide screen once the edge margins are paid
// for. Rather than let the panel run off the edge it is re-laid out at a lower
// effective DPI, which is the same trick layoutDPI performs for the settings
// window. Two corrections are enough for any real screen; the floor stops it
// shrinking the text into illegibility.
//
// WHAT "FITS" MEANS. The loop measures the CONTENT - p.w and p.h are the client
// area - but what has to fit the work area is the whole window, so the caption
// and borders are subtracted from the available space before the comparison.
// Skip that and a panel sized to exactly fill the work area has its caption
// pushed off the top of the screen, where the close button and the title-bar drag
// both live.
func (p *panel) build(work win.RECT, haveWork bool) bool {
	dpi := dpiOf(p.hwnd)

	// Measured once and outside the loop: the frame comes from the system's own
	// metrics, not from the layout, so re-laying the layout out smaller does not
	// make it smaller. That is also why it is subtracted below rather than divided
	// out, which is exactly what fitDPI does for the settings window.
	p.frameW, p.frameH = p.frame()

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
		// The frame is charged to the available space, so everything below stays
		// arithmetic on CLIENT pixels: availW/availH are how much room the content
		// may occupy once the window's own chrome has been paid for, which keeps
		// the dpi*availW/p.w reduction comparing like with like.
		availW := work.Right - work.Left - gap - p.frameW
		availH := work.Bottom - work.Top - gap - p.frameH
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

	// Both of these are outside the loop: neither depends on the layout DPI, and
	// the buffer cannot be sized until the loop has settled p.w and p.h.
	//
	// The window's own DC is borrowed as the reference the buffer's bitmap is made
	// compatible with, because it has to be a DC of a real device - newBackBuffer
	// says what happens otherwise. It is released immediately; the buffer keeps a
	// memory DC of its own, and holding a window DC for the life of the panel
	// would tie up one of the system's cached DCs for no reason.
	ref := win.GetDC(p.hwnd)
	if ref == 0 {
		log.Print("forecast: GetDC failed; the panel has nothing to draw into")
		return false
	}
	p.buf = newBackBuffer(ref, p.w, p.h)
	win.ReleaseDC(p.hwnd, ref)
	if p.buf == nil {
		log.Printf("forecast: the %dx%d back buffer could not be created", p.w, p.h)
		return false
	}

	return p.makeBrushes()
}

// makeBrushes creates the panel's three fills, once, from the flattened palette.
//
// Only the sheet's is worth failing over: it is what covers every pixel of the
// client area, which is the promise WM_ERASEBKGND is allowed to decline to erase
// on. A missing rule or hairline brush costs a line in the table and a line in
// the log, and paint skips what it has no brush for rather than drawing it in
// whatever the DC's stock brush happens to be - which is white, and would be a
// far louder mistake than an absent hairline.
func (p *panel) makeBrushes() bool {
	p.brushes = panelBrushes{
		sheet: createSolidBrush(p.pal.sheet),
		rule:  createSolidBrush(p.pal.rule),
		sep:   createSolidBrush(p.pal.sep),
	}
	if p.brushes.sheet == 0 {
		log.Print("forecast: CreateSolidBrush failed for the panel background")
		return false
	}
	if p.brushes.rule == 0 || p.brushes.sep == 0 {
		log.Print("forecast: CreateSolidBrush failed for a table rule; that line will be missing")
	}
	return true
}

func (p *panel) freeBrushes() {
	for _, b := range []win.HBRUSH{p.brushes.sheet, p.brushes.rule, p.brushes.sep} {
		if b != 0 {
			win.DeleteObject(win.HGDIOBJ(b))
		}
	}
	p.brushes = panelBrushes{}
}

// makeFonts creates the fonts for this DPI. It returns false only when a font
// the table cannot do without is missing; the symbol face is optional, and its
// absence leaves that column blank.
func (p *panel) makeFonts() bool {
	// Weight 600 on the captions and nothing else: `.thead { font-weight: 600 }`
	// is the whole of what separates the header row from the data, one size for
	// the entire table.
	p.fonts = panelFonts{
		thead: panelFont(theadPt, win.FW_SEMIBOLD, p.dpi),
		cell:  panelFont(cellPt, win.FW_NORMAL, p.dpi),
	}
	if p.fonts.thead == 0 || p.fonts.cell == 0 {
		return false
	}
	if !p.haveSymbols {
		return true
	}

	// The symbol is sized in PIXELS, not points: symbolPt is the ink size
	// fonts.Glyph is given on the GTK side, and these glyphs draw at about one
	// em, so an em of that many pixels is the closest the two get.
	f := panelFontPx(scaleDPI(symbolPt, p.dpi), win.FW_NORMAL, weatherIconsFace)
	if f == 0 {
		return true
	}
	// CreateFontIndirect never fails on an unknown face name: with
	// DEFAULT_CHARSET it substitutes "any font with the specified attributes",
	// and the symbol column would then fill with whatever that face draws at
	// U+F0xx - boxes, or worse, plausible-looking letters. GetTextFace reports
	// the face GDI actually realised, which is the only way to tell.
	if name, ok := faceOf(f); ok && !strings.EqualFold(name, weatherIconsFace) {
		log.Printf("forecast: %q resolved to %q; the symbol column will be blank", weatherIconsFace, name)
		win.DeleteObject(win.HGDIOBJ(f))
		return true
	}
	p.fonts.symbol = f
	return true
}

func (p *panel) freeFonts() {
	for _, f := range []win.HFONT{p.fonts.thead, p.fonts.cell, p.fonts.symbol} {
		if f != 0 {
			win.DeleteObject(win.HGDIOBJ(f))
		}
	}
	p.fonts = panelFonts{}
}

// measure asks GDI for the font metrics and the width of every string, then
// hands them to layoutTable.
//
// Measuring all 40-odd strings is what makes the columns come out the width a
// GtkGrid gives them: each column requests its widest cell, and only the slack
// left over is shared. It costs one GetTextExtentPoint32 per string against a
// 1x1 memory DC, which is nothing beside the fetch that preceded it.
func (p *panel) measure() {
	m := newMeasureDC()
	defer m.dispose()

	met := panelMetrics{
		theadH: m.lineHeight(p.fonts.thead),
		cellH:  m.lineHeight(p.fonts.cell),
	}
	if p.fonts.symbol != 0 {
		met.symbolPx = scaleDPI(symbolPt, p.dpi)
	}

	for i := 0; i < numCols; i++ {
		w := int32(0)
		if i < len(p.heads) {
			w = m.width(p.heads[i], p.fonts.thead)
		}
		if i == colCond {
			// The GTK column requests the symbol tile, which is square and
			// symbolPx on a side. Here the glyph's own advance width is measured
			// too, so a glyph wider than its em cannot be clipped by a column
			// sized only for the caption.
			w = max(w, met.symbolPx)
			for r := range p.rows {
				w = max(w, m.width(p.rows[r].cell[i], p.fonts.symbol))
			}
		} else {
			for r := range p.rows {
				w = max(w, m.width(p.rows[r].cell[i], p.fonts.cell))
			}
		}
		met.natural[i] = w
	}

	p.geom, p.w, p.h = layoutTable(met, p.dpi, len(p.rows))
}

// layoutTable computes every rectangle in the panel and the panel's own size.
//
// The vertical rhythm is the GTK grid's: a caption row, rowGapY, the rule,
// rowGapY, then each data row separated from the next by rowGapY, a hairline and
// rowGapY again. Row heights come from the font metrics and the symbol box
// rather than from constants, which is what stops a locale with taller glyphs
// from clipping.
//
// The horizontal rhythm is a GtkGrid of expanding cells: every column asks for
// its widest cell, and the slack left over after the gaps are paid for is shared
// out EQUALLY, which is what GTK does with expanding grid columns. Equal fifths
// would be simpler and wrong - it would give the symbol column as much room as
// "Температура" needs and starve the date.
//
// It is a pure function of its arguments on purpose: it is the one piece of this
// file whose arithmetic can be checked away from Windows.
func layoutTable(m panelMetrics, dpi int32, n int) (panelGeom, int32, int32) {
	s := func(v int) int32 { return scaleDPI(v, dpi) }

	pad := s(pagePad)
	w := s(panelWidthPt)
	contentL := pad
	contentR := w - pad
	cw := contentR - contentL

	gap := s(rowGapY)
	ruleTh := max(1, s(rulePt))
	rowH := max(m.cellH, m.symbolPx)

	rows := int32(n)
	tableH := m.theadH + gap + ruleTh + gap + rows*rowH
	if rows > 1 {
		tableH += (rows - 1) * (2*gap + ruleTh)
	}

	h := s(pagePadTop) + pad + tableH

	var g panelGeom
	// The sheet is not a card inside the window: it fills the CLIENT area, with
	// the caption above it and the padding inside it - see windowSize.
	g.sheet = rectAt(0, 0, w, h)

	colL, colR := tableColumns(m.natural, contentL, contentR, s(colGapX))

	headY := pad
	for i := 0; i < numCols; i++ {
		g.head[i] = win.RECT{Left: colL[i], Top: headY, Right: colR[i], Bottom: headY + m.theadH}
	}
	g.rule = rectAt(contentL, headY+m.theadH+gap, cw, ruleTh)

	rowsY := g.rule.Bottom + gap
	g.rows = make([]rowGeom, n)
	for r := 0; r < n; r++ {
		y := rowsY + int32(r)*(rowH+2*gap+ruleTh)
		for i := 0; i < numCols; i++ {
			g.rows[r].cell[i] = win.RECT{Left: colL[i], Top: y, Right: colR[i], Bottom: y + rowH}
		}
		if r < n-1 {
			g.rows[r].sep = rectAt(contentL, y+rowH+gap, cw, ruleTh)
		}
	}
	return g, w, h
}

// tableColumns turns the columns' natural widths into their left and right
// edges, spanning exactly contentL to contentR.
//
// Slack is shared equally, and shared from a single running division so the
// rounding error is spread instead of landing on the last column. If the natural
// widths do not fit at all - a very narrow layout, or a locale with much longer
// captions - every column is scaled down in proportion and DT_END_ELLIPSIS
// truncates what is left, which is the one outcome that keeps the columns in
// order and inside the sheet.
func tableColumns(natural [numCols]int32, contentL, contentR, gapX int32) (colL, colR [numCols]int32) {
	avail := contentR - contentL - (numCols-1)*gapX
	if avail < 0 {
		avail = 0
	}

	sum := int32(0)
	for _, v := range natural {
		sum += v
	}
	if sum > avail && sum > 0 {
		for i := range natural {
			natural[i] = natural[i] * avail / sum
		}
		sum = 0
		for _, v := range natural {
			sum += v
		}
	}

	extra := avail - sum
	if extra < 0 {
		extra = 0
	}

	x := contentL
	for i := 0; i < numCols; i++ {
		share := extra*int32(i+1)/numCols - extra*int32(i)/numCols
		colL[i] = x
		colR[i] = x + natural[i] + share
		x = colR[i] + gapX
	}
	return colL, colR
}

func rectAt(x, y, w, h int32) win.RECT {
	return win.RECT{Left: x, Top: y, Right: x + w, Bottom: y + h}
}

// ---------------------------------------------------------------------------
// Painting
// ---------------------------------------------------------------------------

// paint draws the whole panel, once, into the back buffer. WM_PAINT does not
// call it: WM_PAINT blits what it leaves behind.
//
// THE INVARIANT THE REST OF THE PAINT PATH RESTS ON is the first FillRect below.
// g.sheet is 0,0,p.w,p.h - layoutTable builds it from the same two numbers it
// returns as the panel's size - and windowSize made the client area exactly that,
// so this one call writes every pixel of the buffer, and the single BitBlt in
// WM_PAINT then writes every pixel of the client area. That is what entitles
// WM_ERASEBKGND to refuse to erase and the window class to carry no background
// brush; break it and the window shows whatever was on the screen before it.
//
// The order is sheet, then rules, then text, and it is not interchangeable.
// FillRect is opaque and paints over whatever is beneath it, so any fill issued
// after a string would wipe that string out. Text goes last for the same reason
// it is drawn with SetBkMode(TRANSPARENT): a glyph then lays ink on what is
// already there instead of arriving in a box of the DC's background colour.
func (p *panel) paint() {
	if p.buf == nil || p.brushes.sheet == 0 {
		return
	}
	dc := p.buf.dc
	g := &p.geom

	fillRect(dc, &g.sheet, p.brushes.sheet)

	if p.brushes.rule != 0 {
		fillRect(dc, &g.rule, p.brushes.rule)
	}
	if p.brushes.sep != 0 {
		for i := range g.rows {
			// The last row's hairline is the zero RECT, and FillRect draws nothing
			// for an empty rectangle: the hairlines go BETWEEN the data rows.
			fillRect(dc, &g.rows[i].sep, p.brushes.sep)
		}
	}

	// A fresh memory DC starts OPAQUE with a white background colour, which would
	// put every cell in a white rectangle on a dark panel and stamp a white band
	// over the hairlines on any panel at all.
	win.SetBkMode(dc, win.TRANSPARENT)

	const cellFlags = win.DT_VCENTER | win.DT_SINGLELINE | win.DT_NOPREFIX
	const textFlags = cellFlags | win.DT_END_ELLIPSIS

	// Selected once here and put back at the end. The buffer's DC outlives this
	// call and release() deletes the fonts, so nothing of the panel's may still be
	// selected into it by then - see release.
	//
	// The order of what follows is chosen to keep SelectObject calls down to three
	// rather than one per string: all the captions, then all the text cells, then
	// all the symbols. Each group is one font and one colour.
	prev := win.SelectObject(dc, win.HGDIOBJ(p.fonts.thead))

	win.SetTextColor(dc, p.pal.thead)
	for i := 0; i < numCols && i < len(p.heads); i++ {
		drawText(dc, p.heads[i], g.head[i], colAlign[i]|textFlags)
	}

	// The cells and the symbols share a colour: the palette's foreground, which is
	// the theme's own ink - COLOR_WINDOWTEXT - for the plainest of reasons, that
	// the symbols are text, and the same value forecast_linux.go tints its
	// rasterised glyphs with. So the colour is set once for both groups.
	win.SetTextColor(dc, p.pal.text)
	win.SelectObject(dc, win.HGDIOBJ(p.fonts.cell))
	for r := range p.rows {
		for i := 0; i < numCols; i++ {
			if i == colCond {
				continue
			}
			drawText(dc, p.rows[r].cell[i], g.rows[r].cell[i], colAlign[i]|textFlags)
		}
	}

	// A zero symbol font is the documented outcome of a weather typeface GDI could
	// not register or resolved to something else - see makeFonts - and the column
	// is then simply left empty rather than filled with a substitute face's idea of
	// U+F0xx.
	if p.fonts.symbol != 0 {
		win.SelectObject(dc, win.HGDIOBJ(p.fonts.symbol))
		for r := range p.rows {
			// DT_NOCLIP, and it is load-bearing. The row is the height of the
			// symbol's BOX, matching the GtkImage on the other side, but GDI
			// positions text by its LINE box, which in this typeface is 1.45 em -
			// taller than the row. Clipping to the row would shave the top and
			// bottom off every symbol. What overhangs is mostly the font's empty
			// ascent and descent: the ink is about one em, centred to within a sixth
			// of an em, so it reaches at most about a quarter of the row beyond the
			// edge and stays clear of the hairline rowGapY away. Nothing can escape
			// the buffer either - GDI clips to the DC.
			//
			// No ellipsis on a single glyph either: DT_END_ELLIPSIS would replace
			// the symbol with "..." rather than shrink it.
			drawText(dc, p.rows[r].cell[colCond], g.rows[r].cell[colCond],
				colAlign[colCond]|cellFlags|win.DT_NOCLIP)
		}
	}

	win.SelectObject(dc, prev)
}

// refresh обновляет содержимое панели новыми данными из кеша.
func (p *panel) refresh() {
	_, daily := weather.Cached(p.req.Lat, p.req.Lon)
	if daily == nil {
		log.Print("forecast: refresh called with no cached data")
		return
	}

	l := i18n.ParseLang(p.req.Lang)
	p.rows = buildRows(daily, p.req, l)
	p.measure()

	if p.buf != nil {
		p.buf.dispose()
	}
	ref := win.GetDC(p.hwnd)
	if ref != 0 {
		p.buf = newBackBuffer(ref, p.w, p.h)
		win.ReleaseDC(p.hwnd, ref)
	}

	if p.buf != nil {
		p.paint()
		win.InvalidateRect(p.hwnd, nil, true)
	}
}

func (p *panel) refreshLoop() {
	for {
		select {
		case <-weather.UpdateCh:
			if p.hwnd != 0 {
				win.PostMessage(p.hwnd, wmRefresh, 0, 0)
			}
		case <-p.refreshStop:
			return
		}
	}
}

// buildRows создаёт rows из данных прогноза. Выделена из newPanel для переиспользования в refresh.
func buildRows(data []weather.DailyForecast, req gui.Forecast, l i18n.Lang) []tableRow {
	out := make([]tableRow, len(data))
	for i, d := range data {
		var row tableRow
		row.cell[0] = d.Date
		row.cell[colCond] = fonts.IconForCode(d.WeatherCode)
		row.cell[2] = weather.TempRange(d, req.Units, l)
		row.cell[3] = weather.WindSpeed(d, req.WindUnit, l)
		row.cell[4] = weather.Precip(d, l)
		out[i] = row
	}
	return out
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
	case win.WM_LBUTTONDOWN:
		// Dragging. A press anywhere on the body is handed to the system's own
		// modal move loop, which is what a title bar does with a press on itself:
		// ReleaseCapture to give up the implicit capture this very message granted
		// - the loop cannot start while another window holds the mouse - and then
		// WM_NCLBUTTONDOWN with HTCAPTION, which is precisely the message
		// DefWindowProc turns into a move. What the user expects of a drag comes
		// free that way: the Escape that cancels it and puts the window back where
		// it started, the Alt-Tab that interrupts it. Reimplementing it with
		// SetCapture and WM_MOUSEMOVE gives up both and gains nothing, because the
		// window's position is re-read from GetWindowRect on WM_MOVE either way.
		//
		// What does NOT come free, and was claimed here in error: Aero Snap.
		// Windows offers edge snapping only for windows it considers resizable,
		// and this one carries neither WS_THICKFRAME nor WS_MAXIMIZEBOX, so
		// dragging it against an edge snaps to nothing.
		//
		// NOT a WM_NCHITTEST that answers HTCAPTION for the whole client area,
		// which is the shortcut every Win32 sample reaches for. That declares the
		// table itself non-client: no mouse message is delivered to it ever again,
		// so anything the panel might later want there - a hit test, a hover, a
		// click target - is dead before it is written, and a right-click on the
		// forecast opens the window menu.
		//
		// SendMessage, not PostMessage: the move loop must run inside this
		// message's own handling, while the button is still physically down.
		//
		// The caption is draggable as well, by DefWindowProc, which starts the very
		// same loop without this handler ever seeing the press. WM_ENTERSIZEMOVE is
		// what copes with that.
		//
		// The first handoff of this showing records where the window is right now,
		// through GetWindowRect - the same call reportMove compares against once the
		// panel closes. The handoff on its own says nothing about whether the user
		// placed the panel anywhere: this very message is delivered for a bare click
		// on the body as well, and the press is handed to the move loop either way.
		// It takes the comparison to tell the two apart, and to discount a drag the
		// user cancels with Escape. See reportMove.
		//
		// A GetWindowRect that fails leaves handedOff clear, so a later press can
		// still establish an origin; the drag itself proceeds regardless, because
		// being unable to remember a position is no reason to refuse to move.
		if !p.handedOff {
			var rc win.RECT
			if win.GetWindowRect(hwnd, &rc) {
				p.handedOff = true
				p.origin = win.POINT{X: rc.Left, Y: rc.Top}
			} else {
				log.Print("forecast: GetWindowRect failed at the drag hand-off; this drag will not be remembered")
			}
		}
		win.ReleaseCapture()
		win.SendMessage(hwnd, win.WM_NCLBUTTONDOWN, win.HTCAPTION, 0)
		return 0

	case win.WM_ENTERSIZEMOVE:
		// The system's modal move loop has started, and this is where the origin
		// for the position report comes from when the loop was NOT started by
		// WM_LBUTTONDOWN. That handler records it before its SendMessage, so a body
		// drag reaches this as a no-op; a captioned window has two more ways into
		// the loop that never touch this window procedure's press handling at all -
		// dragging the title bar, which DefWindowProc turns into a move by itself,
		// and the Alt+Space window menu's Move command, which WS_SYSMENU brings
		// along with the close button. Without this the panel would be moved by the
		// one gesture a captioned window makes most obvious and then forget where it
		// was put, because reportMove refuses to report without a handoff.
		//
		// Reading it here is as good as reading it there: WM_ENTERSIZEMOVE is sent
		// as the loop STARTS, before the window has moved a pixel.
		if !p.handedOff {
			var rc win.RECT
			if win.GetWindowRect(hwnd, &rc) {
				p.handedOff = true
				p.origin = win.POINT{X: rc.Left, Y: rc.Top}
			} else {
				log.Print("forecast: GetWindowRect failed at the start of a move; this drag will not be remembered")
			}
		}
		return 0

	case win.WM_MOVE:
		// The move loop repositions the window behind this code's back, so p.x/p.y
		// - the WINDOW's top-left, which is where show() placed it - are refreshed
		// here rather than left describing where the panel opened.
		//
		// Reading the window rect rather than unpacking lParam: lParam carries the
		// CLIENT origin, which under a caption sits below and inside the window's
		// own top-left, so unpacking it would drift the recorded position up and
		// left by the frame on every move.
		var rc win.RECT
		if win.GetWindowRect(hwnd, &rc) {
			p.x, p.y = rc.Left, rc.Top
		} else if !p.moveRectFailed {
			// Once per panel, not once per message: WM_MOVE arrives dozens of times
			// a second while the window is being dragged, and whatever makes this
			// call fail will make all of them fail. It is worth one line even so:
			// reportMove reads the position through the same call, so a
			// GetWindowRect that has started failing is about to lose the drag the
			// user just made.
			p.moveRectFailed = true
			log.Print("forecast: GetWindowRect failed on WM_MOVE; the panel's recorded position is stale")
		}
		return 0

	case win.WM_PAINT:
		var ps win.PAINTSTRUCT
		dc := win.BeginPaint(hwnd, &ps)
		if dc != 0 {
			// ONE BitBlt and nothing else, which is the whole of why this window
			// cannot flicker: the panel was drawn into the buffer when it was
			// built, so a repaint copies a finished picture instead of assembling
			// one in front of the user.
			//
			// The whole client area, not ps.RcPaint: the client area IS the buffer,
			// pixel for pixel - windowSize added the frame on the outside - so
			// there is nothing to be gained by clipping to the damaged part of a
			// picture that is already in memory, and a partial blit is one more
			// thing to get wrong.
			//
			// A failure needs no branch. blitTo answers false only for a disposed
			// buffer or a null DC, and either way the client area keeps whatever
			// the system last showed rather than becoming garbage: WM_ERASEBKGND
			// painted nothing over it.
			p.buf.blitTo(dc)
			win.EndPaint(hwnd, &ps)
			return 0
		}
		if !p.paintFailed {
			p.paintFailed = true
			log.Print("forecast: BeginPaint failed; leaving the repaint to DefWindowProc")
		}
		// No EndPaint: there is no PAINTSTRUCT to pass it. Falling through to
		// DefWindowProc is what keeps this from spinning the thread at 100% - it
		// does its own BeginPaint/EndPaint pair and validates the region.

	case win.WM_ERASEBKGND:
		// "I erased it", because the WM_PAINT above covers every pixel of the
		// client area from the back buffer and erasing first would only paint those
		// pixels twice and flicker while doing it.
		//
		// THIS IS ONLY SAFE BECAUSE OF TWO THINGS, and both are enforced elsewhere.
		// The buffer is written edge to edge - p.paint's first FillRect, which is
		// where that invariant is stated and argued. And the client area is never
		// larger than the buffer, being sized from windowSize: the panel is far
		// wider than the SM_CXMIN a captioned window's minimum tracking size could
		// clamp it to, 620 layout units being 465 pixels even at the minLayoutDPI
		// floor.
		return 1

	case win.WM_CLOSE:
		// BOTH dismissals funnel through here - the caption's close button and the
		// tray's own PostMessage in closeOpenPanel - which makes it the one place
		// the final position can be read while the window still exists. WM_DESTROY
		// would be too late for GetWindowRect to mean anything.
		//
		// THE CAPTION'S CLOSE BUTTON LANDS HERE TOO, which is the whole reason the
		// position survives the commonest way out of all. The chain is all
		// DefWindowProc's and this file breaks no link in it: a click on the
		// caption's close box hit-tests as HTCLOSE, DefWindowProc turns the release
		// into WM_SYSCOMMAND with SC_CLOSE, and DefWindowProc's SC_CLOSE handling
		// sends WM_CLOSE. Neither WM_NCLBUTTONUP nor WM_SYSCOMMAND is intercepted
		// above, so nothing swallows it, and the reportMove below runs for a caption
		// close exactly as it does for the tray toggle.
		p.reportMove()
		win.DestroyWindow(hwnd)
		return 0

	case wmRefresh:
		p.refresh()
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
// Windows flashes the taskbar button, and this window has none - it is owned, and
// an owned window gets no button - so the refusal is completely silent.
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
