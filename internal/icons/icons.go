package icons

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

var white = color.RGBA{R: 255, G: 255, B: 255, A: 255}

func bgCol(temp float64) color.RGBA {
	switch {
	case temp < -10:
		return color.RGBA{R: 30, G: 60, B: 150, A: 220}
	case temp < 0:
		return color.RGBA{R: 40, G: 80, B: 180, A: 220}
	case temp < 10:
		return color.RGBA{R: 30, G: 90, B: 190, A: 220}
	case temp < 20:
		return color.RGBA{R: 40, G: 140, B: 40, A: 220}
	case temp < 30:
		return color.RGBA{R: 190, G: 130, B: 20, A: 220}
	default:
		return color.RGBA{R: 180, G: 40, B: 30, A: 220}
	}
}

func Generate(temp float64, code int, theme string) []byte {
	return MakeIcon(temp, code, theme)
}

func MakeIcon(temp float64, code int, theme string) []byte {
	_ = theme
	s32 := render(temp, code, 32)
	s64 := render(temp, code, 64)
	return encodeICO(s32, s64)
}

func render(temp float64, code int, sz int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sz, sz))
	bg := bgCol(temp)

	// filled background with rounded corners (approximate via fill on all pixels)
	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			// simple rounded mask
			r := sz / 8
			tl := (x-r)*(x-r)+(y-r)*(y-r) > r*r && x < r && y < r
			tr := (x-(sz-1-r))*(x-(sz-1-r))+(y-r)*(y-r) > r*r && x >= sz-r && y < r
			bl := (x-r)*(x-r)+(y-(sz-1-r))*(y-(sz-1-r)) > r*r && x < r && y >= sz-r
			br := (x-(sz-1-r))*(x-(sz-1-r))+(y-(sz-1-r))*(y-(sz-1-r)) > r*r && x >= sz-r && y >= sz-r
			if !tl && !tr && !bl && !br {
				img.Set(x, y, bg)
			}
		}
	}

	if sz >= 64 {
		drawWeatherLarge(img, code, sz)
		drawTempLarge(img, temp, sz)
	} else {
		drawWeatherSmall(img, code, sz)
		drawTempSmall(img, temp, sz)
	}

	return img
}

func drawWeatherLarge(img *image.RGBA, code int, sz int) {
	half := sz / 2
	switch {
	case code == 0:
		drawClear(img, half, 16)
	case code <= 3:
		drawCloudy(img, half, 16)
	case code >= 45 && code <= 48:
		drawFog(img, half, 16)
	case code >= 51 && code <= 57:
		drawRain(img, half, 16, false)
	case code >= 61 && code <= 65:
		drawRain(img, half, 16, true)
	case code >= 71 && code <= 77:
		drawSnow(img, half, 16)
	case code >= 80 && code <= 86:
		drawRain(img, half, 16, true)
	case code >= 95:
		drawStorm(img, half, 16)
	default:
		drawCloudy(img, half, 16)
	}
}

func drawWeatherSmall(img *image.RGBA, code int, sz int) {
	half := sz / 2
	switch {
	case code == 0:
		drawClearS(img, half, 12)
	case code <= 3:
		drawCloudyS(img, half, 12)
	case code >= 45 && code <= 48:
		drawFogS(img, half, 12)
	case code >= 51 && code <= 57:
		drawRainS(img, half, 12, false)
	case code >= 61 && code <= 65:
		drawRainS(img, half, 12, true)
	case code >= 71 && code <= 77:
		drawSnowS(img, half, 12)
	case code >= 80 && code <= 86:
		drawRainS(img, half, 12, true)
	case code >= 95:
		drawStormS(img, half, 12)
	default:
		drawCloudyS(img, half, 12)
	}
}

func drawTempLarge(img *image.RGBA, temp float64, sz int) {
	t := int(math.Round(temp))
	s := fmt.Sprintf("%d°C", t)
	if t > 0 {
		s = "+" + s
	}
	drawText(img, s, 13, sz-8, white)
}

func drawTempSmall(img *image.RGBA, temp float64, sz int) {
	t := int(math.Round(temp))
	s := fmt.Sprintf("%d°", t)
	if t > 0 {
		s = "+" + s
	}
	drawText(img, s, 3, sz-3, white)
}

func drawText(img *image.RGBA, text string, x, y int, col color.RGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(text)
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
		img.Set(x, y, c)
	}
}

func fillRect(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			set(img, x, y, c)
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				set(img, cx+dx, cy+dy, c)
			}
		}
	}
}

// Large weather symbols (for 64px)
func drawClear(img *image.RGBA, cx, cy int) {
	fillCircle(img, cx, cy, 8, white)
	for _, d := range [][2]int{{0, -13}, {0, 13}, {-13, 0}, {13, 0}, {-9, -9}, {9, 9}, {-9, 9}, {9, -9}} {
		fillRect(img, cx+d[0]-2, cy+d[1]-2, cx+d[0]+2, cy+d[1]+2, white)
	}
}

func drawCloudy(img *image.RGBA, cx, cy int) {
	fillCircle(img, cx-6, cy, 7, white)
	fillCircle(img, cx+6, cy, 7, white)
	fillCircle(img, cx-1, cy-4, 6, white)
	fillRect(img, cx-12, cy, cx+12, cy+5, white)
}

func drawFog(img *image.RGBA, cx, cy int) {
	for y := -6; y <= 6; y += 3 {
		fillRect(img, cx-13, cy+y, cx+13, cy+y+2, white)
	}
}

func drawRain(img *image.RGBA, cx, cy int, heavy bool) {
	drawCloudy(img, cx, cy-1)
	n := 3
	if heavy {
		n = 5
	}
	for i := 0; i < n; i++ {
		x := cx - 8 + i*4
		for dy := 0; dy < 2; dy++ {
			fillRect(img, x, cy+7+dy*3, x+2, cy+9+dy*3, white)
		}
	}
}

func drawSnow(img *image.RGBA, cx, cy int) {
	drawCloudy(img, cx, cy-1)
	for i := -2; i <= 2; i++ {
		x := cx + i*5
		set(img, x, cy+7, white)
		set(img, x+1, cy+7, white)
		set(img, x, cy+8, white)
		set(img, x-1, cy+8, white)
		set(img, x, cy+9, white)
		set(img, x+1, cy+9, white)
	}
}

func drawStorm(img *image.RGBA, cx, cy int) {
	drawCloudy(img, cx, cy-1)
	pts := [][2]int{{-5, 7}, {-5, 8}, {-4, 7}, {-3, 8}, {-3, 9}, {-2, 8}, {-1, 9}, {-1, 10}, {0, 9}}
	for _, p := range pts {
		set(img, cx+p[0], cy+p[1], white)
	}
}

// Small weather symbols (for 32px) - simpler, bolder
func drawClearS(img *image.RGBA, cx, cy int) {
	fillCircle(img, cx, cy, 4, white)
	for _, d := range [][2]int{{0, -7}, {0, 7}, {-7, 0}, {7, 0}} {
		fillRect(img, cx+d[0]-1, cy+d[1]-1, cx+d[0]+1, cy+d[1]+1, white)
	}
}

func drawCloudyS(img *image.RGBA, cx, cy int) {
	fillCircle(img, cx-3, cy, 4, white)
	fillCircle(img, cx+4, cy, 4, white)
	fillCircle(img, cx, cy-2, 3, white)
	fillRect(img, cx-7, cy, cx+8, cy+3, white)
}

func drawFogS(img *image.RGBA, cx, cy int) {
	for y := -4; y <= 4; y += 2 {
		fillRect(img, cx-8, cy+y, cx+8, cy+y+1, white)
	}
}

func drawRainS(img *image.RGBA, cx, cy int, heavy bool) {
	drawCloudyS(img, cx, cy-1)
	n := 3
	if heavy {
		n = 4
	}
	for i := 0; i < n; i++ {
		x := cx - 4 + i*3
		fillRect(img, x, cy+4, x+1, cy+7, white)
	}
}

func drawSnowS(img *image.RGBA, cx, cy int) {
	drawCloudyS(img, cx, cy-1)
	for i := -1; i <= 1; i++ {
		x := cx + i*3
		set(img, x, cy+4, white)
		set(img, x+1, cy+4, white)
		set(img, x, cy+5, white)
		set(img, x-1, cy+5, white)
	}
}

func drawStormS(img *image.RGBA, cx, cy int) {
	drawCloudyS(img, cx, cy-1)
	pts := [][2]int{{-3, 4}, {-3, 5}, {-2, 4}, {-1, 5}, {-1, 6}, {0, 5}}
	for _, p := range pts {
		set(img, cx+p[0], cy+p[1], white)
	}
}

func encodeICO(imgs ...*image.RGBA) []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, uint16(0))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(len(imgs)))

	var off = uint32(6 + len(imgs)*16)
	var allPNG [][]byte

	for _, img := range imgs {
		var pngBuf bytes.Buffer
		png.Encode(&pngBuf, img)
		data := pngBuf.Bytes()
		allPNG = append(allPNG, data)

		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		iw := uint8(w)
		if iw > 255 {
			iw = 0
		}
		ih := uint8(h)
		if ih > 255 {
			ih = 0
		}
		binary.Write(buf, binary.LittleEndian, iw)
		binary.Write(buf, binary.LittleEndian, ih)
		binary.Write(buf, binary.LittleEndian, uint8(0))
		binary.Write(buf, binary.LittleEndian, uint8(0))
		binary.Write(buf, binary.LittleEndian, uint16(1))
		binary.Write(buf, binary.LittleEndian, uint16(32))
		binary.Write(buf, binary.LittleEndian, uint32(len(data)))
		binary.Write(buf, binary.LittleEndian, off)
		off += uint32(len(data))
	}

	for _, data := range allPNG {
		buf.Write(data)
	}
	return buf.Bytes()
}
