// Package fonts owns the embedded Weather Icons typeface and turns its
// codepoints into weather symbols.
//
// There are two ways to get a symbol on screen. Windows registers the face with
// the OS (see register_windows.go) and draws it with GDI. Everywhere else the
// glyph is rasterised here, in Go, and handed to the toolkit as pixels - which
// needs no temp file, no system font registration, and cannot be hijacked by
// another font that happens to claim the same private-use codepoint.
package fonts

import (
	_ "embed"
	"image"
	"image/color"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

//go:embed weathericons.ttf
var weatherIconsTTF []byte

// TTF is the typeface itself, for a toolkit that would rather register a font
// from memory than be handed rasterised pixels. Qt is one: QFontDatabase takes
// the bytes and then draws the glyph at whatever size and colour a QLabel asks
// for, which is a better deal than a bitmap per cell.
//
// It returns the package's own slice rather than a copy. Nothing writes to it,
// and the one caller hands it straight to a C function that reads it once.
func TTF() []byte { return weatherIconsTTF }

const (
	WiDaySunny     = "\uf00d"
	WiDayCloudy    = "\uf002"
	WiCloud        = "\uf041"
	WiFog          = "\uf014"
	WiRain         = "\uf019"
	WiShowers      = "\uf01a"
	WiSnow         = "\uf01b"
	WiThunderstorm = "\uf01e"
)

func IconForCode(code int) string {
	switch {
	case code == 0:
		return WiDaySunny
	case code <= 2:
		return WiDayCloudy
	case code <= 3:
		return WiCloud
	case code >= 45 && code <= 48:
		return WiFog
	case code >= 51 && code <= 57:
		return WiRain
	case code >= 61 && code <= 65:
		return WiShowers
	case code >= 71 && code <= 77:
		return WiSnow
	case code >= 80 && code <= 86:
		return WiShowers
	case code >= 95:
		return WiThunderstorm
	default:
		return WiCloud
	}
}

// RuneForCode is IconForCode as a rune, which is what Glyph wants.
func RuneForCode(code int) rune { return []rune(IconForCode(code))[0] }

var (
	parseOnce sync.Once
	parsed    *sfnt.Font
	parseErr  error

	glyphMu    sync.Mutex
	glyphCache = map[glyphKey]*image.NRGBA{}
)

type glyphKey struct {
	r   rune
	px  int
	col color.NRGBA
}

// Glyph rasterises one Weather Icons codepoint into a square tile roughly px on
// a side, with a transparent background, antialiased, in col. The pixels are
// straight alpha, which is what gdk-pixbuf expects; premultiplied pixels would
// darken every antialiased edge.
//
// px is the size of the drawn symbol, not the em size. These glyphs overhang
// their em box by different amounts - about 1.1x for most and 1.4x for
// day-cloudy - so rasterising them all at one em produces symbols of visibly
// different sizes in the same column. The em is therefore measured per glyph
// and scaled so the ink lands at px.
//
// Results are cached because a forecast table asks for at most eight distinct
// glyphs and the same one repeats across days.
func Glyph(r rune, px int, col color.NRGBA) *image.NRGBA {
	key := glyphKey{r, px, col}
	glyphMu.Lock()
	defer glyphMu.Unlock()
	if img, ok := glyphCache[key]; ok {
		return img
	}

	parseOnce.Do(func() { parsed, parseErr = sfnt.Parse(weatherIconsTTF) })
	if parseErr != nil {
		return nil
	}

	// Ink scales linearly with the em, so one measuring pass gives the em that
	// puts this particular glyph's longest side at px.
	w0, h0, ok := inkSize(r, px)
	if !ok {
		return nil
	}
	em := px * px / max(w0, h0)
	if em < 1 {
		em = 1
	}

	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: float64(em), DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	defer face.Close()

	b, _, ok := face.GlyphBounds(r)
	if !ok {
		return nil
	}
	w := (b.Max.X - b.Min.X).Ceil()
	h := (b.Max.Y - b.Min.Y).Ceil()
	if w <= 0 || h <= 0 {
		return nil
	}
	// Hinting can round the ink a pixel past the target; grow the tile rather
	// than clip the symbol.
	side := max(px, max(w, h))

	dst := image.NewNRGBA(image.Rect(0, 0, side, side))
	(&font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(col),
		Face: face,
		Dot: fixed.Point26_6{
			X: fixed.I((side-w)/2) - b.Min.X,
			Y: fixed.I((side-h)/2) - b.Min.Y,
		},
	}).DrawString(string(r))

	glyphCache[key] = dst
	return dst
}

// inkSize measures a glyph's ink box at the given em size.
func inkSize(r rune, em int) (w, h int, ok bool) {
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size: float64(em), DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		return 0, 0, false
	}
	defer face.Close()

	b, _, ok := face.GlyphBounds(r)
	if !ok {
		return 0, 0, false
	}
	w = (b.Max.X - b.Min.X).Ceil()
	h = (b.Max.Y - b.Min.Y).Ceil()
	return w, h, w > 0 && h > 0
}
