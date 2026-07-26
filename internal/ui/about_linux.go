//go:build linux

package ui

import (
	_ "embed"
	"image"
	"log"
	"sync"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/webui"
)

//go:embed about_logo.png
var aboutLogoPNG []byte

const (
	aboutWidth = 320
	// -1 lets GTK size the window to its natural content height instead of
	// leaving the dead space a fixed height would.
	aboutHeight   = -1
	aboutSubtitle = "Мультиплатформний інформер погоди."
)

var (
	logoOnce sync.Once
	logoRGBA *image.NRGBA

	// aboutWindow is read and written only on the GTK thread, so it needs no
	// lock. Everything that touches it runs inside gtk.Invoke or a signal
	// handler, both of which are dispatched by the GTK main loop.
	aboutWindow gtk.Window
)

// ShowAbout opens the About window and returns immediately; the window is
// built on the GTK main loop thread.
func showAbout(theme string) {
	if !gtk.Ready() {
		// No usable GTK on this machine. Degrade to the browser UI rather than
		// leaving the menu item doing nothing at all.
		webui.ShowAbout(theme)
		return
	}
	if err := gtk.Invoke(func() { buildAbout(theme) }); err != nil {
		log.Printf("about: schedule failed: %v", err)
	}
}

func buildAbout(theme string) {
	if aboutWindow != 0 {
		aboutWindow.Present()
		return
	}

	ensureAppIcon()
	gtk.PreferDark(theme)

	win := gtk.NewWindow("About Nimbus", aboutWidth, aboutHeight, false)
	win.SetBorder(20)
	win.OnDestroy(func() { aboutWindow = 0 })

	box := gtk.NewVBox(12)
	win.Add(box)

	if logo := aboutLogo(); logo != nil {
		b := logo.Bounds()
		if img := gtk.NewImageRGBA(logo.Pix, b.Dx(), b.Dy(), logo.Stride); img != 0 {
			gtk.PackStart(box, img, false, false, 0)
		}
	}
	gtk.PackStart(box, gtk.NewLabel(`<span size="xx-large" weight="bold">Nimbus</span>`), false, false, 0)
	gtk.PackStart(box, gtk.NewText(aboutSubtitle), false, false, 0)

	win.ShowAll()
	aboutWindow = win
}

// aboutLogo decodes the embedded logo once. Unlike the Win32 backend, which
// blits with SRCCOPY and loses the alpha channel, this keeps the logo's
// transparency.
func aboutLogo() *image.NRGBA {
	logoOnce.Do(func() { logoRGBA = decodeRGBA(aboutLogoPNG) })
	return logoRGBA
}
