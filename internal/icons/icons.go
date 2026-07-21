package icons

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func symbolColor(theme string, temp float64) color.RGBA {
	switch theme {
	case "dark":
		return color.RGBA{200, 210, 230, 255}
	case "light":
		return color.RGBA{20, 25, 40, 255}
	default:
		return tempColor(temp)
	}
}

func tempColor(temp float64) color.RGBA {
	switch {
	case temp < -10:
		return color.RGBA{140, 170, 255, 255}
	case temp < 0:
		return color.RGBA{80, 130, 255, 255}
	case temp < 10:
		return color.RGBA{50, 110, 240, 255}
	case temp < 20:
		return color.RGBA{60, 190, 60, 255}
	case temp < 30:
		return color.RGBA{230, 170, 30, 255}
	default:
		return color.RGBA{210, 50, 40, 255}
	}
}

func Generate(temp float64, weatherCode int, theme string) []byte {
	icon := image.NewRGBA(image.Rect(0, 0, 32, 32))
	cx, cy := 16, 16
	col := symbolColor(theme, temp)

	switch {
	case weatherCode == 0:
		drawClear(icon, cx, cy, col)
	case weatherCode <= 3:
		drawCloudy(icon, cx, cy, col)
	case weatherCode >= 45 && weatherCode <= 48:
		drawFog(icon, cx, cy, col)
	case weatherCode >= 51 && weatherCode <= 57:
		drawRain(icon, cx, cy, col, false)
	case weatherCode >= 61 && weatherCode <= 65:
		drawRain(icon, cx, cy, col, true)
	case weatherCode >= 71 && weatherCode <= 77:
		drawSnow(icon, cx, cy, col)
	case weatherCode >= 80 && weatherCode <= 86:
		drawRain(icon, cx, cy, col, true)
	case weatherCode >= 95:
		drawStorm(icon, cx, cy, col)
	default:
		drawCloudy(icon, cx, cy, col)
	}

	var buf bytes.Buffer
	png.Encode(&buf, icon)
	return buf.Bytes()
}

func px(img *image.RGBA, x, y int, c color.RGBA) {
	if x >= 0 && x < 32 && y >= 0 && y < 32 {
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
	fillCircle(img, cx, cy, 6, c)
	for _, d := range [][2]int{
		{0, -9}, {0, 9}, {-9, 0}, {9, 0},
		{-6, -6}, {6, 6}, {-6, 6}, {6, -6},
	} {
		fillRect(img, cx+d[0]-1, cy+d[1]-1, cx+d[0]+1, cy+d[1]+1, c)
	}
}

func drawCloudy(img *image.RGBA, cx, cy int, c color.RGBA) {
	fillCircle(img, cx-5, cy, 5, c)
	fillCircle(img, cx+4, cy, 5, c)
	fillCircle(img, cx-1, cy-3, 4, c)
	fillRect(img, cx-9, cy, cx+8, cy+3, c)
}

func drawFog(img *image.RGBA, cx, cy int, c color.RGBA) {
	for y := -4; y <= 4; y += 2 {
		fillRect(img, cx-9, cy+y, cx+9, cy+y+1, c)
	}
}

func drawRain(img *image.RGBA, cx, cy int, c color.RGBA, heavy bool) {
	drawCloudy(img, cx, cy-1, c)
	n := 3
	if heavy {
		n = 5
	}
	for i := 0; i < n; i++ {
		x := cx - 5 + i*3
		fillRect(img, x, cy+5, x+1, cy+9, c)
	}
}

func drawSnow(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloudy(img, cx, cy-1, c)
	for i := -1; i <= 1; i++ {
		x := cx + i*4
		px(img, x, cy+5, c)
		px(img, x+1, cy+5, c)
		px(img, x, cy+6, c)
		px(img, x-1, cy+6, c)
		px(img, x, cy+7, c)
		px(img, x+1, cy+7, c)
	}
}

func drawStorm(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloudy(img, cx, cy-1, c)
	px(img, cx-4, cy+5, c)
	px(img, cx-4, cy+6, c)
	px(img, cx-3, cy+5, c)
	px(img, cx-2, cy+6, c)
	px(img, cx-2, cy+7, c)
	px(img, cx-1, cy+6, c)
	px(img, cx, cy+7, c)
	px(img, cx, cy+8, c)
	px(img, cx+1, cy+7, c)
}
