package icons

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
)

func symbolColor(theme string, temp float64) color.RGBA {
	switch theme {
	case "dark":
		return color.RGBA{220, 220, 230, 255}
	case "light":
		return color.RGBA{40, 40, 50, 255}
	default:
		return tempColor(temp, false)
	}
}

func bgColor(theme string, temp float64) color.RGBA {
	switch theme {
	case "dark":
		return color.RGBA{255, 255, 255, 30}
	case "light":
		return color.RGBA{0, 0, 0, 20}
	default:
		return tempColor(temp, true)
	}
}

func tempColor(temp float64, dim bool) color.RGBA {
	a := uint8(200)
	if dim {
		a = 70
	}
	switch {
	case temp < -10:
		return color.RGBA{30, 60, 180, a}
	case temp < 0:
		return color.RGBA{50, 100, 220, a}
	case temp < 10:
		return color.RGBA{100, 160, 255, a}
	case temp < 20:
		return color.RGBA{80, 200, 80, a}
	case temp < 30:
		return color.RGBA{240, 180, 40, a}
	default:
		return color.RGBA{220, 60, 50, a}
	}
}

func Generate(temp float64, weatherCode int, theme string) []byte {
	size := 32
	icon := image.NewRGBA(image.Rect(0, 0, size, size))

	cx, cy := size/2, size/2
	r := size/2 - 2

	fillCircle(icon, cx, cy, r, bgColor(theme, temp))
	drawCircle(icon, cx, cy, r, symbolColor(theme, temp))

	sc := symbolColor(theme, temp)

	switch {
	case weatherCode == 0:
		drawSun(icon, cx, cy, sc)
	case weatherCode <= 3:
		drawCloud(icon, cx, cy, sc, true)
	case weatherCode >= 45 && weatherCode <= 48:
		drawFog(icon, cx, cy, sc)
	case weatherCode >= 51 && weatherCode <= 57:
		drawRain(icon, cx, cy, sc, false)
	case weatherCode >= 61 && weatherCode <= 65:
		drawRain(icon, cx, cy, sc, true)
	case weatherCode >= 71 && weatherCode <= 77:
		drawSnow(icon, cx, cy, sc)
	case weatherCode >= 80 && weatherCode <= 86:
		drawRain(icon, cx, cy, sc, true)
	case weatherCode >= 95:
		drawStorm(icon, cx, cy, sc)
	default:
		drawCloud(icon, cx, cy, sc, false)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, icon); err != nil {
		log.Printf("Icon encoding error: %v", err)
		return nil
	}
	return buf.Bytes()
}

func drawCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			dist := dx*dx + dy*dy
			if dist > (r-1)*(r-1) && dist <= r*r {
				if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
					img.Set(x, y, c)
				}
			}
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
					over(img, x, y, c)
				}
			}
		}
	}
}

func over(img *image.RGBA, x, y int, c color.RGBA) {
	_, _, _, a := c.RGBA()
	if a == 0xffff {
		img.Set(x, y, c)
		return
	}
	existing := img.RGBAAt(x, y)
	alpha := float64(c.A) / 255.0
	r := float64(existing.R)*(1-alpha) + float64(c.R)*alpha
	g := float64(existing.G)*(1-alpha) + float64(c.G)*alpha
	b := float64(existing.B)*(1-alpha) + float64(c.B)*alpha
	img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(b), existing.A})
}

func set(img *image.RGBA, x, y int, c color.RGBA) {
	if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
		img.Set(x, y, c)
	}
}

func drawSun(img *image.RGBA, cx, cy int, c color.RGBA) {
	for dx := -4; dx <= 4; dx++ {
		for dy := -4; dy <= 4; dy++ {
			if dx*dx+dy*dy <= 8 {
				set(img, cx+dx, cy+dy, c)
			}
		}
	}
}

func drawCloud(img *image.RGBA, cx, cy int, c color.RGBA, _ bool) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -7; dx <= 7; dx++ {
			if dy > -2 && dy < 2 {
				set(img, cx+dx, cy+dy, c)
			}
		}
	}
	for dx := -4; dx <= 4; dx++ {
		set(img, cx+dx, cy-3, c)
	}
	for dx := -4; dx <= 5; dx++ {
		set(img, cx+dx, cy+3, c)
	}
}

func drawFog(img *image.RGBA, cx, cy int, c color.RGBA) {
	dim := color.RGBA{c.R, c.G, c.B, uint8(float64(c.A) * 0.7)}
	for dy := -3; dy <= 3; dy += 2 {
		for dx := -7; dx <= 7; dx++ {
			set(img, cx+dx, cy+dy, dim)
		}
	}
}

func drawRain(img *image.RGBA, cx, cy int, c color.RGBA, heavy bool) {
	drawCloud(img, cx, cy, c, true)
	count := 3
	if heavy {
		count = 5
	}
	for i := 0; i < count; i++ {
		x := cx - 4 + i*2
		set(img, x, cy+4, c)
		set(img, x, cy+5, c)
		set(img, x, cy+6, c)
	}
}

func drawSnow(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloud(img, cx, cy, c, true)
	for i := -1; i <= 1; i++ {
		x := cx + i*4
		set(img, x, cy+4, c)
		set(img, x+1, cy+5, c)
		set(img, x, cy+6, c)
		set(img, x-1, cy+5, c)
	}
}

func drawStorm(img *image.RGBA, cx, cy int, c color.RGBA) {
	drawCloud(img, cx, cy, c, true)
	set(img, cx-3, cy+4, c)
	set(img, cx-2, cy+4, c)
	set(img, cx-1, cy+5, c)
	set(img, cx, cy+5, c)
	set(img, cx+1, cy+6, c)
	set(img, cx+2, cy+6, c)
}
