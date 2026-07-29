//go:build linux && !qt

package ui

import (
	_ "embed"
	"github.com/JastRedPanda/Nimbus/internal/build"
	"image"
	"log"
	"strings"
	"sync"

	"github.com/JastRedPanda/Nimbus/internal/gtk"
)

//go:embed about_logo.png
var aboutLogoPNG []byte

const (
	aboutWidth = 320
	// -1 lets GTK size the window to its natural content height instead of
	// leaving the dead space a fixed height would.
	aboutHeight = -1

	// The OK button is a third of the window wide and centred. Win32 carries the
	// same fraction of the same 320 units in aboutBtnW, so the two windows agree.
	aboutBtnW = aboutWidth / 3
)

var (
	logoOnce sync.Once
	logoRGBA *image.NRGBA

	// aboutWindow is read and written only on the GTK thread, so it needs no
	// lock. Everything that touches it runs inside gtk.Invoke or a signal
	// handler, both of which are dispatched by the GTK main loop.
	aboutWindow gtk.Window
)

// showAbout opens the About window and returns immediately; the window is
// built on the GTK main loop thread.
func showAbout(theme string) {
	// No GTK check here: choosing a backend is internal/gui's job, and this
	// code only runs because the gtk3 backend was selected. A second, hidden
	// fallback would let a forced backend quietly behave like another one.
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
	// Parity with the Win32 About box, which has answered Escape since it was
	// written while this one quietly did not.
	win.OnEscape(func() { win.Destroy() })

	box := gtk.NewVBox(12)
	win.Add(box)

	if logo := aboutLogo(); logo != nil {
		b := logo.Bounds()
		if img := gtk.NewImageRGBA(logo.Pix, b.Dx(), b.Dy(), logo.Stride); img != 0 {
			gtk.PackStart(box, img, false, false, 0)
		}
	}
	gtk.PackStart(box, gtk.NewLabel(`<span size="xx-large" weight="bold">Nimbus</span>`), false, false, 0)
	gtk.PackStart(box, gtk.NewText(build.Subtitle), false, false, 0)
	// Muted with an explicit grey rather than a theme colour: this window uses
	// the desktop theme, and mid-grey is legible against both a light and a
	// dark one.
	gtk.PackStart(box, gtk.NewLabel(`<span size="small" foreground="#888888">`+
		escapeMarkup(versionLine())+`</span>`), false, false, 0)

	// An explicit way out, because Escape and the title bar are not obvious ones
	// and this window has nothing else to click. halign does the centring; no
	// spacer widget for the desktop theme to state a colour and a padding on.
	//
	// Packed WITHOUT expand and fill, unlike the visually similar closeRow in
	// forecast_linux.go. That one is a horizontal box, where those flags are what
	// give the child the full row width for halign to work against. This is a
	// vertical box, so the same flags mean "take all the spare height and fill
	// it": the button would stretch into a slab down the side of the window the
	// moment anything gave the box height to spare. Harmless today, because the
	// window takes its natural height and cannot be resized - which is exactly the
	// kind of accident that survives until someone sets a height.
	//
	// "OK" is spelled the same in both languages this program speaks, so it needs
	// no entry in internal/i18n - the subtitle above is likewise literal.
	ok := gtk.NewButton("OK", func() { win.Destroy() })
	gtk.SetHAlign(ok, gtk.AlignCenter)
	gtk.SetSizeRequest(ok, aboutBtnW, -1)
	gtk.PackStart(box, ok, false, false, 0)

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

// escapeMarkup makes a literal string safe to embed in Pango markup.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
