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
		return color.RGBA{220, 220, 230, 255}
	case "light":
		return color.RGBA{30, 30, 40, 255}
	default:
		return tempColor(temp)
	}
}

func tempColor(temp float64) color.RGBA {
	switch {
	case temp < -10:
		return color.RGBA{180, 200, 255, 255}
	case temp < 0:
		return color.RGBA{100, 150, 255, 255}
	case temp < 10:
		return color.RGBA{60, 120, 255, 255}
	case temp < 20:
		return color.RGBA{80, 200, 80, 255}
	case temp < 30:
		return color.RGBA{240, 180, 40, 255}
	default:
		return color.RGBA{220, 60, 50, 255}
	}
}

func Generate(temp float64, weatherCode int, theme string) []byte {
	icon := image.NewRGBA(image.Rect(0, 0, 32, 32))
	cx, cy := 16, 16

	sc := symbolColor(theme, temp)
	dim := color.RGBA{sc.R, sc.G, sc.B, 60}
	drawSymbol(icon, cx, cy, weatherCode, sc, dim)

	var buf bytes.Buffer
	if err := png.Encode(&buf, icon); err != nil {
		return nil
	}
	return buf.Bytes()
}

func drawSymbol(img *image.RGBA, cx, cy, code int, c, dim color.RGBA) {
	switch {
	case code == 0:
		drawSun(img, cx, cy, c)
		drawSunGlow(img, cx, cy, dim)
	case code <= 3:
		drawSunCloud(img, cx, cy, c, dim)
	case code >= 45 && code <= 48:
		drawFog(img, cx, cy, dim)
		drawFog(img, cx, cy, c)
	case code >= 51 && code <= 57:
		drawRain(img, cx, cy, c, false)
	case code >= 61 && code <= 65:
		drawRain(img, cx, cy, c, true)
	case code >= 71 && code <= 77:
		drawSnow(img, cx, cy, c)
	case code >= 80 && code <= 86:
		drawRain(img, cx, cy, c, true)
	case code >= 95:
		drawStorm(img, cx, cy, c)
	default:
		drawCloud(img, cx, cy, dim)
		drawCloud(img, cx, cy, c)
	}
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	if x >= 0 && x < 32 && y >= 0 && y < 32 {
		img.Set(x, y, c)
	}
}

func drawSunGlow(img *image.RGBA, cx, cy int, c color.RGBA) {
	for dx := -7; dx <= 7; dx++ {
		for dy := -7; dy <= 7; dy++ {
			d := dx*dx + dy*dy
			if d <= 20 {
				set(img, cx+dx, cy+dy, c)
			}
		}
	}
}

func drawSun(img *image.RGBA, cx, cy int, c color.RGBA) {
	for dx := -5; dx <= 5; dx++ {
		for dy := -5; dy <= 5; dy++ {
			d := dx*dx + dy*dy
			if d >= 4 && d <= 9 {
				set(img, cx+dx, cy+dy, c)
			}
		}
	}
	for dx := -5; dx <= 5; dx++ {
		for dy := -5; dy <= 5; dy++ {
			d := dx*dx + dy*dy
			if d >= 14 && d <= 22 {
				set(img, cx+dx, cy+dy, c)
			}
		}
	}
	set(img, cx-6, cy-2, c)
	set(img, cx+6, cy-2, c)
	set(img, cx-6, cy+2, c)
	set(img, cx+6, cy+2, c)
	set(img, cx-2, cy-6, c)
	set(img, cx+2, cy-6, c)
	set(img, cx-2, cy+6, c)
	set(img, cx+2, cy+6, c)
}

func drawSunCloud(img *image.RGBA, cx, cy int, c, dim color.RGBA) {
	for dx := -4; dx <= 4; dx++ {
		for dy := -4; dy <= 4; dy++ {
			d := dx*dx + dy*dy
			if d >= 4 && d <= 9 {
				set(img, cx-5+dx, cy-3+dy, c)
			}
		}
	}
	set(img, cx-11, cy-5, c)
	set(img, cx-5, cy-5, c)
	set(img, cx-11, cy+1, c)
	set(img, cx-5, cy+1, c)
	set(img, cx-8, cy-8, c)
	set(img, cx-8, cy+4, c)
	drawCloud(img, cx+2, cy+1, dim)
	drawCloud(img, cx+2, cy+1, c)
}

func drawCloud(img *image.RGBA, cx, cy int, c color.RGBA) {
	for x := -4; x <= 4; x++ {
		set(img, cx+x, cy-1, c)
		set(img, cx+x, cy, c)
		set(img, cx+x, cy+1, c)
	}
	for x := -2; x <= 2; x++ {
		set(img, cx+x, cy-2, c)
		set(img, cx+x, cy+2, c)
	}
	for x := -3; x <= 3; x++ {
		set(img, cx+x, cy-3, c)
	}
	for x := -2; x <= 4; x++ {
		set(img, cx+x, cy+3, c)
	}
}

func drawFog(img *image.RGBA, cx, cy int, c color.RGBA) {
	for y := -4; y <= 4; y += 2 {
		for x := -7; x <= 7; x++ {
			set(img, cx+x, cy+y, c)
		}
	}
}

func drawRain(img *image.RGBA, cx, cy int, c color.RGBA, heavy bool) {
	drawCloud(img, cx, cy, c)
	n := 3
	if heavy {
		n = 5
	}
	for i := 0; i < n; i++ {
		x := cx - 4 + i*2
		set(img, x, cy+4, c)
		set(img, x, cy+5, c)
		set(img, x-1, cy+6, c)
		set(img, x+1, cy+6, c)
	}
}

func drawSnow(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloud(img, cx, cy, c)
	for i := -1; i <= 1; i++ {
		x := cx + i*4
		set(img, x, cy+4, c)
		set(img, x+1, cy+5, c)
		set(img, x-1, cy+5, c)
		set(img, x, cy+6, c)
	}
}

func drawStorm(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloud(img, cx, cy, c)
	set(img, cx-3, cy+4, c)
	set(img, cx-2, cy+4, c)
	set(img, cx-2, cy+5, c)
	set(img, cx-1, cy+5, c)
	set(img, cx, cy+6, c)
	set(img, cx+1, cy+6, c)
}
