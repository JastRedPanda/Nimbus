//go:build linux

package ui

import (
	"bytes"
	_ "embed"
	"image"
	"image/draw"
	_ "image/png"
	"log"
	"sync"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
)

// Both sizes come from nimbusicon.ico, the same artwork the Windows build uses:
// 32 is the icon's own frame, drawn for title bars, and 128 covers the window
// switcher and HiDPI without the window manager having to rescale the small one.
//
//go:embed appicon32.png
var appIcon32PNG []byte

//go:embed appicon128.png
var appIcon128PNG []byte

// appName titles windows that have no better caption of their own, matching the
// Win32 backend's MessageBox title.
const appName = "Nimbus"

var appIconOnce sync.Once

// ensureAppIcon installs the icon every Nimbus window inherits. Must be called
// on the GTK thread, before the first window is shown.
func ensureAppIcon() {
	appIconOnce.Do(func() {
		var imgs []gtk.Image
		for _, data := range [][]byte{appIcon32PNG, appIcon128PNG} {
			img := decodeRGBA(data)
			if img == nil {
				continue
			}
			b := img.Bounds()
			imgs = append(imgs, gtk.Image{Pix: img.Pix, W: b.Dx(), H: b.Dy(), Stride: img.Stride})
		}
		if len(imgs) == 0 {
			return
		}
		gtk.SetDefaultIcons(imgs...)
	})
}

// decodeRGBA converts an encoded image into the straight-alpha RGBA layout
// gdk-pixbuf expects, whatever concrete type the decoder produced.
func decodeRGBA(data []byte) *image.NRGBA {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("ui: decode image: %v", err)
		return nil
	}
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}
