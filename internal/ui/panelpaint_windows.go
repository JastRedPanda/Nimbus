//go:build windows

package ui

// Everything the forecast panel draws with: the off-screen buffer it is drawn
// into, the fonts it is drawn in, and the memory DC the layout is measured
// against.
//
// The panel is drawn with ordinary GDI - FillRect for the sheet and the two
// weights of rule, DrawTextEx for every caption, cell and weather symbol - the
// same calls about_windows.go paints itself with, and the weather typeface is
// just another face to ask for, because internal/fonts has registered it with
// GDI privately.
//
// It was not always. The panel used to be a WS_EX_LAYERED sheet fed to
// UpdateLayeredWindow, which reads a PREMULTIPLIED 32bpp bitmap, and every GDI
// drawing call writes three bytes of every four and leaves the fourth as it
// found it - so a DrawText on that bitmap produced the right glyphs at alpha 0,
// which is to say nothing at all. The panel was therefore composed by hand in Go
// into an image.RGBA, and GDI was allowed exactly one job: rendering strings into
// a separate black-and-white scratch bitmap whose grey levels were read back as
// glyph coverage. That window is gone, there is no alpha channel left to
// protect, and the composition went with it.
//
// One thing outlives it. Colours the design states with an alpha - the header
// rule, the row hairlines - have to be flattened into solid COLORREFs before
// they can be drawn, because GDI has no alpha. That is done once, where the
// palette is built: see panelPaletteSystem in forecast_windows.go.

import (
	"syscall"

	"github.com/lxn/win"
)

// ---------------------------------------------------------------------------
// The back buffer
// ---------------------------------------------------------------------------

// backBuffer is the panel's client area drawn off screen: a memory DC with a
// screen-compatible bitmap selected into it.
//
// It earns its place twice over. WM_PAINT must not show the window assembling
// itself, and the panel is a background, a rule, a hairline between each pair of
// its seven rows and some forty strings - a sequence long enough to watch if it
// ran against the window's own DC. And the content cannot change while the panel is open: it is a
// snapshot of one fetch in a window that cannot be resized, so drawing it once
// and blitting it on every WM_PAINT is not merely flicker-free but does no
// drawing at all per repaint.
//
// The bitmap is a plain device-compatible one, NOT the top-down 32bpp DIB
// section this used to be. Nothing reads its bytes any more - GDI writes them
// and BitBlt copies them - so the pixel format is the display's business rather
// than this file's.
type backBuffer struct {
	dc     win.HDC
	bmp    win.HBITMAP
	oldBmp win.HGDIOBJ
	w, h   int32
}

// newBackBuffer builds a buffer w by h pixels.
//
// ref must be a DC of a REAL DEVICE - a window's, typically - because the bitmap
// is made compatible with it. CreateCompatibleBitmap against a memory DC answers
// a bitmap compatible with what that DC currently holds, and a fresh memory DC
// holds the 1x1 monochrome bitmap it is born with, so the panel would come out
// in two colours. The memory DC itself is created from ref for the same reason.
func newBackBuffer(ref win.HDC, w, h int32) *backBuffer {
	if ref == 0 || w <= 0 || h <= 0 {
		return nil
	}
	dc := win.CreateCompatibleDC(ref)
	if dc == 0 {
		return nil
	}
	bmp := win.CreateCompatibleBitmap(ref, w, h)
	if bmp == 0 {
		win.DeleteDC(dc)
		return nil
	}
	b := &backBuffer{dc: dc, bmp: bmp, w: w, h: h}
	b.oldBmp = win.SelectObject(dc, win.HGDIOBJ(bmp))
	return b
}

// dispose frees the DC and the bitmap. It is nil-safe and idempotent, because
// the failure paths in run() call it after the window procedure may already
// have.
//
// The original bitmap is selected back first: DeleteObject refuses to free a GDI
// object that is still selected into a DC, and a bitmap the size of the panel
// leaking once per showing is not a leak this program can afford - it lives in
// the tray for weeks.
func (b *backBuffer) dispose() {
	if b == nil || b.dc == 0 {
		return
	}
	if b.oldBmp != 0 {
		win.SelectObject(b.dc, b.oldBmp)
	}
	win.DeleteObject(win.HGDIOBJ(b.bmp))
	win.DeleteDC(b.dc)
	b.dc = 0
	b.bmp = 0
}

// blitTo copies the whole buffer onto a device context at its origin, which is
// how the panel reaches the screen. The destination is the window's client area,
// obtained from BeginPaint inside WM_PAINT.
//
// The whole buffer, never a sub-rectangle: the buffer IS the client area, pixel
// for pixel - windowSize adds the caption and borders on the outside - so this
// single call is what makes "every pixel of the client area is written on every
// paint" true, and WM_ERASEBKGND's refusal to erase safe.
func (b *backBuffer) blitTo(dc win.HDC) bool {
	if b == nil || b.dc == 0 || dc == 0 {
		return false
	}
	return win.BitBlt(dc, 0, 0, b.w, b.h, b.dc, 0, 0, win.SRCCOPY)
}

// ---------------------------------------------------------------------------
// Brushes and text
// ---------------------------------------------------------------------------

// createSolidBrush wraps HBRUSH CreateSolidBrush(COLORREF crColor), which
// lxn/win does not export.
//
// darkmode_windows.go already reaches for that gdi32 entry through
// createSolidBrushProc, for the two fixed greys it names outright; its lazy proc
// is shared here rather than declared a second time. Not to economise on the
// handle - LoadLibrary is reference counted, which is why panelapi_windows.go
// opens a gdi32 handle of its own without apology - but because a second NewProc
// for an entry point that already has one is a duplicate declaration of
// something that works.
//
// What the panel needs and those two wrappers cannot give is the general form:
// its fills are COMPUTED - the system's window colour, and two rules flattened
// out of an alpha - so the colour is not known until runtime.
func createSolidBrush(c win.COLORREF) win.HBRUSH {
	ret, _, _ := createSolidBrushProc.Call(uintptr(c))
	return win.HBRUSH(ret)
}

// drawText draws one string inside a rectangle, in whatever font, text colour
// and background mode are currently selected into dc. Callers set those once per
// group rather than per string, which is why they are not parameters here.
//
// The rectangle is taken BY VALUE and its address handed to Win32 from this
// frame: DrawTextEx's lpRect is [in,out], so the caller's copy - which is the
// layout's own geometry, drawn from more than once - must not be the one it is
// given.
//
// An empty string or a collapsed rectangle draws nothing. Both are reachable: a
// locale can be short of a caption, and tableColumns can scale a column to
// nothing on a very narrow layout.
func drawText(dc win.HDC, s string, rc win.RECT, flags uint32) {
	if s == "" || rc.Right <= rc.Left {
		return
	}
	u := utf16Of(s)
	win.DrawTextEx(dc, &u[0], -1, &rc, flags, nil)
}

// ---------------------------------------------------------------------------
// Fonts
// ---------------------------------------------------------------------------

// panelFont builds a font at a POINT size scaled for the window's DPI, which is
// how every type size in the panel except the weather symbol is stated - they
// come from `font-size: 11pt` and `font-size: 13pt` in style_linux.go. There are
// 72 points to the inch and dpi pixels, hence the conversion.
//
// The face comes from the user's own shell font, not a hardcoded "Segoe UI", so
// a locale whose script that face cannot draw still gets readable text.
func panelFont(pt, weight, dpi int32) win.HFONT {
	return panelFontPx(win.MulDiv(pt, dpi, 72), weight, shellFontFace())
}

// panelFontPx builds a font from an em height already in DEVICE PIXELS, which is
// how the one size stated in pixels rather than points is asked for: the weather
// symbol, whose counterpart on the GTK side is a rasterised tile that many
// pixels on a side.
//
// LfHeight is NEGATIVE: negative is the character height - the em - and positive
// is the cell height including internal leading. Mixing them up makes every font
// come out about 20% too small, and for the weather typeface, whose internal
// leading is 45% of its em, very much more than that.
//
// LfQuality is CLEARTYPE_QUALITY, the same the rest of this package asks for. It
// used to be ANTIALIASED_QUALITY, and that was not a preference: the glyphs were
// rendered into a scratch bitmap and read back as one coverage value per pixel,
// where subpixel antialiasing answers three. Nothing reads glyph pixels any more
// - they go straight onto an opaque background - so the panel gets the same
// subpixel rendering as every other window on the desktop, which is what it
// should have looked like all along.
//
// face is passed rather than assumed because this is also how the embedded
// weather typeface is requested - internal/fonts has registered it with GDI
// privately, so it can be asked for by family name like any installed font.
// CreateFontIndirect does NOT fail on a face name GDI cannot find: the font
// mapper substitutes the closest match it can, so a handle for a named face has
// to be checked with faceOf before it is trusted to draw private-use codepoints.
func panelFontPx(px, weight int32, face string) win.HFONT {
	lf := win.LOGFONT{
		LfHeight:         -px,
		LfWeight:         weight,
		LfCharSet:        win.DEFAULT_CHARSET,
		LfOutPrecision:   win.OUT_DEFAULT_PRECIS,
		LfClipPrecision:  win.CLIP_DEFAULT_PRECIS,
		LfQuality:        win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	setFaceName(&lf, face)
	return win.CreateFontIndirect(&lf)
}

// faceOf reports the typeface GDI actually realised for f, which is not
// necessarily the one that was asked for. A false second return means the
// question could not be put - no memory DC, or no GetTextFace export - and the
// caller should not read a substitution into the silence.
func faceOf(f win.HFONT) (string, bool) {
	if f == 0 {
		return "", false
	}
	m := newMeasureDC()
	defer m.dispose()
	if m == nil {
		return "", false
	}
	// Deferred second, so it runs FIRST: the font is deselected before the DC
	// goes, and therefore before the caller can be told to delete it.
	prev := win.SelectObject(m.dc, win.HGDIOBJ(f))
	defer win.SelectObject(m.dc, prev)

	name, ok := getTextFace(m.dc)
	return name, ok
}

// ---------------------------------------------------------------------------
// Measurement
// ---------------------------------------------------------------------------

// measureDC is a memory DC used only to ask GDI about font metrics. Text
// extents come from the selected font and the mapping mode, never from the
// bitmap, so the default 1x1 monochrome bitmap a fresh memory DC carries is
// enough - and it has to be, because the back buffer cannot be sized until
// these measurements are in.
type measureDC struct{ dc win.HDC }

func newMeasureDC() *measureDC {
	dc := win.CreateCompatibleDC(0)
	if dc == 0 {
		return nil
	}
	return &measureDC{dc: dc}
}

func (m *measureDC) dispose() {
	if m == nil || m.dc == 0 {
		return
	}
	win.DeleteDC(m.dc)
	m.dc = 0
}

// lineHeight is the height of one line of text in f: ascent plus descent, which
// is what DT_SINGLELINE occupies.
func (m *measureDC) lineHeight(f win.HFONT) int32 {
	if m == nil || f == 0 {
		return 0
	}
	prev := win.SelectObject(m.dc, win.HGDIOBJ(f))
	defer win.SelectObject(m.dc, prev)

	var tm win.TEXTMETRIC
	if !win.GetTextMetrics(m.dc, &tm) {
		return 0
	}
	return tm.TmHeight
}

// width is the advance width of s in f.
func (m *measureDC) width(s string, f win.HFONT) int32 {
	if m == nil || f == 0 || s == "" {
		return 0
	}
	prev := win.SelectObject(m.dc, win.HGDIOBJ(f))
	defer win.SelectObject(m.dc, prev)

	u := utf16Of(s)
	var sz win.SIZE
	if !win.GetTextExtentPoint32(m.dc, &u[0], int32(len(u)-1), &sz) {
		return 0
	}
	return sz.CX
}

// utf16Of returns a NUL-terminated UTF-16 encoding of s, always with at least
// the terminator so &u[0] is safe to take.
func utf16Of(s string) []uint16 {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return []uint16{0}
	}
	return u
}
