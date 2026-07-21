package icons

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

func tempColor(temp float64) color.RGBA {
	switch {
	case temp < -10:
		return color.RGBA{30, 60, 180, 255}
	case temp < 0:
		return color.RGBA{50, 100, 220, 255}
	case temp < 10:
		return color.RGBA{100, 160, 255, 255}
	case temp < 20:
		return color.RGBA{80, 200, 80, 255}
	case temp < 30:
		return color.RGBA{240, 180, 40, 255}
	default:
		return color.RGBA{220, 60, 50, 255}
	}
}

func Generate(temp float64, weatherCode int) []byte {
	size := 24
	icon := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := tempColor(temp)
	draw.Draw(icon, icon.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	white := color.RGBA{255, 255, 255, 255}

	cx, cy := size/2, size/2
	r := size/2 - 1

	fillCircle(icon, cx, cy, r, bg)

	drawCircle(icon, cx, cy, r, white)

	switch {
	case weatherCode == 0:
		drawSun(icon, cx, cy)
	case weatherCode <= 3:
		drawCloud(icon, cx, cy, true)
	case weatherCode >= 45 && weatherCode <= 48:
		drawFog(icon, cx, cy)
	case weatherCode >= 51 && weatherCode <= 57:
		drawRain(icon, cx, cy, false)
	case weatherCode >= 61 && weatherCode <= 65:
		drawRain(icon, cx, cy, true)
	case weatherCode >= 71 && weatherCode <= 77:
		drawSnow(icon, cx, cy)
	case weatherCode >= 80 && weatherCode <= 86:
		drawRain(icon, cx, cy, true)
	case weatherCode >= 95:
		drawStorm(icon, cx, cy)
	default:
		drawCloud(icon, cx, cy, false)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, icon)
	return buf.Bytes()
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				if x >= 0 && x < img.Bounds().Dx() && y >= 0 && y < img.Bounds().Dy() {
					img.Set(x, y, c)
				}
			}
		}
	}
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

func set(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
		img.Set(x, y, c)
	}
}

func drawSun(img *image.RGBA, cx, cy int) {
	white := color.RGBA{255, 255, 255, 255}
	for dx := -3; dx <= 3; dx++ {
		for dy := -3; dy <= 3; dy++ {
			if dx*dx+dy*dy <= 6 {
				set(img, cx+dx, cy+dy, white)
			}
		}
	}
}

func drawCloud(img *image.RGBA, cx, cy int, partial bool) {
	white := color.RGBA{255, 255, 255, 255}
	for dy := -2; dy <= 2; dy++ {
		for dx := -6; dx <= 6; dx++ {
			if dy > -2 && dy < 2 {
				set(img, cx+dx, cy+dy, white)
			}
		}
	}
	for dx := -3; dx <= 3; dx++ {
		set(img, cx+dx, cy-3, white)
	}
	for dx := -3; dx <= 4; dx++ {
		set(img, cx+dx, cy+3, white)
	}
}

func drawFog(img *image.RGBA, cx, cy int) {
	white := color.RGBA{255, 255, 255, 200}
	for dy := -3; dy <= 3; dy += 2 {
		for dx := -6; dx <= 6; dx++ {
			set(img, cx+dx, cy+dy, white)
		}
	}
}

func drawRain(img *image.RGBA, cx, cy int, heavy bool) {
	drawCloud(img, cx, cy, true)
	blue := color.RGBA{150, 180, 255, 255}
	count := 3
	if heavy {
		count = 5
	}
	for i := 0; i < count; i++ {
		x := cx - 4 + i*2
		set(img, x, cy+4, blue)
		set(img, x, cy+5, blue)
		set(img, x, cy+6, blue)
	}
}

func drawSnow(img *image.RGBA, cx, cy int) {
	drawCloud(img, cx, cy, true)
	white := color.RGBA{255, 255, 255, 255}
	for i := -1; i <= 1; i++ {
		x := cx + i*4
		set(img, x, cy+4, white)
		set(img, x+1, cy+5, white)
		set(img, x, cy+6, white)
		set(img, x-1, cy+5, white)
	}
}

func drawStorm(img *image.RGBA, cx, cy int) {
	drawCloud(img, cx, cy, true)
	yellow := color.RGBA{255, 220, 50, 255}
	set(img, cx-3, cy+4, yellow)
	set(img, cx-2, cy+4, yellow)
	set(img, cx-1, cy+5, yellow)
	set(img, cx, cy+5, yellow)
	set(img, cx+1, cy+6, yellow)
	set(img, cx+2, cy+6, yellow)
}
