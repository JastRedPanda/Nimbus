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

	"github.com/JastRedPanda/Nimbus/internal/wicons"
	"github.com/lxn/win"
	xdraw "golang.org/x/image/draw"
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

// ---------------------------------------------------------------------------
// Shapes
// ---------------------------------------------------------------------------

// kappa is the control-point distance that makes a cubic Bezier approximate a
// quarter circle to within about 0.02% of the radius. A single QUADRATIC with
// the corner as its control point is the obvious shortcut and is wrong by 6% of
// the radius, which at r=14 is a visible 0.85px of extra squareness.
const kappa = 0.5522847498307936

// roundRectPath appends a rounded rectangle to z. reverse emits the same
// outline with the opposite winding, which the rasteriser's non-zero fill rule
// turns into a hole - that is how roundRing gets an antialiased 1px annulus
// instead of two stacked fills.
func roundRectPath(z *vector.Rasterizer, x, y, w, h, r float32, reverse bool) {
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

	if !reverse {
		z.MoveTo(x+r, y)
		z.LineTo(x+w-r, y)
		z.CubeTo(x+w-r+c, y, x+w, y+r-c, x+w, y+r)
		z.LineTo(x+w, y+h-r)
		z.CubeTo(x+w, y+h-r+c, x+w-r+c, y+h, x+w-r, y+h)
		z.LineTo(x+r, y+h)
		z.CubeTo(x+r-c, y+h, x, y+h-r+c, x, y+h-r)
		z.LineTo(x, y+r)
		z.CubeTo(x, y+r-c, x+r-c, y, x+r, y)
	} else {
		z.MoveTo(x+r, y)
		z.CubeTo(x+r-c, y, x, y+r-c, x, y+r)
		z.LineTo(x, y+h-r)
		z.CubeTo(x, y+h-r+c, x+r-c, y+h, x+r, y+h)
		z.LineTo(x+w-r, y+h)
		z.CubeTo(x+w-r+c, y+h, x+w, y+h-r+c, x+w, y+h-r)
		z.LineTo(x+w, y+r)
		z.CubeTo(x+w, y+r-c, x+w-r+c, y, x+w-r, y)
		z.LineTo(x+r, y)
	}
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
	roundRectPath(z, 0, 0, float32(w), float32(h), r, false)
	z.Draw(dst, image.Rect(int(x), int(y), int(x+w), int(y+h)), image.NewUniform(c), image.Point{})
}

// roundRing strokes a 1px (times thickness) rounded outline.
//
// It is a real annulus rather than "fill the outer shape, then fill the inner
// one on top", because the card fill is translucent: painting an 88%-alpha fill
// over an opaque border tints the whole interior with the border colour and
// turns the card opaque. Drawing the ring last, on top of the finished fill, is
// the only order that leaves the interior exactly the colour and opacity the
// design asks for.
func roundRing(dst *image.RGBA, x, y, w, h int32, r, thickness float32, c color.RGBA) {
	if w <= 0 || h <= 0 || thickness <= 0 || c.A == 0 || !fitsIn(dst, x, y, w, h) {
		return
	}
	fw, fh := float32(w), float32(h)
	if thickness*2 >= fw || thickness*2 >= fh {
		roundRect(dst, x, y, w, h, r, c)
		return
	}
	z := vector.NewRasterizer(int(w), int(h))
	z.DrawOp = draw.Over
	roundRectPath(z, 0, 0, fw, fh, r, false)
	roundRectPath(z, thickness, thickness, fw-2*thickness, fh-2*thickness, r-thickness, true)
	z.Draw(dst, image.Rect(int(x), int(y), int(x+w), int(y+h)), image.NewUniform(c), image.Point{})
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
// Colour artwork
// ---------------------------------------------------------------------------

// panelIcon renders the Meteocons artwork for a WMO code at exactly px pixels,
// premultiplied and ready to composite. It returns nil when there is no
// artwork, and every caller treats that as "leave the slot empty" rather than
// substituting a wrong symbol - the same contract iconWidget has on the GTK
// side.
//
// internal/wicons ships three rasterised sizes and returns nil for anything
// else, on the grounds that a scaled bitmap is exactly the softened artwork it
// exists to avoid. At 96 DPI the panel asks for 64 and 32 and gets them
// untouched. A scaled desktop asks for sizes in between, and then the least-bad
// answer is to take the next size UP and shrink it: downsampling a 128px raster
// to 96 keeps the thin features, upsampling a 64px one to 96 does not.
//
// BiLinear rather than CatmullRom on purpose: it is a kernel scaler, so it
// area-averages properly when shrinking, and unlike CatmullRom it has no
// negative lobes, so it cannot overshoot and produce a premultiplied component
// larger than its own alpha.
func panelIcon(code int, px int32) *image.RGBA {
	if px <= 0 {
		return nil
	}
	var size wicons.Size
	switch {
	case px <= int32(wicons.Size32):
		size = wicons.Size32
	case px <= int32(wicons.Size64):
		size = wicons.Size64
	default:
		size = wicons.Size128
	}
	src := wicons.Icon(code, size)
	if src == nil {
		return nil
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, int(px), int(px)))
	if b.Dx() == int(px) && b.Dy() == int(px) {
		// wicons returns STRAIGHT alpha; image.RGBA is premultiplied.
		// draw.Draw does that conversion on the way through, because
		// color.NRGBA.RGBA() premultiplies. Hand-rolling it is how every
		// antialiased edge acquires a dark halo.
		draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
		return dst
	}
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Src, nil)
	return dst
}

// drawImage composites premultiplied artwork over the panel buffer.
func drawImage(dst *image.RGBA, src *image.RGBA, x, y int32) {
	if src == nil {
		return
	}
	b := src.Bounds()
	draw.Draw(dst, image.Rect(int(x), int(y), int(x)+b.Dx(), int(y)+b.Dy()), src, b.Min, draw.Over)
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

// panelFont builds a font at a point size scaled for the window's DPI.
//
// LfHeight is NEGATIVE: negative is the character height, positive is the cell
// height including internal leading, and mixing them up makes every font come
// out about 20% too small.
//
// LfQuality is ANTIALIASED_QUALITY rather than the CLEARTYPE_QUALITY the rest
// of this package asks for. Subpixel antialiasing cannot be composited against
// an unknown background, so ClearType is unavailable on a layered window under
// every technique; asking for grayscale explicitly is what keeps the coverage
// arithmetic below exact.
//
// The face comes from the user's own shell font, not a hardcoded "Segoe UI", so
// a locale whose script that face cannot draw still gets readable text.
func panelFont(pt, weight, dpi int32) win.HFONT {
	lf := win.LOGFONT{
		LfHeight:         -win.MulDiv(pt, dpi, 72),
		LfWeight:         weight,
		LfCharSet:        win.DEFAULT_CHARSET,
		LfOutPrecision:   win.OUT_DEFAULT_PRECIS,
		LfClipPrecision:  win.CLIP_DEFAULT_PRECIS,
		LfQuality:        win.ANTIALIASED_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	setFaceName(&lf, shellFontFace())
	return win.CreateFontIndirect(&lf)
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
