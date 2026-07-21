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

func iconColor(theme string, temp float64) color.RGBA {
	switch theme {
	case "dark":
		return color.RGBA{R: 200, G: 210, B: 230, A: 255}
	case "light":
		return color.RGBA{R: 20, G: 25, B: 40, A: 255}
	default:
		return tempColor(temp)
	}
}

func tempColor(temp float64) color.RGBA {
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

func GeneratePNG(temp float64, weatherCode int, theme string) []byte {
	const S = 64
	icon := image.NewRGBA(image.Rect(0, 0, S, S))
	col := iconColor(theme, temp)

	wx, wy := S/2, 22
	switch {
	case weatherCode == 0:
		drawClear(icon, wx, wy, col)
	case weatherCode <= 3:
		drawCloudy(icon, wx, wy, col)
	case weatherCode >= 45 && weatherCode <= 48:
		drawFog(icon, wx, wy, col)
	case weatherCode >= 51 && weatherCode <= 57:
		drawRain(icon, wx, wy, col, false)
	case weatherCode >= 61 && weatherCode <= 65:
		drawRain(icon, wx, wy, col, true)
	case weatherCode >= 71 && weatherCode <= 77:
		drawSnow(icon, wx, wy, col)
	case weatherCode >= 80 && weatherCode <= 86:
		drawRain(icon, wx, wy, col, true)
	case weatherCode >= 95:
		drawStorm(icon, wx, wy, col)
	default:
		drawCloudy(icon, wx, wy, col)
	}

	drawTemp(icon, temp, col, S)

	var buf bytes.Buffer
	png.Encode(&buf, icon)
	return buf.Bytes()
}

func Generate(temp float64, weatherCode int, theme string) []byte {
	pngData := GeneratePNG(temp, weatherCode, theme)
	return encodeICO(pngData, 64)
}

// simple 3×5 pixel bitmap digits 0-9, +, -, °, C, F
// bit 0 = top-left, bit 2 = top-right, bit 3 = next row...
// stored as [row]bitmask
var digitPixels = [10][5]uint8{
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

type dot struct{ x, y int }

func digitDots(d int) []dot {
	g := digitPixels[d]
	var out []dot
	for row := 0; row < 5; row++ {
		v := g[row]
		for col := 0; col < 3; col++ {
			if v&(1<<(2-col)) != 0 {
				out = append(out, dot{col, row})
			}
		}
	}
	return out
}

var plusDots = []dot{{1, 0}, {0, 1}, {1, 1}, {2, 1}, {1, 2}}
var minusDots = []dot{{0, 1}, {1, 1}, {2, 1}}
var degDots = []dot{{1, 0}, {0, 1}, {1, 1}, {2, 1}, {1, 2}}
var letterCDots = []dot{{1, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 4}, {2, 4}}
var letterFDots = []dot{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 0}, {2, 0}, {1, 2}, {2, 2}}

func drawTemp(img *image.RGBA, temp float64, col color.RGBA, S int) {
	t := int(math.Round(temp))
	digits := fmt.Sprintf("%d", t)
	if t > 0 {
		digits = "+" + digits
	}

	// Calculate total width: each char = 4px (3px glyph + 1px space)
	// chars: digits/plus + degree + letter
	n := len(digits)
	totalW := n*4 + 4 + 4 // digits*4 + deg*4 + letter*4
	offX := (S - totalW) / 2
	if offX < 0 {
		offX = 0
	}
	baseY := S - 9

	cx := offX
	draw := func(ds []dot) {
		for _, d := range ds {
			px(img, cx+d.x, baseY+d.y, col)
		}
		cx += 4
	}

	for _, r := range digits {
		if r >= '0' && r <= '9' {
			draw(digitDots(int(r - '0')))
		} else if r == '+' {
			draw(plusDots)
		} else if r == '-' {
			draw(minusDots)
		}
	}

	draw(degDots)
	draw(letterCDots)
}

func px(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
		img.Set(x, y, c)
	}
}

func fillRect(img *image.RGBA, x1, y1, x2, y2 int, c color.RGBA) {
	for y := y1; y <= y2; y++ {
		for x := x1; x <= x2; x++ {
			px(img, x, y, c)
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px(img, cx+dx, cy+dy, c)
			}
		}
	}
}

func drawClear(img *image.RGBA, cx, cy int, c color.RGBA) {
	fillCircle(img, cx, cy, 7, c)
	for _, d := range [][2]int{
		{0, -11}, {0, 11}, {-11, 0}, {11, 0},
		{-8, -8}, {8, 8}, {-8, 8}, {8, -8},
	} {
		fillRect(img, cx+d[0]-1, cy+d[1]-1, cx+d[0]+1, cy+d[1]+1, c)
	}
}

func drawCloudy(img *image.RGBA, cx, cy int, c color.RGBA) {
	fillCircle(img, cx-5, cy, 6, c)
	fillCircle(img, cx+5, cy, 6, c)
	fillCircle(img, cx-1, cy-4, 5, c)
	fillRect(img, cx-10, cy, cx+10, cy+4, c)
}

func drawFog(img *image.RGBA, cx, cy int, c color.RGBA) {
	for y := -5; y <= 5; y += 2 {
		fillRect(img, cx-11, cy+y, cx+11, cy+y+1, c)
	}
}

func drawRain(img *image.RGBA, cx, cy int, c color.RGBA, heavy bool) {
	drawCloudy(img, cx, cy-1, c)
	n := 3
	if heavy {
		n = 5
	}
	for i := 0; i < n; i++ {
		x := cx - 6 + i*3
		fillRect(img, x, cy+6, x+1, cy+11, c)
	}
}

func drawSnow(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloudy(img, cx, cy-1, c)
	for i := -1; i <= 1; i++ {
		x := cx + i*5
		px(img, x, cy+6, c)
		px(img, x+1, cy+6, c)
		px(img, x, cy+7, c)
		px(img, x-1, cy+7, c)
		px(img, x, cy+8, c)
		px(img, x+1, cy+8, c)
	}
}

func drawStorm(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloudy(img, cx, cy-1, c)
	px(img, cx-4, cy+6, c)
	px(img, cx-4, cy+7, c)
	px(img, cx-3, cy+6, c)
	px(img, cx-2, cy+7, c)
	px(img, cx-2, cy+8, c)
	px(img, cx-1, cy+7, c)
	px(img, cx, cy+8, c)
	px(img, cx, cy+9, c)
	px(img, cx+1, cy+8, c)
}

// ICO wrapper for Windows (embeds PNG data in ICO container)
func encodeICO(pngData []byte, size int) []byte {
	ico := &bytes.Buffer{}
	binary.Write(ico, binary.LittleEndian, uint16(0))  // reserved
	binary.Write(ico, binary.LittleEndian, uint16(1))  // type = ICO
	binary.Write(ico, binary.LittleEndian, uint16(1))  // count = 1

	w := uint8(size)
	if w > 255 {
		w = 0
	}
	h := uint8(size)
	if h > 255 {
		h = 0
	}
	binary.Write(ico, binary.LittleEndian, w)                        // width
	binary.Write(ico, binary.LittleEndian, h)                        // height
	binary.Write(ico, binary.LittleEndian, uint8(0))                 // palette colors
	binary.Write(ico, binary.LittleEndian, uint8(0))                 // reserved
	binary.Write(ico, binary.LittleEndian, uint16(1))                // color planes
	binary.Write(ico, binary.LittleEndian, uint16(32))               // bits per pixel
	binary.Write(ico, binary.LittleEndian, uint32(len(pngData)))     // size of image data
	binary.Write(ico, binary.LittleEndian, uint32(6+16))             // offset of image data

	ico.Write(pngData)
	return ico.Bytes()
}
