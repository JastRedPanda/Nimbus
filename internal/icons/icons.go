package icons

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
)

const S = 64

func iconCol(theme string, temp float64) color.RGBA {
	switch theme {
	case "dark":
		return color.RGBA{R: 200, G: 210, B: 230, A: 255}
	case "light":
		return color.RGBA{R: 20, G: 25, B: 40, A: 255}
	default:
		return tempCol(temp)
	}
}

func tempCol(temp float64) color.RGBA {
	switch {
	case temp < -10:
		return color.RGBA{R: 140, G: 170, B: 255, A: 255}
	case temp < 0:
		return color.RGBA{R: 80, G: 130, B: 255, A: 255}
	case temp < 10:
		return color.RGBA{R: 50, G: 110, B: 240, A: 255}
	case temp < 20:
		return color.RGBA{R: 60, G: 190, B: 60, A: 255}
	case temp < 30:
		return color.RGBA{R: 230, G: 170, B: 30, A: 255}
	default:
		return color.RGBA{R: 210, G: 50, B: 40, A: 255}
	}
}

func Generate(temp float64, code int, theme string) []byte {
	return MakeIcon(temp, code, theme)
}

func generateAt(temp float64, code int, theme string, sz int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sz, sz))
	col := iconCol(theme, temp)
	half := sz / 2

	// Scale weather symbol position by size
	s := float64(sz) / 64.0

	switch {
	case code == 0:
		drawClear(img, half, int(float64(24)*s), col, s)
	case code <= 3:
		drawCloudy(img, half, int(float64(24)*s), col, s)
	case code >= 45 && code <= 48:
		drawFog(img, half, int(float64(24)*s), col, s)
	case code >= 51 && code <= 57:
		drawRain(img, half, int(float64(24)*s), col, false, s)
	case code >= 61 && code <= 65:
		drawRain(img, half, int(float64(24)*s), col, true, s)
	case code >= 71 && code <= 77:
		drawSnow(img, half, int(float64(24)*s), col, s)
	case code >= 80 && code <= 86:
		drawRain(img, half, int(float64(24)*s), col, true, s)
	case code >= 95:
		drawStorm(img, half, int(float64(24)*s), col, s)
	default:
		drawCloudy(img, half, int(float64(24)*s), col, s)
	}

	drawTemp(img, temp, col, sz)
	return img
}

// 3×5 pixel bitmap digits
var digPix = [10][5]uint8{
	{7, 5, 5, 5, 7}, // 0
	{2, 6, 2, 2, 7}, // 1
	{7, 1, 7, 4, 7}, // 2
	{7, 1, 3, 1, 7}, // 3
	{5, 5, 7, 1, 1}, // 4
	{7, 4, 7, 1, 7}, // 5
	{7, 4, 7, 5, 7}, // 6
	{7, 1, 2, 2, 2}, // 7
	{7, 5, 7, 5, 7}, // 8
	{7, 5, 7, 1, 7}, // 9
}

type pt struct{ x, y int }

func digPts(d int) []pt {
	g := digPix[d]
	var out []pt
	for r := 0; r < 5; r++ {
		v := g[r]
		for c := 0; c < 3; c++ {
			if v&(1<<(2-c)) != 0 {
				out = append(out, pt{c, r})
			}
		}
	}
	return out
}

var ptsPlus = []pt{{1, 0}, {0, 1}, {1, 1}, {2, 1}, {1, 2}}
var ptsMinus = []pt{{0, 1}, {1, 1}, {2, 1}}
var ptsDeg = []pt{{1, 0}, {0, 1}, {1, 1}, {2, 1}, {1, 2}}
var ptsC = []pt{{1, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 4}, {2, 4}}
var ptsF = []pt{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 0}, {2, 0}, {1, 2}, {2, 2}}

func drawTemp(img *image.RGBA, temp float64, col color.RGBA, sz int) {
	t := int(math.Round(temp))
	digits := fmt.Sprintf("%d", t)
	if t > 0 {
		digits = "+" + digits
	}

	n := len(digits)
	totalW := n*4 + 4 + 4 // digits*4 + deg*4 + letter*4
	offX := (sz - totalW) / 2
	if offX < 0 {
		offX = 0
	}
	baseY := sz - 9

	cx := offX
	draw := func(pts []pt) {
		for _, p := range pts {
			set(img, cx+p.x, baseY+p.y, col)
		}
		cx += 4
	}

	for _, r := range digits {
		if r >= '0' && r <= '9' {
			draw(digPts(int(r - '0')))
		} else if r == '+' {
			draw(ptsPlus)
		} else if r == '-' {
			draw(ptsMinus)
		}
	}
	draw(ptsDeg)
	draw(ptsC)
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

func sc(s float64, v int) int {
	return int(float64(v) * s)
}

func drawClear(img *image.RGBA, cx, cy int, c color.RGBA, s float64) {
	fillCircle(img, cx, cy, sc(s, 7), c)
	for _, d := range [][2]int{
		sc2(s, 0, -11), sc2(s, 0, 11), sc2(s, -11, 0), sc2(s, 11, 0),
		sc2(s, -8, -8), sc2(s, 8, 8), sc2(s, -8, 8), sc2(s, 8, -8),
	} {
		r := sc(s, 1)
		fillRect(img, cx+d[0]-r, cy+d[1]-r, cx+d[0]+r, cy+d[1]+r, c)
	}
}

func sc2(s float64, x, y int) [2]int {
	return [2]int{sc(s, x), sc(s, y)}
}

func drawCloudy(img *image.RGBA, cx, cy int, c color.RGBA, s float64) {
	r := sc(s, 6)
	fillCircle(img, cx-sc(s, 5), cy, r, c)
	fillCircle(img, cx+sc(s, 5), cy, r, c)
	fillCircle(img, cx-sc(s, 1), cy-sc(s, 4), sc(s, 5), c)
	fillRect(img, cx-sc(s, 10), cy, cx+sc(s, 10), cy+sc(s, 4), c)
}

func drawFog(img *image.RGBA, cx, cy int, c color.RGBA, s float64) {
	for y := -5; y <= 5; y += 2 {
		fillRect(img, cx-sc(s, 11), cy+sc(s, y), cx+sc(s, 11), cy+sc(s, y+1), c)
	}
}

func drawRain(img *image.RGBA, cx, cy int, c color.RGBA, heavy bool, s float64) {
	drawCloudy(img, cx, cy-sc(s, 1), c, s)
	n := 3
	if heavy {
		n = 5
	}
	for i := 0; i < n; i++ {
		x := cx - sc(s, 6) + sc(s, i*3)
		fillRect(img, x, cy+sc(s, 6), x+sc(s, 1), cy+sc(s, 11), c)
	}
}

func drawSnow(img *image.RGBA, cx, cy int, c color.RGBA, s float64) {
	drawCloudy(img, cx, cy-sc(s, 1), c, s)
	for i := -1; i <= 1; i++ {
		x := cx + sc(s, i*5)
		d := sc(s, 1)
		set(img, x, cy+sc(s, 6), c)
		set(img, x+d, cy+sc(s, 6), c)
		set(img, x, cy+sc(s, 7), c)
		set(img, x-d, cy+sc(s, 7), c)
		set(img, x, cy+sc(s, 8), c)
		set(img, x+d, cy+sc(s, 8), c)
	}
}

func drawStorm(img *image.RGBA, cx, cy int, c color.RGBA, s float64) {
	drawCloudy(img, cx, cy-sc(s, 1), c, s)
	pts := [][2]int{
		{-4, 6}, {-4, 7}, {-3, 6}, {-2, 7}, {-2, 8},
		{-1, 7}, {0, 8}, {0, 9}, {1, 8},
	}
	for _, p := range pts {
		set(img, cx+sc(s, p[0]), cy+sc(s, p[1]), c)
	}
}

func encodeICO(pngSizes ...*image.RGBA) []byte {
	buf := &bytes.Buffer{}
	binary.Write(buf, binary.LittleEndian, uint16(0))
	binary.Write(buf, binary.LittleEndian, uint16(1)) // ICO
	binary.Write(buf, binary.LittleEndian, uint16(len(pngSizes)))

	var dataOff = uint32(6 + len(pngSizes)*16)
	var allPNG [][]byte

	for _, img := range pngSizes {
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
		binary.Write(buf, binary.LittleEndian, uint8(0))  // palette
		binary.Write(buf, binary.LittleEndian, uint8(0))  // reserved
		binary.Write(buf, binary.LittleEndian, uint16(1)) // planes
		binary.Write(buf, binary.LittleEndian, uint16(32))// bpp
		binary.Write(buf, binary.LittleEndian, uint32(len(data)))
		binary.Write(buf, binary.LittleEndian, dataOff)
		dataOff += uint32(len(data))
	}

	for _, data := range allPNG {
		buf.Write(data)
	}
	return buf.Bytes()
}

func MakeIcon(temp float64, code int, theme string) []byte {
	s32 := generateAt(temp, code, theme, 32)
	s64 := generateAt(temp, code, theme, 64)
	return encodeICO(s32, s64)
}
