//go:build windows

package ui

// The 7-day forecast panel, Win32 edition.
//
// The CONTENT is a plain five-column table: one header row, a rule, then seven
// data rows separated by hairlines. No summary header and no cards - that layout
// was tried and reverted. The CHROME around it comes in two looks, described
// below; what they have in common and never trade away is: above other windows,
// off the taskbar, placed at the work-area corner nearest the click - or at the
// position the caller remembers - draggable by its body, and dismissed by
// Escape, by focus loss, or by a close button.
//
// While the request is PINNED the first two dismissals are switched off, so the
// close button and another click on the tray icon are the only ways out. The
// policy is asked of gui.Forecast.Pinned at the moment of each event, never
// cached: the user can uncheck the box while this very panel is on screen.
//
// Every metric is shared with the GTK backend value for value: the constants
// below carry the same names and numbers as the ones in forecast_linux.go and
// the stylesheet in style_linux.go, and the column widths are distributed the
// way a GtkGrid of expanding cells distributes them. The point of the two files
// is that the two platforms look like one product.
//
// How it is built, in one line: the whole panel is composed in pure Go into a
// premultiplied image.RGBA, GDI is used for exactly one thing (turning strings
// into glyph coverage, in a scratch bitmap), the result is copied into a
// top-down 32bpp DIB section, and that DIB is what reaches the screen. GDI never
// touches the panel surface, so GDI can never destroy its alpha. See
// panelpaint_windows.go for why that rule exists, and do not reach for DrawText
// on the surface itself while changing this layout: it writes R, G and B and
// leaves A at zero, which is a pixel-perfect panel with invisible text.
//
// TWO LOOKS, ONE COMPOSITION. gui.Forecast.Appearance chooses between them once,
// in newPanel, and nothing re-reads it afterwards. What differs is the window
// the image lives in, where the colours come from, and how the image gets onto
// the screen. The layout arithmetic, the fonts, every metric and the whole
// compositing pipeline are the same code in both, which is the point: the panel
// is the same product either way.
//
// MODERN - the default, and what this panel has always been.
//
//	Window style:   WS_POPUP. No WS_CAPTION, no WS_BORDER, no WS_THICKFRAME.
//	Extended style: WS_EX_LAYERED | WS_EX_TOOLWINDOW | WS_EX_TOPMOST.
//	                NOT WS_EX_NOACTIVATE, which is the tempting wrong flag: it
//	                stops the window becoming foreground, so it never holds the
//	                keyboard focus, so Escape never arrives.
//	Content:        one UpdateLayeredWindow call. There is no WM_PAINT handler
//	                for it and none is wanted - the system keeps the surface and
//	                repaints it itself.
//	Colours:        the app's own palette, translucent with rounded corners
//	                where the display can composite per-pixel alpha.
//	Closing:        the × the panel draws into its own top row, because a window
//	                with no title bar has no close button of its own.
//
// SYSTEM LOOK - an ordinary application window, coloured by the desktop.
//
//	Window style:   WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU. A title bar with
//	                the window manager's own close button, square corners,
//	                opaque. WS_SYSMENU is what puts the close button in the
//	                caption; WS_CAPTION alone draws a bar with nothing in it.
//	Extended style: WS_EX_TOPMOST, and deliberately NOT WS_EX_LAYERED - see
//	                below - nor WS_EX_TOOLWINDOW, which was here and was wrong.
//	                That flag draws the thin palette-window caption with a small
//	                close button, so the one control this window offers did not
//	                look like the one every other window on the desktop offers.
//	                The caption is now the ordinary one, the same the About box
//	                gets. WS_EX_TOOLWINDOW was also what kept the panel off the
//	                taskbar and out of Alt+Tab; an invisible owner window does
//	                that now instead - see ensurePanelOwner.
//	Content:        the same composed DIB, BitBlt to the client area from a
//	                WM_PAINT handler.
//	Colours:        GetSysColor, with one documented exception that is a Windows
//	                wart rather than a choice - see panelPaletteSystem.
//	Closing:        the caption's close button. It arrives here as WM_CLOSE like
//	                every other dismissal, so it reports the position too.
//
// WHY THE LAYERED PATH IS NOT SIMPLY KEPT AND A CAPTION ADDED TO IT.
// UpdateLayeredWindow requires WS_EX_LAYERED - without it the call fails with
// ERROR_INVALID_PARAMETER - and what it hands the system is the WHOLE WINDOW:
// pptDst is the window's position on screen and psize the window's size, so
// every pixel of the window, non-client area included, would have to come out of
// the composed bitmap. The caption is drawn by DefWindowProc and the DWM into
// the window's own device context, which is never that bitmap, so there is
// nowhere for a title bar to appear. Hence a second delivery path rather than a
// flag on the first - and nothing is lost by it, because an opaque sheet has no
// use for per-pixel alpha and BitBlt out of the same DIB puts the identical
// pixels on the screen.
//
// Dragging:       WM_LBUTTONDOWN hands the press to the system's own move loop.
//                 NOT a WM_NCHITTEST that answers HTCAPTION, which looks like
//                 the tidier trick and destroys the close button - panelWndProc
//                 spells the reason out. That loop PUMPS THIS THREAD'S QUEUE
//                 while it runs, so every dismissal path has to be re-entrancy
//                 safe for its duration - see the moving flag. In the system
//                 look the caption starts the very same loop on its own, without
//                 this code seeing a press at all, which is why the loop's END
//                 is settled from run's message pump and not only from
//                 WM_LBUTTONDOWN.

import (
	"image"
	"image/color"
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
	"golang.org/x/image/vector"
)

// Layout, in the same units the GTK side uses: logical pixels at 96 DPI, scaled
// to the window's real DPI at layout time. The counterpart of each value in
// forecast_linux.go or style_linux.go is named beside it.
const (
	panelWidthPt = 620 // forecastWidth
	panelEdgeGap = 12  // panelMargin: clearance from the work-area edges
	pagePad      = 14  // .page padding, sides and bottom
	// The top is tighter than the rest so the close affordance can be a
	// comfortable target without making the panel taller. The glyph occupies
	// the space the padding gives up, so the panel does not read as cramped.
	pagePadTop    = 6
	sheetRadiusPt = 14 // #nimbus-forecast.translucent border-radius

	pageGapY = 2  // page VBox spacing: the close row to the table
	rowGapY  = 6  // grid row spacing
	colGapX  = 18 // grid column spacing

	closePadX     = 9 // .close padding
	closePadY     = 0
	closeRadiusPt = 8 // .close border-radius

	theadPt = 11 // .thead font-size, weight 600
	cellPt  = 11 // .cell font-size
	closePt = 15 // .close font-size

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

// closeGlyph is the panel's own close affordance IN MODERN ONLY. With no title
// bar there is no system close button, so the panel supplies one, exactly as the
// GTK panel does. The system look draws no glyph and reserves no room for one:
// its caption already carries a close button, and two would be one too many.
const closeGlyph = "×" // MULTIPLICATION SIGN

const panelClassName = "NimbusForecastPanel"

// The two window shapes, named rather than spelled out at the CreateWindowEx
// call, because panel.frame has to be asked about the very styles the window was
// created with - BOTH of them. Ask about different ones and the client area comes
// out the wrong size, which shows up either as a caption eating the header row or
// as a strip of uninitialised memory along the bottom, neither of which looks
// like the sizing bug it is.
//
// The system look is deliberately NOT resizable: no WS_THICKFRAME and no
// WS_MAXIMIZEBOX. The layout is fixed-width by design, and those flags would
// also hand the window Aero Snap, which would fling a 620-unit panel across half
// the screen the first time a user dragged it near an edge.
const (
	panelStyleModern   = uint32(win.WS_POPUP)
	panelExStyleModern = uint32(win.WS_EX_LAYERED | win.WS_EX_TOOLWINDOW | win.WS_EX_TOPMOST)

	panelStyleSystem   = uint32(win.WS_OVERLAPPED | win.WS_CAPTION | win.WS_SYSMENU)
	panelExStyleSystem = uint32(win.WS_EX_TOPMOST)
)

// ---------------------------------------------------------------------------
// Palette
// ---------------------------------------------------------------------------

// panelPalette is one of the four palettes style_linux.go keeps: dark or light,
// translucent or solid. Which pair is used is decided at runtime, not assumed -
// see panel.show.
//
// There is no separate card colour because there is no card. The window IS the
// sheet: one background, one radius, and pagePad of padding inside it, which is
// exactly what `#nimbus-forecast.translucent` plus `.page` states.
type panelPalette struct {
	sheet color.RGBA // #nimbus-forecast background-color, premultiplied
	// radiusPt is the sheet's corner radius in layout points. Zero in the solid
	// palettes: the GTK sheet asks for a radius only on its translucent
	// classes, because a rounded corner over a background that cannot be
	// composited is a notch of whatever is behind it.
	radiusPt  int
	rule      color.RGBA // .rule background, under the captions
	sep       color.RGBA // separator background, between data rows
	hoverFill color.RGBA // .close:hover background
	text      [3]uint8   // label color, the cells and the symbols
	thead     [3]uint8   // .thead color, also .close
}

// panelPaletteFor builds a palette. translucent selects the halves of the GTK
// sheet whose colours carry alpha.
func panelPaletteFor(dark, translucent bool) panelPalette {
	if dark {
		p := panelPalette{
			sheet:     premul(28, 31, 38, 255),   // #1c1f26
			rule:      premul(255, 255, 255, 71), // rgba(255,255,255,0.28)
			sep:       premul(255, 255, 255, 26), // rgba(255,255,255,0.10)
			hoverFill: premul(255, 255, 255, 26), // rgba(255,255,255,0.10)
			text:      [3]uint8{0xf2, 0xf4, 0xf7},
			thead:     [3]uint8{0x9a, 0xa3, 0xb0},
		}
		if translucent {
			p.sheet = premul(28, 31, 38, 245) // rgba(28,31,38,0.96)
			p.radiusPt = sheetRadiusPt
		}
		return p
	}
	p := panelPalette{
		sheet:     premul(255, 255, 255, 255), // #ffffff
		rule:      premul(0, 0, 0, 61),        // rgba(0,0,0,0.24)
		sep:       premul(0, 0, 0, 26),        // rgba(0,0,0,0.10)
		hoverFill: premul(0, 0, 0, 20),        // rgba(0,0,0,0.08)
		text:      [3]uint8{0x14, 0x16, 0x1a},
		thead:     [3]uint8{0x5b, 0x64, 0x72},
	}
	if translucent {
		p.sheet = premul(255, 255, 255, 250) // rgba(255,255,255,0.98)
		p.radiusPt = sheetRadiusPt
	}
	return p
}

// panelPaletteSystem builds the palette for the system look: the desktop's own
// colours, so the panel matches the settings and About windows and everything
// else on the screen instead of carrying the app's design into a window that has
// stopped pretending to be a floating sheet.
//
// radiusPt stays 0 and hoverFill stays transparent. There are no rounded corners
// in this look and no close box to highlight, and leaving both at their zero
// values means paint() takes the square-corner path and roundRect declines the
// hover pill on its own alpha check rather than on a second flag.
//
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
		// The colours the rest of the program paints on a dark Windows, NOT the
		// app's own Modern palette. That is what used to be here, and it was
		// wrong in a way a screenshot showed at once: Modern's sheet is #1c1f26,
		// a near-black with a blue cast, so the panel sat visibly darker and
		// cooler than the About window next to it. This look is supposed to
		// disappear into the desktop, and on a dark desktop the closest thing to
		// the desktop this program can honestly claim is what it paints its other
		// windows with.
		//
		// Asked from darkmode_windows.go rather than written out here, because
		// About paints from the same constants - see the note there for why they
		// have to be constants at all rather than a GetSysColor call.
		sr, sg, sb := refRGB(darkSurface)
		tr, tg, tb := refRGB(darkText)
		hr, hg, hb := refRGB(darkTextDim)
		return panelPalette{
			sheet: premul(sr, sg, sb, 255),
			// The two separator weights are the Modern dark palette's, and they
			// stay: 0.28 and 0.10 of white are a RELATIVE statement about which
			// line is the header rule and which is a row hairline, not a colour
			// borrowed from a look this window is not in. They read the same over
			// #2d2d2d as over #1c1f26.
			rule:  premul(255, 255, 255, 71),
			sep:   premul(255, 255, 255, 26),
			text:  [3]uint8{tr, tg, tb},
			thead: [3]uint8{hr, hg, hb},
		}
	}

	wr, wg, wb := sysRGB(win.COLOR_WINDOW)
	tr, tg, tb := sysRGB(win.COLOR_WINDOWTEXT)
	hr, hg, hb := sysRGB(win.COLOR_GRAYTEXT)
	sr, sg, sb := sysRGB(win.COLOR_3DSHADOW)

	return panelPalette{
		sheet: premul(wr, wg, wb, 255),
		// COLOR_3DSHADOW is the theme's own divider shade - the line an etched
		// border is drawn with - which is what the header rule is. The hairlines
		// between data rows keep the same colour at the share of it the Modern
		// palette gives them, 0.10 of 0.28, so the two weights stay distinguishable
		// instead of collapsing into one flat grid.
		rule:  premul(sr, sg, sb, 255),
		sep:   premul(sr, sg, sb, 91), // 0.10/0.28 of the rule
		text:  [3]uint8{tr, tg, tb},
		thead: [3]uint8{hr, hg, hb},
	}
}

// sysRGB unpacks one system colour, refRGB any COLORREF. A COLORREF is
// 0x00bbggrr, so the byte order is the opposite of the way the value is usually
// written down - which is invisible for the greys in darkmode_windows.go and
// would bite the first time one of them is not grey.
func sysRGB(index int) (r, g, b uint8) {
	return refRGB(win.GetSysColor(index))
}

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

		// Before the fetch, so a click that only closes the panel does not spend
		// ten seconds on a result it will discard.
		if closeOpenPanel() {
			return
		}

		data, err := weather.FetchDaily(req.Lat, req.Lon)
		if err != nil || len(data) == 0 {
			if err != nil {
				log.Printf("forecast: fetch failed: %v", err)
			} else {
				log.Printf("forecast: fetch returned no days")
			}
			showError(l.ForecastFailed())
			return
		}

		p := newPanel(data, req, l)
		p.run(at, haveAt)
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
	sheet    win.RECT
	closeBox win.RECT
	head     [numCols]win.RECT
	rule     win.RECT
	rows     []rowGeom
}

type panelFonts struct {
	thead  win.HFONT
	cell   win.HFONT
	close  win.HFONT
	symbol win.HFONT // 0 when the Weather Icons face is not usable
}

// panelMetrics is everything the layout needs to know about the fonts and the
// strings, so that the arithmetic in layoutTable is a pure function of it.
type panelMetrics struct {
	theadH int32
	cellH  int32
	closeH int32
	closeW int32
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

	// req is kept whole rather than unpacked into fields because two of its
	// members are callbacks that must be called LATER: Pinned on every Escape
	// and every activation change, OnMove as the window closes. Copying their
	// answers into the struct at construction time is exactly the bug the
	// function-valued field exists to prevent.
	req gui.Forecast

	// sysLook is the appearance decided ONCE, in newPanel, from
	// req.Appearance. Every branch in this file reads this and never the string:
	// an unrecognised value has to behave as modern, and one switch that says so
	// is easier to keep honest than a comparison repeated a dozen times.
	sysLook bool
	// dark says draw dark, and the two looks answer it differently on purpose -
	// see newPanel.
	dark bool
	pal  panelPalette
	dpi  int32

	// frameW and frameH are the non-client overhead of this look, measured once
	// in build. Zero in Modern; the caption and borders in the system look.
	frameW, frameH int32

	heads []string
	rows  []tableRow

	// haveSymbols records whether the embedded typeface was registered with
	// GDI. Without it the symbol column stays blank rather than filling with
	// whatever glyphs a substitute face happens to have at those codepoints.
	haveSymbols bool

	fonts panelFonts
	geom  panelGeom

	x, y, w, h int32

	img  *image.RGBA
	surf *surface
	mask *surface

	// ulwFlags is settled by show() and reused by every repaint, so a hover
	// redraw cannot silently switch the window between opaque and blended.
	ulwFlags uint32

	// armed is set by the first real activation. Until then a WA_INACTIVE must
	// be ignored: SetForegroundWindow can be refused, and an unarmed panel
	// would then vanish before the user ever saw it.
	armed bool
	// closing collapses the several ways the panel can be dismissed into one
	// WM_CLOSE, so a second trigger cannot destroy the window twice.
	closing  bool
	hover    bool
	tracking bool
	// reported keeps OnMove to one call. The panel can be told to close twice -
	// a focus loss and the tray's own WM_CLOSE arrive independently - and the
	// caller writes a config file for each report it gets.
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
	// exitedMove records that WM_EXITSIZEMOVE has arrived for a move loop that is
	// still on the stack. run's message pump reads it to settle a loop THIS CODE
	// DID NOT START, which only the system look can produce: its caption and its
	// Alt+Space window menu both enter the system's move loop directly, so the
	// SendMessage in WM_LBUTTONDOWN - the only settling point Modern needs - is
	// never reached and moving would be stranded set for the rest of the panel's
	// life, leaving it immune to focus loss and unable to act on a deferred close.
	exitedMove bool
	// moving is true from WM_ENTERSIZEMOVE until settleDrag clears it, which is the
	// whole life of the system's modal move loop plus the instant it takes
	// SendMessage to return. It switches the focus-loss dismissal off for exactly
	// that long - see the WM_ACTIVATE case and settleDrag. Nothing else clears it,
	// least of all WM_EXITSIZEMOVE, which arrives from inside the loop.
	moving bool
	// deferredInactive is a WA_INACTIVE that arrived while moving was set and has
	// not been settled yet. settleDrag decides what becomes of it; a loop starting
	// discards any that is somehow still pending, since WA_INACTIVE never repeats
	// and a stale one could otherwise close the panel over a focus loss the user
	// has long since resolved.
	deferredInactive bool
	// deferredClose is a WM_CLOSE that the move loop's own pump delivered while
	// the drag was still on the stack. Unlike WA_INACTIVE this one is not a policy
	// decision to weigh up later - it is an explicit dismissal that simply has to
	// wait, because destroying the window from inside the system's move loop is
	// the re-entrancy the moving flag exists to prevent.
	//
	// It is reachable even though the mouse button is down: closeOpenPanel posts
	// WM_CLOSE from the tray's goroutine, so a second pointing device is enough.
	// Tap the tray icon with a finger on a 2-in-1 while the mouse holds the drag,
	// and the loop's pump dispatches it.
	deferredClose bool
	// byFocus says this panel is closing because it lost the foreground, and it
	// is the only cause allowed to arm the tray toggle's grace period. The grace
	// exists solely because a click on the notification area can take the
	// foreground and close the panel before the click itself is delivered, so a
	// click arriving just after such a close is the closing one rather than a new
	// opening one. Arming it after a deliberate close - the x button, the toggle,
	// Escape - answers a burst of taps with silence, which is exactly what a user
	// reported. forecast_linux.go carries the same flag for the same reason.
	byFocus bool
	// moveRectFailed keeps the WM_MOVE diagnostic to one line per panel.
	moveRectFailed bool
	// raster is reused by every roundRect call this panel makes, because a fresh
	// rasterizer per call costs 1.64 MB and about 5 ms of the repaint that runs
	// inside the window procedure - see roundRect.
	raster *vector.Rasterizer
	// paintFailed does the same for the WM_PAINT diagnostic, and for a stronger
	// reason: a window whose BeginPaint fails is asked to paint again immediately,
	// so an ungated line there is not a log entry but a log flood.
	paintFailed bool
}

func newPanel(data []weather.DailyForecast, req gui.Forecast, l i18n.Lang) *panel {
	// The one place the requested appearance is interpreted. A switch with a
	// default arm, not a comparison against "modern": gui.Forecast.Appearance
	// documents the empty string and anything unrecognised as modern, and a
	// config file edited by hand is the case that has to land there.
	sysLook := false
	switch req.Appearance {
	case "system":
		sysLook = true
	}

	// In Modern the theme option chooses the palette, which is what resolveDark
	// answers: an explicit "dark" or "light" overrides the desktop.
	//
	// In the system look it must NOT, because the desktop theme is now what paints
	// the window. The only question left is the one GetSysColor cannot answer -
	// whether Windows is drawing applications dark - so the preference is read
	// directly. Honouring icon_theme="light" here would put the app's light
	// palette in a window whose caption the DWM has drawn dark, which is the
	// mismatch this look exists to remove. The option still drives the tray icon
	// and still chooses the Modern palette; it simply has nothing to say about a
	// window painted by the desktop.
	dark := resolveDark(req.Theme)
	if sysLook {
		// High contrast wins over the dark-apps switch - see panelPaletteSystem for
		// why the substitution that is right for dark mode is wrong here.
		dark = systemDark() && !highContrastOn()
	}

	p := &panel{
		title:   l.ForecastTitle(),
		req:     req,
		sysLook: sysLook,
		dark:    dark,
		// Provisional: show() replaces it once the display has been asked
		// whether it will composite per-pixel alpha - a question the system look
		// never asks, since it composites nothing.
		pal:   panelPaletteFor(dark, true),
		heads: l.ForecastHeaders(),
	}
	for _, d := range data {
		var row tableRow
		// The ISO date exactly as Open-Meteo returned it. No weekday name and no
		// localised month: the column is a date, and a sortable one reads the
		// same in both languages.
		row.cell[0] = d.Date
		row.cell[colCond] = fonts.IconForCode(d.WeatherCode)
		row.cell[2] = weather.TempRange(d, req.Units, l)
		row.cell[3] = weather.WindSpeed(d, req.WindUnit, l)
		row.cell[4] = weather.Precip(d, l)
		p.rows = append(p.rows, row)
	}
	return p
}

// The owner of the system look's panel: one invisible, zero-sized window that is
// never shown and never painted.
//
// It exists because of what dropping WS_EX_TOOLWINDOW cost. That flag was doing
// two jobs at once - the thin palette caption, which was wrong and is gone, and
// keeping the window out of the taskbar and out of Alt+Tab, which the panel
// promises on every platform and the other two backends deliver. Windows leaves
// an OWNED top-level window out of both as well, and an owner is invisible, so
// the caption stays exactly the one an ordinary application window gets.
//
// One owner for the life of the process, not one per panel: it holds no state, it
// is never destroyed, and the process exiting takes it with it. A failure is
// logged and survivable - the panel then behaves as it would have with no owner
// at all, which is to say it picks up a taskbar button.
var (
	panelOwnerOnce sync.Once
	panelOwner     win.HWND
)

func ensurePanelOwner(inst win.HINSTANCE) win.HWND {
	panelOwnerOnce.Do(func() {
		cn := utf16Of(panelClassName)
		name := utf16Of("")
		panelOwner = win.CreateWindowEx(
			0,
			&cn[0], &name[0],
			win.WS_POPUP,
			0, 0, 0, 0,
			0, 0, inst, nil,
		)
		if panelOwner == 0 {
			log.Print("forecast: the owner window could not be created; the panel will show a taskbar button")
		}
	})
	return panelOwner
}

// styles is the window's shape in this look. CreateWindowEx and frameOverhead
// must both be fed from here, never from a literal, or the frame the layout pays
// for stops being the frame the window has.
func (p *panel) styles() (style, exStyle uint32) {
	if p.sysLook {
		return panelStyleSystem, panelExStyleSystem
	}
	return panelStyleModern, panelExStyleModern
}

// frame is how much larger the window is than the image it shows: the caption
// and the borders.
//
// Zero in Modern, and stated as a branch rather than left to the syscall: a bare
// WS_POPUP has no non-client pixels at all, AdjustWindowRectEx would answer 0,0
// for it, and there is no reason to make a call to be told so.
//
// It is asked about the SAME styles CreateWindowEx was given, both of them, and
// through the Ex form of the call. Passing anything else - or dropping the
// extended style, or using the non-Ex AdjustWindowRect - answers for a frame this
// window does not have, and the difference lands as client area below the
// composed image, which WM_PAINT does not cover and WM_ERASEBKGND deliberately
// does not erase. See adjustWindowRectEx.
//
// A failure answers 0,0 and says so. That is the safe direction to be wrong in:
// too small a window clips the bottom of the last row, where too large a one
// shows uninitialised memory, and a panel missing a hairline is easier to live
// with than one with a garbage stripe across it.
func (p *panel) frame() (w, h int32) {
	if !p.sysLook {
		return 0, 0
	}
	var rc win.RECT
	if !adjustWindowRectEx(&rc, panelStyleSystem, false, panelExStyleSystem) {
		log.Print("forecast: AdjustWindowRectEx failed; the panel will be sized without its frame")
		return 0, 0
	}
	return rc.Right - rc.Left, rc.Bottom - rc.Top
}

// windowSize is the size to give the WINDOW.
//
// p.w and p.h are always the CLIENT size: they size p.img and p.surf, and the
// composed image is exactly what the client area shows. In Modern the window is
// the client area, because a WS_POPUP with no border has no non-client pixels at
// all, and the two numbers are the same. In the system look the caption and
// borders have to be added on top - hand p.w/p.h straight to CreateWindowEx there
// and the caption eats the top of the table, because the client area comes out
// shorter than the image by exactly the caption's height.
//
// EVERY placement decision has to use this and not p.w/p.h: panelCorner and
// clampToWork both reason about the window's edges against the work area, and
// SetWindowPos is given a window rect. build's shrink-to-fit has to account for
// it too - see build.
// One consequence worth writing down, because it is invisible until a user
// switches looks: everything this file remembers as "the position" is
// GetWindowRect's, which is the FRAME in the system look and the window itself in
// Modern. The two are the same value only when there is no frame, so a position
// saved in one look and reused in the other is off by the caption height, once,
// until the next drag. gtk.Window.Position carries the same caveat with measured
// numbers; neither backend stores which look wrote a position, and a config field
// for a one-time nudge of about thirty pixels is not worth it.
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
	// panelClosedAt is when the panel last went away on its own, which
	// closeOpenPanel needs to tell a closing click from an opening one.
	panelClosedAt time.Time

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

// releasePanel frees the singleton. byFocus arms the tray toggle's grace period
// and must be true only for a close caused by losing the foreground - see the
// panel field of the same name.
func releasePanel(byFocus bool) {
	panelMu.Lock()
	panelBusy = false
	panelHWND = 0
	if byFocus {
		panelClosedAt = time.Now()
	}
	panelMu.Unlock()
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
// The grace period exists because of an ordering hazard outside this function's
// own logic. Clicking the tray icon can move focus away from the panel, and the
// panel closes itself on focus loss - so by the time the host delivers the click
// the panel may already be gone, and a naive toggle would see "not open" and
// open a new one. Treating a click that lands just after a focus-loss close as
// the closing click is what makes the second click dismiss the window rather
// than reopen it.
func closeOpenPanel() bool {
	panelMu.Lock()
	hwnd := panelHWND
	busy := panelBusy
	closedAt := panelClosedAt
	panelMu.Unlock()

	if busy {
		if hwnd != 0 {
			win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
		}
		return true
	}
	if !closedAt.IsZero() && time.Since(closedAt) < toggleGrace {
		// Worth a line: to the user this click did nothing at all.
		log.Print("forecast: click within the toggle grace period, treated as the closing click")
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
	// byFocus is read at return time, not now: the WndProc sets it on this same
	// thread when a focus loss is what closed the panel.
	defer func() { releasePanel(p.byFocus) }()

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
			//
			// One class serves both looks, because the difference between them is
			// entirely in the per-window styles CreateWindowEx is given. The null
			// brush is right for the system look too: its WM_PAINT BitBlt covers
			// every pixel of the client area, so a class brush would only paint
			// the same pixels twice and flicker while doing it.
			HbrBackground: 0,
			LpszClassName: &cn[0],
		}
		panelClassOK = win.RegisterClassEx(wc) != 0
	})
	if !panelClassOK {
		log.Printf("forecast: RegisterClassEx failed")
		return
	}

	// The owner comes first, and before pending is set below: it is created with
	// this same class, so a WM_NCCREATE arriving while pending is non-nil would
	// hand it the panel that is about to be built.
	var owner win.HWND
	if p.sysLook {
		owner = ensurePanelOwner(p.inst)
	}

	// Create INVISIBLE, at 1x1, in both looks. A layered window shows nothing
	// until UpdateLayeredWindow has been called for it, so creating it with
	// WS_VISIBLE only flashes an empty frame; the system look would flash
	// something worse, a 1x1 title bar in the top-left corner of the screen,
	// because its size and position are not known until build and show have run.
	panelMu.Lock()
	pending = p
	panelMu.Unlock()

	// The title is only ever seen in the system look, where it is the caption's
	// text. Modern has no caption to put it in, but it is passed there too: it is
	// also what the window reports to anything that asks its name.
	title := utf16Of(p.title)
	cn := utf16Of(panelClassName)
	style, exStyle := p.styles()
	p.hwnd = win.CreateWindowEx(
		exStyle,
		&cn[0], &title[0],
		style,
		0, 0, 1, 1,
		owner, 0, p.inst, nil,
	)
	if p.hwnd == 0 {
		panelMu.Lock()
		pending = nil
		panelMu.Unlock()
		log.Printf("forecast: CreateWindowEx failed")
		return
	}

	if p.sysLook && p.dark {
		// The caption is the one part of this window the app does not paint, and
		// DwmSetWindowAttribute is the only way to ask for a dark one. Without it
		// the client area drawn from the dark palette sits under a white title bar,
		// which is exactly the mismatch panelPaletteSystem's dark branch exists to
		// avoid. Modern has no caption, so it has nothing to ask for.
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
	// corner in both looks - it is what UpdateLayeredWindow is given as pptDst,
	// what SetWindowPos is given, and what GetWindowRect answers - so the size
	// weighed against the work area has to be the window's too. In Modern the two
	// are identical and this changes nothing.
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
		// DispatchMessage has returned, so nothing is on the stack between here
		// and GetMessage - in particular no modal move loop, since such a loop
		// runs INSIDE the DispatchMessage of the message that started it. That
		// makes this the one settling point that works for a loop this code did
		// not start. See settleForeignDrag.
		p.settleForeignDrag()
	}
}

// perPixelAlpha reports whether THIS display will honour the panel's per-pixel
// alpha, asked of the system rather than assumed.
//
// The condition is documented on ULW_ALPHA itself: "If the display mode is 256
// colors or less, the effect of this value is the same as the effect of
// ULW_OPAQUE." That is the trap, because the call still SUCCEEDS - it simply
// ignores the alpha channel - so failure is not detectable after the fact, and
// the translucent palette would then be composited as though it were opaque: the
// sheet's premultiplied 82% colour drawn flat, with the four rounded corners
// showing whatever the zeroed pixels outside the sheet happen to be, which is
// black. That is exactly the black-notch failure the GTK stylesheet keeps a
// solid palette for.
//
// A colour depth of 256 or fewer means at most 8 bits per pixel, and the depth
// is BITSPIXEL times PLANES: BITSPIXEL is "the number of adjacent color bits for
// each pixel" and PLANES "the number of color planes", so a planar 4-plane
// 1-bit device answers 1 and 4 and is correctly read as 4bpp. Zero bits means
// the question could not be answered, and the answer that always displays
// correctly is the opaque one.
//
// It must be a DC for the SCREEN. A memory DC reports the depth of the bitmap
// currently selected into it, which for a freshly created one is the default 1x1
// monochrome bitmap - 1bpp, and every desktop would fail the test.
func perPixelAlpha(hwnd win.HWND) bool {
	dc := win.GetDC(hwnd)
	if dc == 0 {
		return false
	}
	defer win.ReleaseDC(hwnd, dc)

	bits := win.GetDeviceCaps(dc, win.BITSPIXEL)
	planes := win.GetDeviceCaps(dc, win.PLANES)
	if planes < 1 {
		// Every raster display reports exactly one plane, so a zero here is a
		// driver that declined to answer rather than a plane-less device. Taking
		// it literally would multiply a perfectly good 32bpp depth to nothing
		// and make every desktop opaque.
		planes = 1
	}
	return bits*planes > 8
}

// show paints the panel in the palette this display can actually show and gets
// it onto the screen.
//
// The two palettes are the translucent and solid halves of the same design, and
// which one is used is decided by perPixelAlpha, never assumed. The solid one
// squares the sheet's corners and fills every pixel of the window opaquely, and
// the flag becomes ULW_OPAQUE, the documented "draw an opaque layered window".
func (p *panel) show() bool {
	if p.sysLook {
		// perPixelAlpha is not asked, because nothing here composites: the window
		// is opaque and the display's colour depth cannot make it anything else.
		// ulwFlags stays 0 for the same reason - it names a flag for a call this
		// look never makes.
		p.pal = panelPaletteSystem(p.dark)
		p.paint()

		// SetWindowPos does what UpdateLayeredWindow does for Modern: move and
		// resize in one call. It is given the WINDOW rect, and the window is sized
		// so its CLIENT area is exactly the composed image - see windowSize.
		//
		// SWP_NOZORDER because WS_EX_TOPMOST already put the window at the top of
		// the topmost band and this call has no business reordering it;
		// SWP_NOACTIVATE because the window is still hidden and activation is
		// forceForeground's job, after ShowWindow.
		winW, winH := p.windowSize()
		if !win.SetWindowPos(p.hwnd, 0, p.x, p.y, winW, winH,
			win.SWP_NOZORDER|win.SWP_NOACTIVATE) {
			// Same reasoning as the layered failure below: silence here leaves a
			// 1x1 window in the corner of the screen, which reads as "the tray icon
			// does nothing".
			log.Print("forecast: SetWindowPos failed; the panel cannot be placed")
			return false
		}
		// No InvalidateRect: the window is not visible yet, and the ShowWindow in
		// run generates the first WM_PAINT.
		return true
	}

	translucent := perPixelAlpha(p.hwnd)
	p.pal = panelPaletteFor(p.dark, translucent)
	p.ulwFlags = ulwAlpha
	if !translucent {
		p.ulwFlags = ulwOpaque
		log.Printf("forecast: the display cannot composite per-pixel alpha; drawing the panel opaque")
	}

	p.paint()
	ok, errno := p.push()
	if !ok {
		// A silent failure here produces an invisible window, which reads to
		// the user as "the menu item does nothing".
		log.Printf("forecast: UpdateLayeredWindow failed: %v", errno)
	}
	return ok
}

// push hands the finished surface to the compositor. The one call moves,
// resizes and repaints the window, which is why no SetWindowPos is needed.
//
// MODERN ONLY. UpdateLayeredWindow requires WS_EX_LAYERED and the system look
// does not have it, so a call from there would fail with
// ERROR_INVALID_PARAMETER and the panel would simply never appear. That is a
// programming error rather than a runtime condition, which is why it is logged
// as one instead of being quietly absorbed.
func (p *panel) push() (bool, syscall.Errno) {
	if p.sysLook {
		log.Print("forecast: push called in the system look; UpdateLayeredWindow needs WS_EX_LAYERED")
		return false, 0
	}
	if p.surf == nil {
		return false, 0
	}
	ptDst := win.POINT{X: p.x, Y: p.y}
	ptSrc := win.POINT{X: 0, Y: 0}
	size := win.SIZE{CX: p.w, CY: p.h}
	blend := win.BLENDFUNCTION{
		BlendOp:    blendOpSrcOver,
		BlendFlags: 0,
		// 255 is mandatory, not merely sensible: "Set the SourceConstantAlpha
		// value to 255 (opaque) when you only want to use per-pixel alpha
		// values." Anything less multiplies the whole panel down again. It is
		// read only under ULW_ALPHA; ULW_OPAQUE ignores pblend entirely.
		SourceConstantAlpha: 255,
		AlphaFormat:         win.AC_SRC_ALPHA,
	}
	return updateLayeredWindow(p.hwnd, 0, &ptDst, &size,
		p.surf.dc, &ptSrc, 0, &blend, p.ulwFlags)
}

// release frees every GDI object the panel owns.
//
// The DCs go first and the fonts second, which is the order that cannot leak.
// DeleteObject refuses to free a GDI object that is still selected into a DC,
// and a font that fails to delete leaks for the life of the process; deleting
// the DC discards whatever was selected into it, so by the time freeFonts runs
// there is provably no DC left to hold one. drawTextGroup does deselect the font
// it selected, but this way the invariant does not depend on that.
//
// Both dispose and freeFonts are idempotent and nil-safe, so release can be
// called twice - which it is, on the failure paths in run().
func (p *panel) release() {
	p.surf.dispose()
	p.mask.dispose()
	p.freeFonts()
	p.surf = nil
	p.mask = nil
	p.img = nil
	p.raster = nil
}

// pinned reports the dismissal policy AT THIS MOMENT.
//
// It is a call per event rather than a field, so unchecking the box in settings
// applies to the panel already on screen. A nil Pinned means "not pinned", which
// is the behaviour that existed before the option did, so every caller that does
// not care about it gets it by writing nothing.
//
// It runs on the panel's own message-pump thread, inside a window procedure, so
// the callback must not block: the tray satisfies that by answering from an
// atomic mirror of the config rather than from behind a mutex.
func (p *panel) pinned() bool {
	return p.req.Pinned != nil && p.req.Pinned()
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
// stored position as "put it back there" and the panel is pinned by default.
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

func (p *panel) requestClose() {
	if p.closing {
		return
	}
	p.closing = true
	// PostMessage rather than a direct DestroyWindow: the dismissal triggers
	// include WM_ACTIVATE, and destroying a window from inside the system's own
	// activation handling is re-entrant in a way nothing documents as safe.
	//
	// Posting is not enough on its own while the system's move loop is running,
	// because that loop pumps this thread's queue and would dispatch the posted
	// WM_CLOSE itself. That is what the moving flag is for; requestClose is
	// deliberately not the place to check it, since the caller is the one that
	// knows whether it can afford to wait.
	win.PostMessage(p.hwnd, win.WM_CLOSE, 0, 0)
}

// settleDrag disposes of a deactivation that arrived while the system's move
// loop was running, and MUST be called with the loop off the stack.
//
// THE CHOICE, stated once: the deferred deactivation is honoured only if the
// panel is still not the foreground window now that the drag is over, and
// dropped otherwise.
//
// Dropping it outright would be wrong. WA_INACTIVE during a drag comes from
// something taking the foreground with the mouse button still down - Win+L,
// Ctrl+Alt+Del, Win+D, a toast, a UAC prompt - and WA_INACTIVE is delivered on
// the TRANSITION, so a panel that is already inactive is never told again unless
// it is activated first. An unpinned panel would then be a topmost window over
// whatever the user switched to, with only its × button and the tray icon left
// to dismiss it, which is the opposite of what "closes when you look away"
// promised.
//
// Honouring it unconditionally would be equally wrong: the ordinary end of a
// drag hands the foreground straight back, and closing then would make the panel
// vanish the instant the user let go of it. So the foreground is re-read rather
// than assumed, and pinned is asked again as well, because the policy is read at
// the moment of the event and the close is the event now.
func (p *panel) settleDrag() {
	// THE ONLY PLACE the flag is cleared. WM_EXITSIZEMOVE deliberately leaves it
	// alone because that message arrives from inside the loop, which can still pump
	// afterwards; here the loop is provably off the stack because the SendMessage
	// that started it has returned. Clearing unconditionally rather than only when
	// something was deferred: a loop that somehow never sent WM_ENTERSIZEMOVE
	// leaves nothing to clear, and one that never let go of the flag would leave
	// the panel immune to focus loss for the rest of its life.
	p.moving = false
	// And with it the note that a loop had ended, so run's pump does not settle a
	// second time over an event this call has already disposed of.
	p.exitedMove = false

	if p.deferredClose {
		// An explicit dismissal outranks any judgement about the foreground, so it
		// is settled first and nothing else is considered. PostMessage directly
		// rather than through requestClose: requestClose already set closing when it
		// posted the WM_CLOSE that got deferred, so it would refuse to post a second
		// time and the panel would stay up for good.
		p.deferredClose = false
		p.deferredInactive = false
		win.PostMessage(p.hwnd, win.WM_CLOSE, 0, 0)
		return
	}

	if !p.deferredInactive {
		return
	}
	p.deferredInactive = false
	if p.pinned() {
		return
	}
	if win.GetForegroundWindow() == p.hwnd {
		log.Print("forecast: the foreground came back when the drag ended; staying open")
		return
	}
	log.Print("forecast: closing, the foreground was taken during the drag")
	p.byFocus = true
	p.requestClose()
}

// settleForeignDrag settles a move loop that this code did not start, and is
// called from run's message pump after every DispatchMessage.
//
// IT EXISTS BECAUSE OF THE SYSTEM LOOK'S CAPTION. In Modern the only way into the
// system's move loop is the SendMessage in WM_LBUTTONDOWN, and settleDrag is
// called on the line after it - that is the earliest point at which the loop is
// provably off the stack, and it is enough. A captioned window has two more ways
// in that never touch this window procedure's press handling at all: dragging the
// title bar, which DefWindowProc turns into a move by itself, and the Alt+Space
// window menu's Move command, which WS_SYSMENU brings with the close button.
// Neither reaches the SendMessage, so neither would ever be settled: moving would
// stay set for the rest of the panel's life, switching the focus-loss dismissal
// off permanently and - far worse - stranding any deferred WM_CLOSE, which would
// leave a panel that cannot be closed by the tray icon at all.
//
// Settling from WM_EXITSIZEMOVE instead is not an option, and the case for that
// message says why: it is sent from INSIDE the loop, which can still pump
// afterwards, so the WM_CLOSE this can post would be dispatched with the drag on
// the stack. Here DispatchMessage has returned and the loop is provably gone.
//
// The exitedMove test is what keeps this from firing mid-drag. A move loop pumps
// this thread's queue, but through its own GetMessage, not run's: run does not
// regain control until the whole loop has unwound, so seeing moving still set
// AFTER a WM_EXITSIZEMOVE means precisely "a loop ran to completion and nobody
// settled it".
func (p *panel) settleForeignDrag() {
	// No log line: in the system look this is what the ordinary end of a title-bar
	// drag looks like, and a line per drag is noise, not diagnosis.
	if !p.moving || !p.exitedMove {
		return
	}
	p.settleDrag()
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
//
// WHAT "FITS" MEANS DEPENDS ON THE LOOK. The loop measures the CONTENT - p.w and
// p.h are the client area - but what has to fit the work area is the whole
// window, so the system look's caption and borders are subtracted from the
// available space before the comparison. Skip that and a panel sized to exactly
// fill the work area has its caption pushed off the top of the screen, where the
// close button and the title-bar drag both live.
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

	p.img = image.NewRGBA(image.Rect(0, 0, int(p.w), int(p.h)))
	// One rasterizer for the panel's whole life. Reset sizes it per shape and grows
	// its buffer if it ever has to, so allocating it at the sheet's size - the
	// largest shape drawn - means the hover repaint allocates nothing at all.
	p.raster = vector.NewRasterizer(int(p.w), int(p.h))
	p.surf = newSurface(int(p.w), int(p.h))
	p.mask = newSurface(int(p.w), int(p.h))
	if p.surf == nil || p.mask == nil {
		log.Printf("forecast: CreateDIBSection failed for %dx%d", p.w, p.h)
		return false
	}
	return true
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
		close: panelFont(closePt, win.FW_BOLD, p.dpi),
	}
	if p.fonts.thead == 0 || p.fonts.cell == 0 || p.fonts.close == 0 {
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
	for _, f := range []win.HFONT{p.fonts.thead, p.fonts.cell, p.fonts.close, p.fonts.symbol} {
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
		closeH: m.lineHeight(p.fonts.close),
		closeW: m.width(closeGlyph, p.fonts.close),
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

	// The close row exists only in Modern: the system look's caption carries the
	// close button, so there is no glyph to draw and no room to reserve for one.
	p.geom, p.w, p.h = layoutTable(met, p.dpi, len(p.rows), !p.sysLook)
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
//
// closeRow is false in the system look, where the caption's own close button
// replaces the panel's. It collapses the close box to the zero RECT and reclaims
// the height it and its gap occupied, so the table starts at the top padding
// instead of under an empty band - and, because rectContains never matches a zero
// rectangle, it is also what turns off the three mouse hit tests without a second
// flag reaching them. THE TABLE ITSELF IS UNTOUCHED by it: every column edge, row
// height, gap and hairline below comes out identical in both looks, which is the
// promise the two backends' shared metrics rest on.
//
// m.closeW and m.closeH are simply ignored when closeRow is false. They are still
// measured, because makeFonts creates the close font either way: one unused HFONT
// is cheaper than a second failure path through makeFonts.
func layoutTable(m panelMetrics, dpi int32, n int, closeRow bool) (panelGeom, int32, int32) {
	s := func(v int) int32 { return scaleDPI(v, dpi) }

	pad := s(pagePad)
	w := s(panelWidthPt)
	contentL := pad
	contentR := w - pad
	cw := contentR - contentL

	closeBoxW := min(m.closeW+2*s(closePadX), cw)
	closeBoxH := m.closeH + 2*s(closePadY)
	// headOffset is what the close row costs the table: its own height plus the
	// page VBox spacing under it. Zero when there is no close row, and then the
	// close box is the zero RECT rather than a strip of nothing at the top.
	headOffset := closeBoxH + s(pageGapY)
	if !closeRow {
		closeBoxW, closeBoxH, headOffset = 0, 0, 0
	}

	gap := s(rowGapY)
	ruleTh := max(1, s(rulePt))
	rowH := max(m.cellH, m.symbolPx)

	rows := int32(n)
	tableH := m.theadH + gap + ruleTh + gap + rows*rowH
	if rows > 1 {
		tableH += (rows - 1) * (2*gap + ruleTh)
	}

	h := s(pagePadTop) + pad + headOffset + tableH

	var g panelGeom
	// The window IS the sheet: no inner card, and the padding is inside it. In the
	// system look the sheet is what fills the CLIENT area, with the caption above
	// it - see windowSize.
	g.sheet = rectAt(0, 0, w, h)
	if closeRow {
		g.closeBox = rectAt(contentR-closeBoxW, pad, closeBoxW, closeBoxH)
	}

	colL, colR := tableColumns(m.natural, contentL, contentR, s(colGapX))

	headY := pad + headOffset
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
	// Start fully transparent. In the translucent palette the only pixels the
	// design never claims are the four rounded corners, and their alpha 0 is
	// what lets a click there fall through to the desktop.
	//
	// WHAT THE ALPHA CHANNEL IS FOR, PER LOOK, because the difference is easy to
	// get wrong. In Modern it is the window: UpdateLayeredWindow reads it, alpha 0
	// is a pixel the window does not have, and GDI writing into this surface would
	// zero it - that is the rule panelpaint_windows.go is built around. In the
	// system look nothing reads it: the sheet is opaque so every pixel ends at
	// alpha 255, and BitBlt out of a BI_RGB DIB copies only the B, G and R bytes.
	// The composition is left byte for byte identical anyway - premul is the
	// identity at alpha 255, so premultiplied and plain are the same values - and
	// the "GDI never touches the panel surface" rule still holds there, now as
	// tidiness rather than as a load-bearing invariant. Do not weaken it on that
	// account: it is one pipeline serving both looks and both backends, and the
	// moment a DrawText lands on this surface it is Modern that breaks.
	for i := range p.img.Pix {
		p.img.Pix[i] = 0
	}

	g := &p.geom
	s := func(v int) int32 { return scaleDPI(v, p.dpi) }

	if r := s(p.pal.radiusPt); r > 0 {
		roundRect(p.raster, p.img, g.sheet.Left, g.sheet.Top,
			g.sheet.Right-g.sheet.Left, g.sheet.Bottom-g.sheet.Top,
			float32(r), p.pal.sheet)
	} else {
		// Square corners: no rasteriser, and therefore no degenerate Bezier at
		// each corner to reason about.
		paintRect(p.img, g.sheet.Left, g.sheet.Top,
			g.sheet.Right-g.sheet.Left, g.sheet.Bottom-g.sheet.Top, p.pal.sheet)
	}

	// The hover pill, and the × further down, are Modern's alone: the system look's
	// close button is in the caption, so there is no target to highlight and no
	// glyph to draw. Both are guarded explicitly rather than left to fall out of a
	// zero closeBox - the pill would, but drawTextGroup would still clear the mask,
	// flush GDI and composite a blank full-surface pass for a run it then skips.
	if p.hover && !p.sysLook {
		roundRect(p.raster, p.img, g.closeBox.Left, g.closeBox.Top,
			g.closeBox.Right-g.closeBox.Left, g.closeBox.Bottom-g.closeBox.Top,
			float32(s(closeRadiusPt)), p.pal.hoverFill)
	}

	paintRect(p.img, g.rule.Left, g.rule.Top,
		g.rule.Right-g.rule.Left, g.rule.Bottom-g.rule.Top, p.pal.rule)
	for i := range g.rows {
		sep := g.rows[i].sep
		// The last row's hairline is the zero RECT; paintRect draws nothing for
		// a zero-sized rectangle.
		paintRect(p.img, sep.Left, sep.Top,
			sep.Right-sep.Left, sep.Bottom-sep.Top, p.pal.sep)
	}

	const cellFlags = win.DT_VCENTER | win.DT_SINGLELINE | win.DT_NOPREFIX

	caps := make([]textRun, 0, numCols)
	for i := 0; i < numCols && i < len(p.heads); i++ {
		caps = append(caps, textRun{
			text:  p.heads[i],
			rect:  g.head[i],
			flags: colAlign[i] | cellFlags | win.DT_END_ELLIPSIS,
			font:  p.fonts.thead,
		})
	}

	cells := make([]textRun, 0, len(p.rows)*numCols)
	for r := range p.rows {
		for i := 0; i < numCols; i++ {
			if i == colCond {
				if p.fonts.symbol == 0 {
					continue
				}
				cells = append(cells, textRun{
					text: p.rows[r].cell[i],
					rect: g.rows[r].cell[i],
					// DT_NOCLIP, and it is load-bearing. The row is the height
					// of the symbol's BOX, matching the GtkImage on the other
					// side, but GDI positions text by its LINE box, which in
					// this typeface is 1.45 em - taller than the row. Clipping
					// to the row would shave the top and bottom off every
					// symbol. What overhangs is mostly the font's empty ascent
					// and descent: the ink is about one em, centred to within a
					// sixth of an em, so it reaches at most about a quarter of
					// the row beyond the edge and stays clear of the hairline
					// rowGapY away. Nothing can escape the bitmap either - GDI
					// clips to the DC.
					//
					// No ellipsis on a single glyph either: DT_END_ELLIPSIS
					// would replace the symbol with "..." rather than shrink it.
					flags: colAlign[i] | cellFlags | win.DT_NOCLIP,
					font:  p.fonts.symbol,
				})
				continue
			}
			cells = append(cells, textRun{
				text:  p.rows[r].cell[i],
				rect:  g.rows[r].cell[i],
				flags: colAlign[i] | cellFlags | win.DT_END_ELLIPSIS,
				font:  p.fonts.cell,
			})
		}
	}

	// The symbols share the cells' group because they share its colour: the
	// palette's foreground, which is the same value forecast_linux.go tints its
	// rasterised glyphs with. One group is one full-surface composite pass, so
	// folding them in is free. In the system look that foreground is the theme's
	// own ink - COLOR_WINDOWTEXT - for the same reason: the symbols are text.
	drawTextGroup(p.img, p.mask, caps, p.pal.thead[0], p.pal.thead[1], p.pal.thead[2])
	drawTextGroup(p.img, p.mask, cells, p.pal.text[0], p.pal.text[1], p.pal.text[2])

	if !p.sysLook {
		closeColour := p.pal.thead
		if p.hover {
			closeColour = p.pal.text
		}
		drawTextGroup(p.img, p.mask, []textRun{{
			text:  closeGlyph,
			rect:  g.closeBox,
			flags: win.DT_CENTER | cellFlags,
			font:  p.fonts.close,
		}}, closeColour[0], closeColour[1], closeColour[2])
	}

	p.surf.blitFrom(p.img)
}

// repaint redraws the surface and gets it back onto the screen.
//
// In Modern that is another UpdateLayeredWindow, which always updates the entire
// window, so there is no partial-invalidate path. In the system look it is an
// InvalidateRect and the WM_PAINT that follows, which is the only way to reach a
// window that is not layered.
//
// It is reachable only from the hover paths today, and those are Modern's alone,
// so the system look's half of this is insurance rather than a live path. It is
// here because the alternative - a repaint that silently did nothing, or worse
// called push - is the kind of thing the next hover-like feature would trip over.
func (p *panel) repaint() {
	p.paint()
	if p.sysLook {
		if !win.InvalidateRect(p.hwnd, nil, false) {
			log.Print("forecast: InvalidateRect failed on repaint; the panel may show stale content")
		}
		return
	}
	if ok, errno := p.push(); !ok {
		log.Printf("forecast: UpdateLayeredWindow failed on repaint: %v", errno)
	}
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
		// A top-level window with no child controls holds the focus itself, so
		// VK_ESCAPE arrives here directly - true of the WS_POPUP in Modern and of
		// the captioned window in the system look alike, since neither has a child
		// to hand the focus to. IsDialogMessage is deliberately absent from the
		// message loop: there are no controls to navigate and it would swallow keys.
		if wParam == win.VK_ESCAPE {
			// A pinned panel swallows Escape rather than passing it on: there is
			// nothing else in this window that wants the key, and the point of
			// the option is that a stray Escape aimed at another window cannot
			// take the forecast down with it.
			if !p.pinned() {
				p.requestClose()
			}
			return 0
		}

	case win.WM_ACTIVATE:
		if win.LOWORD(uint32(wParam)) == win.WA_INACTIVE {
			// Pinned is checked here and not once at open time so that
			// unchecking the box in settings applies to this window: the
			// settings window itself takes the activation, and this is the
			// message that would otherwise have closed the panel underneath it.
			if p.armed && !p.pinned() {
				if p.moving {
					// RE-ENTRANCY, and the one hazard this window has that
					// nothing else in it does. The system's move loop pumps this
					// thread's queue while it runs, so it is the loop's own pump
					// that delivered this WA_INACTIVE, and it is the loop's own
					// pump that would dispatch the WM_CLOSE requestClose posts:
					// DestroyWindow and the GDI release would run inside the
					// system's move loop with the WM_LBUTTONDOWN frame that
					// started it still on the stack, and by the time SendMessage
					// returned the surfaces would be freed and the live-window
					// entry gone. That is exactly the re-entrancy requestClose
					// exists to avoid, reopened by the drag.
					//
					// It cannot be a genuine click-away either: the drag holds
					// the mouse, so the press that would have landed on another
					// window has not happened. Deferring costs nothing that a
					// user can perceive - see settleDrag for what becomes of it.
					p.deferredInactive = true
					log.Print("forecast: focus lost during a drag; deferring the close until the move loop ends")
					return 0
				}
				// Any window taking activation lands here, not just one the
				// user clicked - a notification or a background window will
				// close the panel too. The line is here because an unexplained
				// disappearance is otherwise indistinguishable from a crash.
				log.Print("forecast: closing, focus lost")
				p.byFocus = true
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
		if p.sysLook {
			// Nothing to track. The close button is in the caption, so there is no
			// in-window target whose hover state could change, and asking for
			// TME_LEAVE would buy a WM_MOUSELEAVE that nothing acts on. This is the
			// "no hover tracking for a button that is not there" half of the look.
			return 0
		}
		x, y := clientPoint(lParam)
		if !p.tracking {
			// Without a leave notification the close button stays highlighted
			// after the pointer exits through a transparent corner, where no
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

	case win.WM_LBUTTONDOWN:
		// Dragging. A press anywhere on the body except the close button is
		// handed to the system's own modal move loop, which is what a title bar
		// does with a press on itself: ReleaseCapture to give up the implicit
		// capture this very message granted - the loop cannot start while
		// another window holds the mouse - and then WM_NCLBUTTONDOWN with
		// HTCAPTION, which is precisely the message DefWindowProc turns into a
		// move. What the user expects of a drag comes free that way: the Escape
		// that cancels it and puts the window back where it started, the Alt-Tab
		// that interrupts it. Reimplementing it with SetCapture and WM_MOUSEMOVE
		// gives up both and gains nothing, because the window's position is
		// re-read from GetWindowRect on WM_MOVE either way.
		//
		// What does NOT come free, and was claimed here in error: Aero Snap.
		// Windows offers edge snapping only for windows it considers resizable,
		// and neither look carries WS_THICKFRAME or WS_MAXIMIZEBOX - Modern is a
		// bare WS_POPUP, the system look a WS_CAPTION|WS_SYSMENU window with no
		// size box - so dragging either against an edge snaps to nothing.
		//
		// NOT a WM_NCHITTEST that answers HTCAPTION for the whole client area,
		// which is the shortcut every Win32 sample reaches for. It declares the
		// entire window a caption, and a caption does not deliver WM_LBUTTONUP,
		// WM_MOUSEMOVE or WM_MOUSELEAVE to the client area at all: the close
		// button stops closing and its hover highlight stops appearing, and both
		// failures look like a painting bug rather than a hit-testing one.
		//
		// SendMessage, not PostMessage: the move loop must run inside this
		// message's own handling, while the button is still physically down.
		//
		// The close button is excluded HERE, at press time, rather than tidied up
		// afterwards, because the move loop consumes the matching WM_LBUTTONUP -
		// the button release is what ends the loop, and it is never delivered to
		// this window procedure. A drag started on the × would therefore swallow
		// the click that was meant to close the panel, and no amount of
		// bookkeeping in WM_LBUTTONUP can recover an event that never arrives.
		//
		// In the system look there is no close box and geom.closeBox is the zero
		// RECT, which rectContains never matches, so the whole client area is
		// draggable - and the caption is draggable too, by DefWindowProc, which
		// starts the very same loop without this handler ever seeing the press.
		// settleForeignDrag is what copes with that.
		x, y := clientPoint(lParam)
		if rectContains(p.geom.closeBox, x, y) {
			return 0
		}
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
		// SendMessage has returned, so the system's move loop is provably off the
		// stack and a WM_CLOSE can no longer be dispatched from inside it. This -
		// not WM_EXITSIZEMOVE, which the loop sends while it is still running - is
		// the earliest place a deferred dismissal is safe to act on.
		p.settleDrag()
		return 0

	case win.WM_ENTERSIZEMOVE:
		// The system's modal move loop has started. It runs a message pump of its
		// own on this thread until the drag ends, and while it does, WA_INACTIVE
		// must not close the panel - the WM_ACTIVATE case above says why.
		//
		// Two dismissals need holding back, and only two. WA_INACTIVE is one; the
		// WM_CLOSE that closeOpenPanel posts from the tray's goroutine is the other,
		// and the WM_CLOSE case holds it the same way. It is tempting to argue that
		// one cannot arrive during a drag - the notification area needs a click, and
		// clicking means letting go of the button - but that assumes a single
		// pointing device. A finger on a 2-in-1, a pen, or injected input all reach
		// the tray while the mouse still holds the drag.
		//
		// Escape needs nothing: it goes to the loop itself and cancels the move
		// rather than reaching this window procedure. Neither does the close button,
		// which cannot be clicked while the button is already down.
		p.moving = true
		// Any deferral still pending belongs to an earlier loop that was never
		// settled, and this loop's settleDrag would otherwise act on it - closing
		// the panel over a focus loss the user resolved long ago. WA_INACTIVE is
		// delivered on the TRANSITION and never repeats, so a stranded deferral can
		// never be corrected by a later message either; discarding it here is the
		// only cheap correction available.
		//
		// NO LONGER LATENT, and that is what settleForeignDrag is for. Modern is
		// still the case this comment described: settleDrag runs after every loop
		// THIS code starts, immediately after the SendMessage in WM_LBUTTONDOWN,
		// and a bare WS_POPUP has no other way in - no caption to drag, and no
		// Alt+Space window menu without WS_SYSMENU. The system look has both, so a
		// loop can now end without WM_LBUTTONDOWN ever having been involved, and
		// run's pump settles those. Discarding a stale deferral here is unchanged
		// either way, and matters more now that more loops exist to strand one.
		p.deferredInactive = false
		p.exitedMove = false

		// The origin for the position report, when the loop was not started by
		// WM_LBUTTONDOWN. That handler records it before its SendMessage, so in
		// Modern this is always a no-op; in the system look a title-bar drag arrives
		// here having recorded nothing, and without this the panel would be moved by
		// the one gesture a captioned window makes most obvious and then forget
		// where it was put, because reportMove refuses to report without a handoff.
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

	case win.WM_EXITSIZEMOVE:
		// Deliberately does NOT clear the moving flag. This is sent as the loop
		// unwinds but STILL FROM INSIDE IT, and the loop can pump more messages
		// before the SendMessage in WM_LBUTTONDOWN returns: a WA_INACTIVE arriving
		// in that window would find moving already false, post WM_CLOSE, and the
		// loop's own pump could dispatch DestroyWindow inside itself - the exact
		// re-entrancy the flag was added to close. settleDrag is the only settler,
		// it clears the flag unconditionally the moment SendMessage returns, and
		// that is the earliest point at which the loop is provably off the stack.
		// Clearing here would buy nothing and is pure exposure.
		//
		// It does RECORD that the loop has ended, which is all run's pump needs to
		// settle a loop this code did not start. A note is not an action: nothing
		// reads it until DispatchMessage has returned, by which time the loop is
		// gone. See settleForeignDrag.
		p.exitedMove = true
		return 0

	case win.WM_MOVE:
		// The move loop repositions the window behind this code's back, and
		// p.x/p.y are what push() hands UpdateLayeredWindow as the DESTINATION of
		// every repaint. Left stale, the next hover redraw would teleport the
		// panel back to where it opened. Reading the window rect rather than
		// unpacking lParam keeps this independent of the client-origin equality
		// that only holds because the window has no frame - and the system look is
		// exactly the case where it does not hold: lParam carries the CLIENT
		// origin, which under a caption sits below and inside the window's own
		// top-left, so unpacking it would drift the panel up and left by the frame
		// on every move. p.x/p.y are the WINDOW's position in both looks.
		var rc win.RECT
		if win.GetWindowRect(hwnd, &rc) {
			p.x, p.y = rc.Left, rc.Top
		} else if !p.moveRectFailed {
			// Two visible failures from one swallowed error, which is why this is
			// the one GetWindowRect in this file that must not fail quietly: the
			// next hover repaint pushes the stale destination and drags the panel
			// back to where it opened, and since that is also the recorded origin,
			// the drag then reports nothing when the panel closes.
			//
			// Once per panel, not once per message: WM_MOVE arrives dozens of times
			// a second while the window is being dragged, and whatever makes this
			// call fail will make all of them fail.
			p.moveRectFailed = true
			log.Print("forecast: GetWindowRect failed on WM_MOVE; the panel may snap back on the next repaint")
		}
		return 0

	case win.WM_LBUTTONUP:
		// Hit testing on a layered window follows the alpha channel: in Modern the
		// four rounded corners, where the alpha is zero, let a click through to
		// whatever is underneath, which deactivates this window and closes it
		// through WM_ACTIVATE above. Everywhere else lands here, and only the
		// close button does anything with it.
		//
		// The system look is an ordinary opaque rectangle: nothing falls through,
		// and geom.closeBox is the zero RECT, so this arrives and does nothing.
		// Clicks on the caption never reach it at all - they are non-client and go
		// to DefWindowProc, which is how its close button works.
		x, y := clientPoint(lParam)
		if rectContains(p.geom.closeBox, x, y) {
			p.requestClose()
		}
		return 0

	case win.WM_PAINT:
		// MODERN FALLS THROUGH. Its surface belongs to the compositor, which
		// repaints it without asking; the message is left to DefWindowProc exactly
		// as it was before this case existed. Something must answer it either way -
		// a WM_PAINT that does not validate the update region is sent again
		// immediately, forever.
		if p.sysLook {
			var ps win.PAINTSTRUCT
			dc := win.BeginPaint(hwnd, &ps)
			if dc != 0 {
				// The whole client area in one BitBlt, not ps.RcPaint: the client
				// area IS the composed image, pixel for pixel - windowSize added
				// the frame on the outside - so there is nothing to be gained by
				// clipping to the damaged part of an image that is already in
				// memory, and a partial blit is one more thing to get wrong.
				//
				// A failure needs no branch. blitTo answers false only for a
				// disposed surface or a null DC, and either way the client area
				// keeps whatever the system last showed rather than becoming
				// garbage: WM_ERASEBKGND painted nothing over it.
				p.surf.blitTo(dc)
				win.EndPaint(hwnd, &ps)
				return 0
			}
			if !p.paintFailed {
				p.paintFailed = true
				log.Print("forecast: BeginPaint failed; leaving the repaint to DefWindowProc")
			}
			// No EndPaint: there is no PAINTSTRUCT to pass it. Falling through to
			// DefWindowProc is what keeps this from spinning the thread at 100% -
			// it does its own BeginPaint/EndPaint pair and validates the region.
		}

	case win.WM_ERASEBKGND:
		// "I erased it", in both looks and for two different reasons. In Modern
		// nothing may draw on the surface at all - UpdateLayeredWindow owns it. In
		// the system look the WM_PAINT above covers every pixel of the client area
		// from the DIB, so erasing first would only paint those pixels twice and
		// flicker while doing it. The client area cannot be larger than the image
		// either: it is set from windowSize, and the panel is far wider than the
		// SM_CXMIN a captioned window's minimum tracking size could clamp it to -
		// 620 layout units is 465 pixels even at the minLayoutDPI floor.
		return 1

	case win.WM_CLOSE:
		// Every dismissal funnels through here - Escape, focus loss, the close
		// button, and the tray's own PostMessage in closeOpenPanel - which makes
		// it the one place the final position can be read while the window still
		// exists. WM_DESTROY would be too late for GetWindowRect to mean
		// anything.
		//
		// THE SYSTEM LOOK'S CAPTION CLOSE BUTTON LANDS HERE TOO, and that is the
		// whole reason the position survives in that look: there is no in-window ×
		// to route through requestClose, so this is the main way out. The chain is
		// all DefWindowProc's, and this file breaks no link in it - a click on the
		// caption's close box hit-tests as HTCLOSE, DefWindowProc turns the release
		// into WM_SYSCOMMAND with SC_CLOSE, and DefWindowProc's SC_CLOSE handling
		// sends WM_CLOSE. Neither WM_NCLBUTTONUP nor WM_SYSCOMMAND is intercepted
		// above, so nothing swallows it, and the reportMove below runs for a caption
		// close exactly as it does for the tray toggle.
		if p.moving {
			// Dispatched by the move loop's own pump, with the drag still on the
			// stack. Destroying the window here would free the surfaces underneath
			// the loop that is driving this very HWND. Hold it until settleDrag.
			p.deferredClose = true
			return 0
		}
		p.reportMove()
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
