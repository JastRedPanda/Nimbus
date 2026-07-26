// Package wicons owns the embedded colour weather artwork and turns a WMO
// weather code into pixels.
//
// The artwork is Meteocons (https://github.com/basmilius/meteocons) by Bas
// Milius, MIT licensed, taken from the @meteocons/svg-static@0.1.0 npm
// package (see THIRD-PARTY-LICENSES for the verbatim notice this ships to
// satisfy). The upstream files are SVG; they are rasterised to PNG at BUILD
// time and the resulting files are committed next to this source, because
// Nimbus has no cgo and therefore no librsvg, and the gdk-pixbuf SVG loader
// is not present on every target box either - decoding a PNG needs only the
// stdlib. See internal/wicons/generate.sh for the exact rsvg-convert
// invocation that produced the committed PNGs.
//
// Three sizes ship: 32, 64 and 128px. Anything smaller stops being legible -
// the precipitation-type icons (drizzle/rain/sleet/snow) are visually
// indistinguishable below about 48px - and each size is rasterised directly
// from the source SVG rather than scaled from one master, because scaling
// visibly softens the thin features that carry the icon's identity: the
// sun's rays and the 2px precipitation droplets.
package wicons

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"sync"
)

//go:embed icons32/*.png icons64/*.png icons128/*.png
var files embed.FS

// Size is one of the three rasterised sizes committed into this package.
type Size int

// The exact sizes embedded. There is no in-between: asking Icon for any other
// value returns nil rather than silently scaling a nearby size, because a
// scaled bitmap is exactly the softened artwork this package exists to avoid.
const (
	Size32  Size = 32
	Size64  Size = 64
	Size128 Size = 128
)

// name maps a WMO weather code to a Meteocons file stem.
//
// Open-Meteo's daily endpoint returns a whole-day summary: there is no
// sunrise/sunset and no is_day flag in the payload weather.FetchDaily asks
// for (see internal/weather/weather.go), so every entry here is a day
// variant. Meteocons ships matching *-night-* artwork; using it would need
// asking Open-Meteo for daily=sunrise,sunset or current=is_day, which only
// makes sense for a current-conditions view, not a 7-day summary.
//
// The three-way split partly-cloudy / overcast / extreme carries the
// intensity the WMO code encodes: light, moderate, heavy. "extreme" is
// Meteocons' dark-cloud variant, the only one of the three that stays
// legible below 48px.
func name(code int) string {
	switch code {
	case 0:
		return "clear-day"
	case 1:
		return "mostly-clear-day"
	case 2:
		return "partly-cloudy-day"
	case 3:
		// Overcast means the sky is fully covered, so the sunless variant is
		// the accurate one - and it also keeps code 3 visibly different from
		// code 2, whose artwork otherwise differs by one small cloud lobe.
		return "overcast"
	case 45:
		return "fog-day"
	case 48:
		return "extreme-day-fog"

	case 51:
		return "partly-cloudy-day-drizzle"
	case 53:
		return "overcast-day-drizzle"
	case 55:
		return "extreme-day-drizzle"

	// Freezing drizzle and freezing rain both read as sleet: Meteocons has no
	// "freezing drizzle" artwork, and sleet is the closer visual match than
	// plain rain or snow. The severity ladder still holds - 56 (light
	// freezing drizzle) is the lightest cloud, 66 (light freezing rain) is
	// next, and 57/67 (dense freezing drizzle, heavy freezing rain) share the
	// heaviest.
	case 56:
		return "partly-cloudy-day-sleet"
	case 57, 67:
		return "extreme-day-sleet"
	case 66:
		return "overcast-day-sleet"

	// Steady rain (61-65) and rain showers (80-82) share artwork: a daily
	// summary cannot show the difference between "all day" and "in bursts",
	// and inventing a distinction would be a lie.
	case 61, 80:
		return "partly-cloudy-day-rain"
	case 63, 81:
		return "overcast-day-rain"
	case 65, 82:
		return "extreme-day-rain"

	case 71, 85:
		return "partly-cloudy-day-snow"
	case 73, 77:
		return "overcast-day-snow"
	case 75, 86:
		return "extreme-day-snow"

	case 95:
		return "thunderstorms-day-rain"
	case 96:
		return "thunderstorms-day-hail"
	case 99:
		return "extreme-thunderstorms-day-hail"

	default:
		// A code Open-Meteo added after this table was written. A neutral
		// cloud is honest; guessing a condition is not.
		return "cloudy"
	}
}

type key struct {
	stem string
	size Size
}

var (
	mu    sync.Mutex
	cache = map[key]*image.NRGBA{}
)

// Icon returns the artwork for a WMO code at one of the three embedded sizes,
// decoded to straight-alpha NRGBA - what gdk_pixbuf_new_from_data expects;
// premultiplied pixels would darken every antialiased edge. It returns nil if
// size is not one of Size32, Size64 or Size128, or if the embedded asset is
// missing, which would be a build error rather than a runtime condition.
//
// Results are cached, which also keeps the backing array alive: callers that
// hand img.Pix to a toolkit which does not copy the buffer (gtk.NewImageRGBA
// on Linux does not) need it reachable for as long as the image is displayed,
// and a forecast window redraws the same handful of icons every time it
// opens.
func Icon(code int, size Size) *image.NRGBA {
	if size != Size32 && size != Size64 && size != Size128 {
		return nil
	}
	k := key{name(code), size}

	mu.Lock()
	defer mu.Unlock()
	if img, ok := cache[k]; ok {
		return img
	}

	b, err := files.ReadFile(fmt.Sprintf("icons%d/%s.png", int(size), k.stem))
	if err != nil {
		return nil
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}

	img, ok := src.(*image.NRGBA)
	if !ok || img.Bounds().Min != (image.Point{}) {
		bd := src.Bounds()
		dst := image.NewNRGBA(image.Rect(0, 0, bd.Dx(), bd.Dy()))
		draw.Draw(dst, dst.Bounds(), src, bd.Min, draw.Src)
		img = dst
	}

	cache[k] = img
	return img
}
