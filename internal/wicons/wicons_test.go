package wicons

import (
	"fmt"
	"strings"
	"testing"
)

// allCodes is every WMO weather_code value Open-Meteo's daily endpoint can
// return, per internal/weather/weather.go's daily=... query.
var allCodes = []int{
	0, 1, 2, 3, 45, 48,
	51, 53, 55, 56, 57,
	61, 63, 65, 66, 67,
	71, 73, 75, 77,
	80, 81, 82, 85, 86,
	95, 96, 99,
}

func TestEveryWMOCodeResolves(t *testing.T) {
	for _, code := range allCodes {
		for _, size := range []Size{Size32, Size64, Size128} {
			img := Icon(code, size)
			if img == nil {
				t.Errorf("Icon(%d, %d) = nil, want decoded artwork (stem %q)", code, size, name(code))
				continue
			}
			b := img.Bounds()
			if b.Dx() != int(size) || b.Dy() != int(size) {
				t.Errorf("Icon(%d, %d) size = %dx%d, want %dx%d", code, size, b.Dx(), b.Dy(), size, size)
			}
		}
	}
}

func TestUnknownCodeFallsBack(t *testing.T) {
	if got := name(12345); got != "cloudy" {
		t.Errorf("name(12345) = %q, want %q", got, "cloudy")
	}
	if Icon(12345, Size64) == nil {
		t.Error("Icon(12345, Size64) = nil, want the cloudy fallback")
	}
}

func TestInvalidSizeReturnsNil(t *testing.T) {
	if img := Icon(0, 96); img != nil {
		t.Errorf("Icon(0, 96) = %v, want nil (96 is not an embedded size)", img)
	}
}

func TestIconsAreNonEmptyStraightAlpha(t *testing.T) {
	img := Icon(0, Size64)
	if img == nil {
		t.Fatal("Icon(0, Size64) = nil")
	}
	var opaque, transparent int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0xffff {
				opaque++
			} else if a == 0 {
				transparent++
			}
		}
	}
	// clear-day is a sun on a transparent background: both fully-opaque and
	// fully-transparent pixels should be present in quantity, which is only
	// true for straight (non-premultiplied) alpha PNG decoding.
	if opaque == 0 {
		t.Error("clear-day has no opaque pixels at all")
	}
	if transparent == 0 {
		t.Error("clear-day has no transparent pixels at all - expected a transparent background")
	}
}

func TestCacheReturnsSameBuffer(t *testing.T) {
	a := Icon(0, Size64)
	b := Icon(0, Size64)
	if a == nil || b == nil {
		t.Fatal("Icon(0, Size64) = nil")
	}
	if &a.Pix[0] != &b.Pix[0] {
		t.Error("Icon returned different backing arrays for the same (code, size) - gdk_pixbuf_new_from_data retains by pointer, so this would grow the retained map unboundedly")
	}
}

func TestUniqueStemCount(t *testing.T) {
	seen := map[string]bool{}
	for _, code := range allCodes {
		seen[name(code)] = true
	}
	seen["cloudy"] = true // the fallback stem, exercised by unknown codes
	if len(seen) != 22 {
		t.Errorf("mapping resolves to %d unique stems, want 22", len(seen))
	}
}

// TestNoOrphanedAssets guards the other direction from TestEveryWMOCodeResolves:
// that test proves every code has a file, this one proves every file has a code.
// Dropping a code from name() without pruning generate.sh leaves PNGs that the
// embed pattern still ships as dead payload, and nothing else would notice.
func TestNoOrphanedAssets(t *testing.T) {
	used := map[string]bool{"cloudy": true}
	for _, code := range allCodes {
		used[name(code)] = true
	}
	for _, size := range []Size{Size32, Size64, Size128} {
		dir := fmt.Sprintf("icons%d", int(size))
		entries, err := files.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			stem := strings.TrimSuffix(e.Name(), ".png")
			if !used[stem] {
				t.Errorf("%s/%s is embedded but no weather code maps to it", dir, e.Name())
			}
		}
	}
}
