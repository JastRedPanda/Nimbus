//go:build windows

package ui

import (
	"bytes"
	_ "embed"
	"github.com/JastRedPanda/Nimbus/internal/build"
	"image"
	_ "image/png"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

//go:embed about_logo.png
var aboutLogoPNG []byte

const (
	aboutClassName = "NimbusAboutClass"
	// WS_CLIPCHILDREN because this window fills its own background in both
	// WM_ERASEBKGND and WM_PAINT, and now has a child: without it every repaint
	// paints over the button first and the button repaints on top, which flashes.
	// It does not affect AdjustWindowRect, so frameOverhead and layoutDPI are
	// unchanged by it.
	aboutStyle = win.WS_CAPTION | win.WS_SYSMENU | win.WS_CLIPCHILDREN

	// Layout in 96-DPI units, top to bottom: padding, logo, title, subtitle,
	// version, padding. Everything goes through dp() on its way to Win32, so
	// at 100% scaling these are the pixels Windows receives.
	aboutContentW  = 320
	aboutPad       = 20
	aboutLogoGap   = 16
	aboutTitleH    = 32
	aboutTitleGap  = 36
	aboutSubtitleH = 40
	aboutVerGap    = 44
	aboutVerH      = 20
	// The OK button, bottom right. Its own gap above it rather than reusing
	// aboutPad: the version line is small grey text and needs more air between it
	// and a control than the window edge needs.
	aboutBtnGap = 18
	// A third of the window wide, the same fraction about_linux.go asks GTK for.
	aboutBtnW = aboutContentW / 3
	aboutBtnH = 26

	// ID_ABOUT_OK is this window's only control. The settings dialog's ids stay
	// below 200 in its own window and cannot collide - WM_COMMAND is delivered to the
	// parent, and these are different parents - but the numbering is kept clear of
	// them anyway so a future shared helper cannot be caught out.
	ID_ABOUT_OK = 200

	// Character heights, again at 96 DPI.
	aboutTitlePx = 24
	aboutBodyPx  = 15

	// AC_SRC_OVER, which lxn/win does not declare - it declares only
	// AC_SRC_ALPHA. Zero means "blend the source over the destination", the
	// only blend operation AlphaBlend has ever supported.
	acSrcOver = 0
)

var (
	aboutClassOnce sync.Once
	aboutClassOK   bool

	aboutLogoOnce   sync.Once
	aboutLogoBitmap win.HBITMAP
	aboutLogoW      int32
	aboutLogoH      int32

	// aboutBusy keeps the window a singleton: clicking About twice should
	// raise the window that is already open, the way the GTK backend presents
	// its existing one, not stack a second copy on top of it.
	aboutBusy atomic.Bool
	aboutHWND atomic.Uintptr
)

type aboutDlg struct {
	// ok is the only control this window has. It is a field because the focus is
	// put on it once the window is up, and because release has to free the font it
	// was given.
	ok win.HWND

	hwnd win.HWND
	inst win.HINSTANCE
	dark bool
	dpi  int32

	bgBrush   win.HBRUSH
	titleFont win.HFONT
	bodyFont  win.HFONT
	// btnFont is the shell's 9pt face, the same uiFont every control in the
	// settings window gets. bodyFont is a 15px cell meant for painted body text,
	// and a button wearing it has a visibly larger caption than every other button
	// in the program.
	btnFont win.HFONT
}

// aboutDialogs maps a window to its dialog, for the same reason
// settings_windows.go does: the alternative parks a Go pointer in
// GWLP_USERDATA, where the garbage collector cannot see it and `go vet`
// cannot approve of it.
var (
	aboutDialogsMu sync.Mutex
	aboutDialogs   = map[win.HWND]*aboutDlg{}
)

func aboutDlgFor(hwnd win.HWND) *aboutDlg {
	aboutDialogsMu.Lock()
	defer aboutDialogsMu.Unlock()
	return aboutDialogs[hwnd]
}

// showAbout opens the About window and returns immediately.
func showAbout(theme string) {
	if !aboutBusy.CompareAndSwap(false, true) {
		if h := win.HWND(aboutHWND.Load()); h != 0 {
			win.SetForegroundWindow(h)
		}
		return
	}
	d := &aboutDlg{dark: resolveDark(theme), dpi: baseDPI}
	go d.run()
}

func (d *aboutDlg) run() {
	// The window, its message queue and its GetMessage loop all have to be on
	// one OS thread; see the same comment in settings_windows.go, which
	// explains what goes wrong without this and why the thread is never
	// handed back.
	runtime.LockOSThread()

	defer aboutBusy.Store(false)
	defer aboutHWND.Store(0)

	d.inst = win.GetModuleHandle(nil)
	if d.inst == 0 {
		log.Print("about: GetModuleHandle failed")
		return
	}

	aboutClassOnce.Do(func() {
		cn := syscall.StringToUTF16(aboutClassName)
		wc := &win.WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			Style:         win.CS_HREDRAW | win.CS_VREDRAW,
			LpfnWndProc:   syscall.NewCallback(aboutWndProc),
			HInstance:     d.inst,
			HIcon:         win.LoadIcon(d.inst, win.MAKEINTRESOURCE(1)),
			HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			HbrBackground: win.COLOR_BTNFACE + 1,
			LpszClassName: &cn[0],
		}
		aboutClassOK = win.RegisterClassEx(wc) != 0
	})
	if !aboutClassOK {
		log.Print("about: RegisterClassEx failed")
		return
	}

	initAboutLogo()

	d.hwnd = win.CreateWindowEx(
		0, syscall.StringToUTF16Ptr(aboutClassName), syscall.StringToUTF16Ptr("About Nimbus"),
		aboutStyle,
		win.CW_USEDEFAULT, win.CW_USEDEFAULT, 100, 100,
		0, 0, d.inst, nil,
	)
	if d.hwnd == 0 {
		log.Print("about: CreateWindowEx failed")
		return
	}

	aboutDialogsMu.Lock()
	aboutDialogs[d.hwnd] = d
	aboutDialogsMu.Unlock()
	defer func() {
		aboutDialogsMu.Lock()
		delete(aboutDialogs, d.hwnd)
		aboutDialogsMu.Unlock()
	}()
	aboutHWND.Store(uintptr(d.hwnd))

	if d.dark {
		d.bgBrush = createDarkBrush()
	}

	contentH := aboutContentH()
	d.dpi = layoutDPI(d.hwnd, aboutContentW, contentH, aboutStyle)
	d.titleFont = aboutFont(d.dp(aboutTitlePx), win.FW_BOLD)
	d.bodyFont = aboutFont(d.dp(aboutBodyPx), win.FW_NORMAL)

	ow, oh := frameOverhead(aboutStyle)
	centreOn(d.hwnd, scaleDPI(aboutContentW, d.dpi)+ow, scaleDPI(contentH, d.dpi)+oh)

	// After centreOn, so the client size the button is placed against is final.
	//
	// Deliberately NOT run through unthemeForDark, and there is no
	// WM_CTLCOLORBTN case for it either. A BS_PUSHBUTTON ignores the brush its
	// parent returns - Microsoft documents that push buttons "are always drawn
	// with the default system colors" and that the returned text colour reaches
	// only the focus rectangle - so neither would colour anything. What
	// unthemeForDark WOULD do is drop this one button to classic grey chrome next
	// to the settings window's themed ones. Its own reason for existing is a group
	// box, a radio or a checkbox drawing over the PARENT's brush, which a push
	// button never does; a themed push button paints its own face and is legible
	// on a dark window as it stands.
	var crc win.RECT
	win.GetClientRect(d.hwnd, &crc)
	bw, bh := int(d.dp(aboutBtnW)), int(d.dp(aboutBtnH))
	pad := int(d.dp(aboutPad))
	d.ok = createButton(d.hwnd, "OK",
		(int(crc.Right)-bw)/2, int(crc.Bottom)-pad-bh, bw, bh, ID_ABOUT_OK)
	if d.ok == 0 {
		// Worth a line: the window still works through Escape, Enter and the title
		// bar, but the button the user was told about is simply absent.
		log.Print("about: could not create the OK button; Escape and the title bar still close the window")
	} else {
		d.btnFont = uiFont(d.dpi)
		if d.btnFont != 0 {
			// Guarded: WM_SETFONT with a null handle selects SYSTEM_FONT, which is
			// the Windows 3.1 bitmap face this is here to avoid in the first place.
			win.SendMessage(d.ok, win.WM_SETFONT, uintptr(d.btnFont), 1)
		}
	}

	if d.dark {
		setDarkTitleBar(d.hwnd, true)
	}
	win.ShowWindow(d.hwnd, win.SW_SHOW)
	win.UpdateWindow(d.hwnd)
	if d.ok != 0 {
		// After ShowWindow, not before. This class runs DefWindowProc, not
		// DefDlgProc, so nothing restores the focus to a child when the window is
		// activated - a SetFocus issued while the window was still hidden would be
		// undone by the activation that ShowWindow triggers, leaving the button
		// without focus and the space bar doing nothing.
		win.SetFocus(d.ok)
	}

	var msg win.MSG
	for {
		switch win.GetMessage(&msg, 0, 0, 0) {
		case 0:
			return
		case -1:
			log.Print("about: GetMessage failed")
			return
		}
		// The same two steps, in the same order, as the settings window: catch
		// Escape by hand, then let IsDialogMessage have everything else.
		//
		// Escape is caught here because the Escape-means-cancel rule lives in the
		// real dialog manager inside DefDlgProc, and this class runs
		// DefWindowProc. IsDialogMessage supplies the rest of the dialog keyboard
		// interface to a plain window that owns controls - Tab, the space bar on
		// the focused button, mnemonics - which is what makes the OK button
		// reachable without a mouse. WM_COMMAND answers IDOK and IDCANCEL as well
		// as the button's own id, so Enter closes the window whichever of them it
		// arrives as.
		//
		// The child test claims the keystroke whether it landed on the window or on
		// the button.
		if msg.Message == win.WM_KEYDOWN && msg.WParam == win.VK_ESCAPE && d.owns(msg.HWnd) {
			win.DestroyWindow(d.hwnd)
			continue
		}
		if !win.IsDialogMessage(d.hwnd, &msg) {
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}
	}
}

// aboutContentH is the client height the layout needs, in 96-DPI units.
func aboutContentH() int {
	h := aboutPad
	if aboutLogoBitmap != 0 {
		h += int(aboutLogoH) + aboutLogoGap
	}
	return h + aboutTitleGap + aboutVerGap + aboutVerH + aboutBtnGap + aboutBtnH + aboutPad
}

// dp scales a layout unit to device pixels.
func (d *aboutDlg) dp(v int) int32 { return scaleDPI(v, d.dpi) }

// owns reports whether hwnd is this window or one of its children, which is what
// decides whether a keystroke on the message queue belongs to this About box.
func (d *aboutDlg) owns(hwnd win.HWND) bool {
	return hwnd == d.hwnd || win.IsChild(d.hwnd, hwnd)
}

// aboutFont builds a font whose character height is px device pixels, in the
// face the shell draws its own text with.
func aboutFont(px, weight int32) win.HFONT {
	lf := win.LOGFONT{
		LfHeight:         -px,
		LfWeight:         weight,
		LfCharSet:        win.DEFAULT_CHARSET,
		LfOutPrecision:   win.OUT_DEFAULT_PRECIS,
		LfClipPrecision:  win.CLIP_DEFAULT_PRECIS,
		LfQuality:        win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	setFaceName(&lf, shellFontFace())
	return win.CreateFontIndirect(&lf)
}

// initAboutLogo decodes the embedded logo into a 32-bit DIB, once.
//
// Go's color model hands back alpha-PREMULTIPLIED components, which is exactly
// the form AlphaBlend wants, so the pixels go straight in.
func initAboutLogo() {
	aboutLogoOnce.Do(func() {
		img, _, err := image.Decode(bytes.NewReader(aboutLogoPNG))
		if err != nil {
			log.Printf("about: decoding the logo failed: %v", err)
			return
		}
		b := img.Bounds()
		w, h := int32(b.Dx()), int32(b.Dy())

		bmi := &win.BITMAPINFOHEADER{
			BiSize:  uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
			BiWidth: w,
			// Negative height asks for a top-down bitmap, so row 0 is the top
			// one and the loop below can walk the image in its own order.
			BiHeight:      -h,
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: win.BI_RGB,
		}

		var bits unsafe.Pointer
		hbm := win.CreateDIBSection(0, bmi, win.DIB_RGB_COLORS, &bits, 0, 0)
		if hbm == 0 || bits == nil {
			log.Print("about: CreateDIBSection failed")
			return
		}

		stride := int(w) * 4
		pixels := unsafe.Slice((*byte)(bits), stride*int(h))
		for y := 0; y < int(h); y++ {
			for x := 0; x < int(w); x++ {
				r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				off := y*stride + x*4
				pixels[off+0] = byte(bl >> 8)
				pixels[off+1] = byte(g >> 8)
				pixels[off+2] = byte(r >> 8)
				pixels[off+3] = byte(a >> 8)
			}
		}
		aboutLogoBitmap, aboutLogoW, aboutLogoH = hbm, w, h
	})
}

func aboutWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	dlg := aboutDlgFor(hwnd)
	if dlg == nil {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case win.WM_ERASEBKGND:
		// Erased here rather than left to DefWindowProc so that the colour
		// matches what onPaint fills with and the window never flashes the
		// wrong one.
		eraseBg(hwnd, wParam, dlg.background())
		return 1
	case win.WM_CTLCOLORSTATIC:
		if brush := handleCtlColor(hwnd, wParam, lParam, dlg.dark, dlg.bgBrush, 0); brush != 0 {
			return brush
		}
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	case win.WM_COMMAND:
		// The only control here is OK, and it does what the title bar's close
		// button and Escape do. IDOK and IDCANCEL are answered too: they are how
		// the dialog keyboard interface reports Enter, and a window that ignored
		// them would take Enter as a keystroke that does nothing.
		switch win.LOWORD(uint32(wParam)) {
		case ID_ABOUT_OK, win.IDOK, win.IDCANCEL:
			win.DestroyWindow(hwnd)
			return 0
		}
	case win.WM_PAINT:
		dlg.onPaint(hwnd)
		return 0
	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
	case win.WM_DESTROY:
		win.PostQuitMessage(0)
	case win.WM_NCDESTROY:
		// After WM_NCDESTROY nothing can paint with these any more, which is
		// not true during WM_DESTROY.
		dlg.release()
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

// release frees the GDI objects the window owns. The logo bitmap is not among
// them: it is decoded once and kept for the life of the process, because the
// About window can be opened again.
func (d *aboutDlg) release() {
	if d.titleFont != 0 {
		win.DeleteObject(win.HGDIOBJ(d.titleFont))
		d.titleFont = 0
	}
	if d.btnFont != 0 {
		win.DeleteObject(win.HGDIOBJ(d.btnFont))
		d.btnFont = 0
	}
	if d.bodyFont != 0 {
		win.DeleteObject(win.HGDIOBJ(d.bodyFont))
		d.bodyFont = 0
	}
	if d.bgBrush != 0 {
		win.DeleteObject(win.HGDIOBJ(d.bgBrush))
		d.bgBrush = 0
	}
}

// font falls back to the stock UI font when CreateFontIndirect came up empty,
// so a failure costs the right size rather than the whole window's text.
func (d *aboutDlg) font(f win.HFONT) win.HGDIOBJ {
	if f == 0 {
		return win.GetStockObject(win.DEFAULT_GUI_FONT)
	}
	return win.HGDIOBJ(f)
}

// background is the brush the window is filled with. FillRect accepts a system
// colour index plus one in place of a brush handle, which is what the light
// case relies on.
func (d *aboutDlg) background() win.HBRUSH {
	if d.dark {
		return d.bgBrush
	}
	return win.HBRUSH(win.COLOR_WINDOW + 1)
}

func (d *aboutDlg) onPaint(hwnd win.HWND) {
	var ps win.PAINTSTRUCT
	hdc := win.BeginPaint(hwnd, &ps)
	defer win.EndPaint(hwnd, &ps)

	var rc win.RECT
	win.GetClientRect(hwnd, &rc)
	cw := rc.Right - rc.Left
	fillRect(hdc, &rc, d.background())

	win.SetBkMode(hdc, win.TRANSPARENT)
	if d.dark {
		win.SetTextColor(hdc, win.COLORREF(0x00FFFFFF))
	} else {
		win.SetTextColor(hdc, win.COLORREF(0x00000000))
	}

	y := d.dp(aboutPad)
	if aboutLogoBitmap != 0 {
		w, h := d.dp(int(aboutLogoW)), d.dp(int(aboutLogoH))
		d.drawLogo(hdc, (cw-w)/2, y, w, h)
		y += h + d.dp(aboutLogoGap)
	}

	old := win.SelectObject(hdc, d.font(d.titleFont))
	defer win.SelectObject(hdc, old)

	d.text(hdc, "Nimbus", 0, y, cw, d.dp(aboutTitleH), win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE)
	y += d.dp(aboutTitleGap)

	win.SelectObject(hdc, d.font(d.bodyFont))
	pad := d.dp(aboutPad)
	d.text(hdc, build.Subtitle, pad, y, cw-pad, d.dp(aboutSubtitleH), win.DT_CENTER|win.DT_WORDBREAK)
	y += d.dp(aboutVerGap)

	// The version sits below the subtitle in a muted grey that reads on both
	// the light and the dark background this window can have.
	if d.dark {
		win.SetTextColor(hdc, win.COLORREF(0x00909090))
	} else {
		win.SetTextColor(hdc, win.COLORREF(0x00787878))
	}
	d.text(hdc, versionLine(), pad, y, cw-pad, d.dp(aboutVerH), win.DT_CENTER|win.DT_SINGLELINE)
}

// text draws one run inside a rectangle given in device pixels.
func (d *aboutDlg) text(hdc win.HDC, s string, left, top, right, height int32, format uint32) {
	t := utf16Of(s)
	rc := win.RECT{Left: left, Top: top, Right: right, Bottom: top + height}
	win.DrawTextEx(hdc, &t[0], -1, &rc, format, nil)
}

// drawLogo blends the logo onto the window, scaling it to the layout DPI.
//
// BitBlt with SRCCOPY - what this used to do - copies the colour channels and
// throws the alpha away, so every transparent pixel of the logo lands as
// whatever the colour channels happen to hold underneath it, which for a PNG
// is black. AlphaBlend is the call that reads the fourth channel, and it wants
// premultiplied source pixels, which is what initAboutLogo writes.
func (d *aboutDlg) drawLogo(hdc win.HDC, x, y, w, h int32) {
	memDC := win.CreateCompatibleDC(hdc)
	if memDC == 0 {
		return
	}
	defer win.DeleteDC(memDC)

	old := win.SelectObject(memDC, win.HGDIOBJ(aboutLogoBitmap))
	defer win.SelectObject(memDC, old)

	win.AlphaBlend(hdc, x, y, w, h, memDC, 0, 0, aboutLogoW, aboutLogoH,
		win.BLENDFUNCTION{
			BlendOp:             acSrcOver,
			SourceConstantAlpha: 255,
			AlphaFormat:         win.AC_SRC_ALPHA,
		})
}
