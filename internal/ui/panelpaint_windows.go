//go:build windows

package ui

// Everything the forecast panel draws with.
//
// THE ONE RULE THAT SHAPES THIS FILE: GDI must never touch the panel surface.
// Every GDI drawing call writes the R, G and B bytes of a 32bpp DIB and leaves
// the fourth byte exactly as it found it. On a surface whose alpha is
// meaningful that produces shapes and glyphs in the right colour with alpha 0 -
// invisible content on a window that is otherwise pixel perfect, which is the
// single most likely way this code can look broken.
//
// So the panel is composed in pure Go into a premultiplied image.RGBA and
// copied into the DIB at the end. GDI is used for exactly one thing: turning a
// string into glyph coverage, in a SEPARATE scratch DIB that is white on black
// and therefore has no alpha to destroy.

import (
	"image"
	"image/color"
	"image/draw"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/image/vector"
)

// ---------------------------------------------------------------------------
// The DIB surface
// ---------------------------------------------------------------------------

// surface is a top-down 32bpp BGRA DIB section selected into a memory DC: the
// bitmap UpdateLayeredWindow reads, and the bitmap GDI text lands in.
//
// Top-down (a NEGATIVE BiHeight) is not cosmetic. It makes row 0 the top row so
// the byte indexing matches image.RGBA with no vertical flip, and it is a hard
// requirement of DrawThemeTextEx's DTT_COMPOSITED should that alternative ever
// be tried here.
type surface struct {
	dc     win.HDC
	bmp    win.HBITMAP
	oldBmp win.HGDIOBJ
	bits   []byte // BGRA, premultiplied, stride = w*4, row 0 = top
	w, h   int
}

func newSurface(w, h int) *surface {
	if w <= 0 || h <= 0 {
		return nil
	}
	dc := win.CreateCompatibleDC(0)
	if dc == 0 {
		return nil
	}
	bmi := &win.BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
		BiWidth:       int32(w),
		BiHeight:      -int32(h), // negative => top-down
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: win.BI_RGB,
	}
	// CreateDIBSection's real parameter is a BITMAPINFO. lxn/win types it as
	// *BITMAPINFOHEADER, which is safe here and only here: at 32bpp with
	// BI_RGB no colour table follows the header and none is read.
	var ptr unsafe.Pointer
	bmp := win.CreateDIBSection(dc, bmi, win.DIB_RGB_COLORS, &ptr, 0, 0)
	if bmp == 0 || ptr == nil {
		win.DeleteDC(dc)
		return nil
	}
	s := &surface{
		dc: dc,
		// The stride is w*4 with no padding: DIB rows are DWORD aligned and at
		// 32bpp every row is already a whole number of DWORDs.
		bits: unsafe.Slice((*byte)(ptr), w*h*4),
		bmp:  bmp,
		w:    w,
		h:    h,
	}
	s.oldBmp = win.SelectObject(dc, win.HGDIOBJ(bmp))
	return s
}

func (s *surface) dispose() {
	if s == nil || s.dc == 0 {
		return
	}
	if s.oldBmp != 0 {
		win.SelectObject(s.dc, s.oldBmp)
	}
	win.DeleteObject(win.HGDIOBJ(s.bmp))
	win.DeleteDC(s.dc)
	s.bits = nil
	s.dc = 0
	s.bmp = 0
}

func (s *surface) clear() {
	for i := range s.bits {
		s.bits[i] = 0
	}
}

// blitFrom copies a premultiplied image.RGBA into the DIB.
//
// image.RGBA is ALREADY premultiplied, which is exactly the format
// UpdateLayeredWindow documents as mandatory for AC_SRC_ALPHA. The only
// difference between the two buffers is channel order: GDI's 32bpp BI_RGB
// layout is B, G, R, A in memory.
func (s *surface) blitFrom(src *image.RGBA) {
	for y := 0; y < s.h; y++ {
		so := src.PixOffset(0, y)
		do := y * s.w * 4
		for x := 0; x < s.w; x++ {
			si := so + x*4
			di := do + x*4
			s.bits[di+0] = src.Pix[si+2] // B
			s.bits[di+1] = src.Pix[si+1] // G
			s.bits[di+2] = src.Pix[si+0] // R
			s.bits[di+3] = src.Pix[si+3] // A
		}
	}
}

// blitTo copies the whole surface onto a device context at its origin, which is
// how the composed image reaches an ORDINARY window - the forecast panel's system
// look, where there is no WS_EX_LAYERED and therefore no UpdateLayeredWindow to
// hand it to. The destination is the window's client area, obtained from
// BeginPaint inside WM_PAINT.
//
// A plain SRCCOPY needs no vertical flip and no attention to alpha, and both facts
// come from how newSurface built the DIB. Top-down, so row 0 is the top row in the
// destination as well as in the source. BI_RGB at 32bpp declares no alpha channel,
// so GDI reads the B, G and R bytes and ignores the fourth - which is exactly
// right here, because the sheet under an ordinary window is opaque and the alpha
// the composition wrote is 255 everywhere it matters and meaningless everywhere
// else.
//
// This does not breach the rule at the top of this file. GDI is READING the
// surface, not drawing into it; nothing here can touch a byte of it.
func (s *surface) blitTo(dc win.HDC) bool {
	if s == nil || s.dc == 0 || dc == 0 {
		return false
	}
	return win.BitBlt(dc, 0, 0, int32(s.w), int32(s.h), s.dc, 0, 0, win.SRCCOPY)
}

// ---------------------------------------------------------------------------
// Shapes
// ---------------------------------------------------------------------------

// kappa is the control-point distance that makes a cubic Bezier approximate a
// quarter circle to within about 0.02% of the radius. A single QUADRATIC with
// the corner as its control point is the obvious shortcut and is wrong by 6% of
// the radius, which at r=14 is a visible 0.85px of extra squareness.
const kappa = 0.5522847498307936

// roundRectPath appends a rounded rectangle to z, clockwise from the start of
// the top edge.
func roundRectPath(z *vector.Rasterizer, x, y, w, h, r float32) {
	if r < 0 {
		r = 0
	}
	if r*2 > w {
		r = w / 2
	}
	if r*2 > h {
		r = h / 2
	}
	c := r * kappa

	z.MoveTo(x+r, y)
	z.LineTo(x+w-r, y)
	z.CubeTo(x+w-r+c, y, x+w, y+r-c, x+w, y+r)
	z.LineTo(x+w, y+h-r)
	z.CubeTo(x+w, y+h-r+c, x+w-r+c, y+h, x+w-r, y+h)
	z.LineTo(x+r, y+h)
	z.CubeTo(x+r-c, y+h, x, y+h-r+c, x, y+h-r)
	z.LineTo(x, y+r)
	z.CubeTo(x, y+r-c, x+r-c, y, x+r, y)
	z.ClosePath()
}

// roundRect fills an antialiased rounded rectangle into a premultiplied buffer.
//
// This is the whole answer to "how does the panel get rounded corners".
// SetWindowRgn + CreateRoundRectRgn is a 1-bit clip: staircase corners that
// fight the per-pixel alpha the surface already carries, and the system takes
// ownership of the region handle. DWMWA_WINDOW_CORNER_PREFERENCE rounds the
// DWM-drawn window FRAME, and a layered WS_POPUP has no frame. GDI's RoundRect
// compiles and leaves the alpha byte at whatever the brush wrote, which is zero
// - a completely transparent card.
func roundRect(dst *image.RGBA, x, y, w, h int32, r float32, c color.RGBA) {
	if w <= 0 || h <= 0 || c.A == 0 || !fitsIn(dst, x, y, w, h) {
		return
	}
	z := vector.NewRasterizer(int(w), int(h))
	z.DrawOp = draw.Over
	roundRectPath(z, 0, 0, float32(w), float32(h), r)
	z.Draw(dst, image.Rect(int(x), int(y), int(x+w), int(y+h)), image.NewUniform(c), image.Point{})
}

// paintRect fills an axis-aligned rectangle: the table's rule and its row
// separators, and the page background of the opaque fallback.
//
// It is a straight premultiplied source-over rather than a call to roundRect
// with a zero radius, because the shapes it draws are one pixel tall and
// exactly on the pixel grid. There is nothing for a rasteriser to antialias
// there, a degenerate Bezier at every corner is a needless question to ask of
// one, and this loop cannot fail to be exact.
//
// Unlike roundRect it CLIPS rather than declining to draw: a rule is the full
// width of the card and a rounding error of one pixel at the far edge would
// otherwise silently lose the whole line.
func paintRect(dst *image.RGBA, x, y, w, h int32, c color.RGBA) {
	if w <= 0 || h <= 0 || c.A == 0 {
		return
	}
	b := dst.Bounds()
	x0, y0 := max(int(x), b.Min.X), max(int(y), b.Min.Y)
	x1, y1 := min(int(x+w), b.Max.X), min(int(y+h), b.Max.Y)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	// Premultiplied source-over: out = src + dst*(1-srcA). It cannot overflow,
	// because a premultiplied component is never larger than its own alpha:
	// src.C + dst.C*(255-src.A)/255 <= src.A + (255 - src.A) = 255. An opaque
	// colour falls out of the same expression with inv = 0, so it needs no case
	// of its own.
	inv := 255 - uint32(c.A)
	for py := y0; py < y1; py++ {
		row := dst.PixOffset(x0, py)
		for px := x0; px < x1; px++ {
			dst.Pix[row+0] = uint8(uint32(c.R) + uint32(dst.Pix[row+0])*inv/255)
			dst.Pix[row+1] = uint8(uint32(c.G) + uint32(dst.Pix[row+1])*inv/255)
			dst.Pix[row+2] = uint8(uint32(c.B) + uint32(dst.Pix[row+2])*inv/255)
			dst.Pix[row+3] = uint8(uint32(c.A) + uint32(dst.Pix[row+3])*inv/255)
			row += 4
		}
	}
}

// fitsIn reports whether a shape lies wholly inside dst.
//
// The rasteriser maps its own origin to the destination rectangle's top-left
// corner and does no clipping at all, so a shape that hangs off the edge would
// index past the end of dst.Pix and panic - inside a window procedure, which is
// a C callback, so the panic would take the process with it. Every rectangle
// this file is handed is provably inside the panel; this is the belt to that
// braces, and it fails by not drawing rather than by crashing.
func fitsIn(dst *image.RGBA, x, y, w, h int32) bool {
	b := dst.Bounds()
	return int(x) >= b.Min.X && int(y) >= b.Min.Y &&
		int(x+w) <= b.Max.X && int(y+h) <= b.Max.Y
}

// premul converts a straight (r,g,b,a) design colour into the premultiplied
// form both image.RGBA and the DIB store.
func premul(r, g, b, a uint8) color.RGBA {
	return color.RGBA{
		R: uint8(uint32(r) * uint32(a) / 255),
		G: uint8(uint32(g) * uint32(a) / 255),
		B: uint8(uint32(b) * uint32(a) / 255),
		A: a,
	}
}

// ---------------------------------------------------------------------------
// Text
// ---------------------------------------------------------------------------

// textRun is one string to be drawn in one font, in one colour group.
type textRun struct {
	text  string
	rect  win.RECT
	flags uint32
	font  win.HFONT
}

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
// LfQuality is ANTIALIASED_QUALITY rather than the CLEARTYPE_QUALITY the rest of
// this package asks for. Subpixel antialiasing cannot be composited against an
// unknown background, so ClearType is unavailable on a layered window under every
// technique; asking for grayscale explicitly is what keeps the coverage
// arithmetic in compositeMask exact.
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
		LfQuality:        win.ANTIALIASED_QUALITY,
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

// measureDC is a memory DC used only to ask GDI about font metrics. Text
// extents come from the selected font and the mapping mode, never from the
// bitmap, so the default 1x1 monochrome bitmap a fresh memory DC carries is
// enough - and it has to be, because the real surface cannot be sized until
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

// drawTextGroup renders every run in one colour group and composites the result
// into dst at (r,g,b), fully opaque.
//
// Full opacity is deliberate and is what the GTK panel produces: an opaque
// label over a card whose own background is 82% alpha, so the glyphs read
// crisply against whatever is behind the window while the card stays
// translucent.
//
// The mask trick: white text on a black surface with BkMode TRANSPARENT leaves
// each channel byte at exactly coverage*255, because dst = 255*cov +
// 0*(1-cov). That is a perfect 8-bit alpha mask, and no GDI call ever went
// anywhere near the panel's own alpha channel.
func drawTextGroup(dst *image.RGBA, mask *surface, runs []textRun, r, g, b uint8) {
	if len(runs) == 0 || mask == nil {
		return
	}
	mask.clear()
	win.SetBkMode(mask.dc, win.TRANSPARENT)
	win.SetTextColor(mask.dc, win.RGB(255, 255, 255))

	var restore win.HGDIOBJ
	for i := range runs {
		run := &runs[i]
		if run.rect.Right <= run.rect.Left || run.text == "" {
			continue
		}
		if run.font != 0 {
			prev := win.SelectObject(mask.dc, win.HGDIOBJ(run.font))
			if restore == 0 {
				restore = prev
			}
		}
		u := utf16Of(run.text)
		// DrawTextEx's rect is [in,out]; pass a copy so the caller's geometry
		// survives.
		rc := run.rect
		win.DrawTextEx(mask.dc, &u[0], -1, &rc, run.flags, nil)
	}
	if restore != 0 {
		// Leave no font selected: DeleteObject refuses to free a GDI object
		// that is still selected into a DC, and the font would leak.
		win.SelectObject(mask.dc, restore)
	}

	// GDI batches its drawing calls. Without this the bytes read below can
	// predate the DrawTextEx calls that were supposed to produce them.
	win.GdiFlush()

	compositeMask(dst, mask, r, g, b)
}

// compositeMask does premultiplied source-over of a solid colour through an
// 8-bit coverage mask:
//
//	out.C = src.C*cov/255 + dst.C*(1 - cov/255)
//	out.A = cov           + dst.A*(1 - cov/255)
//
// The coverage is the mean of the three channels. Under grayscale antialiasing
// they are equal; if Windows overrides the requested quality back to ClearType
// they are the three subpixel coverages, and the mean is a reasonable
// grayscale reduction of them.
func compositeMask(dst *image.RGBA, mask *surface, sr, sg, sb uint8) {
	w, h := mask.w, mask.h
	if w > dst.Bounds().Dx() {
		w = dst.Bounds().Dx()
	}
	if h > dst.Bounds().Dy() {
		h = dst.Bounds().Dy()
	}
	for y := 0; y < h; y++ {
		mo := y * mask.w * 4
		do := dst.PixOffset(0, y)
		for x := 0; x < w; x++ {
			mi := mo + x*4
			cov := (uint32(mask.bits[mi+0]) + uint32(mask.bits[mi+1]) + uint32(mask.bits[mi+2])) / 3
			if cov == 0 {
				continue
			}
			inv := 255 - cov
			p := do + x*4
			dst.Pix[p+0] = uint8((uint32(sr)*cov + uint32(dst.Pix[p+0])*inv) / 255)
			dst.Pix[p+1] = uint8((uint32(sg)*cov + uint32(dst.Pix[p+1])*inv) / 255)
			dst.Pix[p+2] = uint8((uint32(sb)*cov + uint32(dst.Pix[p+2])*inv) / 255)
			dst.Pix[p+3] = uint8(cov + uint32(dst.Pix[p+3])*inv/255)
		}
	}
}
