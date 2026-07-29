package icons

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func bgCol(temp float64) color.RGBA {
	_ = temp
	return color.RGBA{R: 50, G: 50, B: 50, A: 220}
}

func tempStr(temp float64) string {
	t := int(math.Round(temp))
	if t > 0 {
		return fmt.Sprintf("+%d", t)
	}
	return fmt.Sprintf("%d", t)
}

func Generate(temp float64, code int, theme string) []byte {
	return MakeIcon(temp, code, theme, 100)
}

func GenerateScale(temp float64, code int, theme string, fontScale int) []byte {
	return MakeIcon(temp, code, theme, fontScale)
}

func MakeIcon(temp float64, code int, theme string, fontScale int) []byte {
	_ = code
	_ = theme
	s32 := render(temp, 32, fontScale)
	s64 := render(temp, 64, fontScale)
	return encodeIcon(s32, s64)
}

func render(temp float64, sz int, fontScale int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, sz, sz))
	bg := bgCol(temp)

	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
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

	drawTempFit(img, temp, sz, fontScale)

	return img
}

func drawTempFit(img *image.RGBA, temp float64, sz int, fontScale int) {
	text := tempStr(temp)
	face := basicfont.Face7x13

	adv := font.MeasureString(face, text)
	baseW := adv.Ceil()
	baseH := face.Metrics().Height.Ceil()

	targetH := float64(sz-2) * float64(fontScale) / 100.0
	if targetH < 1 {
		targetH = 1
	}

	maxH := float64(sz) * 0.9
	if targetH > maxH {
		targetH = maxH
	}

	scale := int(math.Ceil(targetH / float64(baseH)))
	if scale < 1 {
		scale = 1
	}
	if scale > 4 {
		scale = 4
	}

	small := image.NewRGBA(image.Rect(0, 0, baseW, baseH))
	white := image.NewUniform(color.White)
	d := &font.Drawer{
		Dst:  small,
		Src:  white,
		Face: face,
		Dot:  fixed.P(0, face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(text)

	if scale > 1 {
		big := image.NewRGBA(image.Rect(0, 0, baseW*scale, baseH*scale))
		for y := 0; y < baseH*scale; y++ {
			for x := 0; x < baseW*scale; x++ {
				big.Set(x, y, small.At(x/scale, y/scale))
			}
		}
		small = big
	}

	srcW := float64(baseW * scale)
	srcH := float64(baseH * scale)
	dstW := targetH * srcW / srcH
	if dstW > float64(sz) {
		dstW = float64(sz)
	}

	xOff := (float64(sz) - dstW) / 2
	yOff := (float64(sz) - targetH) / 2
	if yOff < 0 {
		yOff = 0
	}
	if xOff < 0 {
		xOff = 0
	}

	src := image.Rectangle{Max: image.Point{X: int(srcW), Y: int(srcH)}}
	dst := image.Rectangle{
		Min: image.Point{X: int(xOff), Y: int(yOff)},
		Max: image.Point{X: int(xOff + dstW), Y: int(yOff + targetH)},
	}
	draw.BiLinear.Scale(img, dst, small, src, draw.Over, nil)
}


