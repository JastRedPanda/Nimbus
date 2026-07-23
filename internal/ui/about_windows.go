//go:build windows

package ui

import (
	"bytes"
	_ "embed"
	"image"
	_ "image/png"
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

//go:embed about_logo.png
var aboutLogoPNG []byte

var aboutLogoBitmap win.HBITMAP
var aboutLogoW, aboutLogoH int32

type aboutDlg struct {
	hwnd    win.HWND
	inst    win.HINSTANCE
	dark    bool
	bgBrush win.HBRUSH
}

var (
	aboutClassOnce sync.Once
	aboutClassOK   bool
	aboutClassInst win.HINSTANCE
)

func initAboutLogo() {
	img, _, err := image.Decode(bytes.NewReader(aboutLogoPNG))
	if err != nil {
		return
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	aboutLogoW = int32(w)
	aboutLogoH = int32(h)

	bmi := &win.BITMAPINFOHEADER{
		BiSize:     uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
		BiWidth:    int32(w),
		BiHeight:   -int32(h),
		BiPlanes:   1,
		BiBitCount: 32,
		BiCompression: win.BI_RGB,
	}

	var bits unsafe.Pointer
	hbm := win.CreateDIBSection(0, bmi, win.DIB_RGB_COLORS, &bits, 0, 0)
	if hbm == 0 || bits == nil {
		return
	}

	pixels := (*[1 << 30]byte)(bits)
	stride := w * 4
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			off := y*stride + x*4
			pixels[off+0] = byte(b >> 8)
			pixels[off+1] = byte(g >> 8)
			pixels[off+2] = byte(r >> 8)
			pixels[off+3] = byte(a >> 8)
		}
	}
	aboutLogoBitmap = hbm
}

func ShowAbout(theme string) {
	dark := theme == "dark"
	d := &aboutDlg{dark: dark}
	if dark {
		d.bgBrush = createDarkBrush()
	}
	go d.run()
}

func (d *aboutDlg) run() {
	d.inst = win.GetModuleHandle(nil)
	if d.inst == 0 {
		return
	}

	aboutClassOnce.Do(func() {
		cn := syscall.StringToUTF16("NimbusAboutClass")
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
		if win.RegisterClassEx(wc) != 0 {
			aboutClassOK = true
			aboutClassInst = d.inst
		}
	})

	if !aboutClassOK {
		return
	}

	if aboutLogoBitmap == 0 {
		initAboutLogo()
	}

	const winW, winH = 320, 260
	d.hwnd = win.CreateWindowEx(
		0, syscall.StringToUTF16Ptr("NimbusAboutClass"), syscall.StringToUTF16Ptr("About Nimbus"),
		win.WS_CAPTION|win.WS_SYSMENU|win.WS_VISIBLE,
		win.CW_USEDEFAULT, win.CW_USEDEFAULT, winW, winH, 0, 0, d.inst, unsafe.Pointer(d),
	)
	if d.hwnd == 0 {
		return
	}

	if d.dark {
		setDarkTitleBar(d.hwnd, true)
	}
	win.ShowWindow(d.hwnd, win.SW_SHOW)
	win.UpdateWindow(d.hwnd)

	var msg win.MSG
	for win.GetMessage(&msg, 0, 0, 0) != 0 {
		if !win.IsDialogMessage(d.hwnd, &msg) {
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}
	}
}

func aboutWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_NCCREATE {
		cs := (*win.CREATESTRUCT)(unsafe.Pointer(lParam))
		win.SetWindowLongPtr(hwnd, win.GWLP_USERDATA, uintptr(cs.CreateParams))
		return 1
	}
	ptr := win.GetWindowLongPtr(hwnd, win.GWLP_USERDATA)
	if ptr == 0 {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	dlg := (*aboutDlg)(unsafe.Pointer(ptr))

	switch msg {
	case win.WM_ERASEBKGND:
		if dlg.dark {
			eraseBg(hwnd, wParam, dlg.bgBrush)
			return 1
		}
		return 0
	case win.WM_CTLCOLORSTATIC:
		return handleCtlColor(hwnd, wParam, lParam, dlg.dark, dlg.bgBrush, 0)
	case win.WM_PAINT:
		dlg.onPaint(hwnd)
	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
	case win.WM_DESTROY:
		if dlg.dark {
			win.DeleteObject(win.HGDIOBJ(dlg.bgBrush))
		}
		win.PostQuitMessage(0)
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func (d *aboutDlg) onPaint(hwnd win.HWND) {
	var ps win.PAINTSTRUCT
	hdc := win.BeginPaint(hwnd, &ps)
	defer win.EndPaint(hwnd, &ps)

	var rc win.RECT
	win.GetClientRect(hwnd, &rc)
	cw := rc.Right - rc.Left

	bg := win.HBRUSH(win.COLOR_WINDOW + 1)
	if d.dark {
		bg = d.bgBrush
	}
	fillRect(hdc, &rc, bg)

	txtColor := uint32(0x000000)
	if d.dark {
		txtColor = 0x00FFFFFF
	}
	win.SetTextColor(hdc, win.COLORREF(txtColor))
	win.SetBkMode(hdc, win.TRANSPARENT)

	if aboutLogoBitmap != 0 {
		memDC := win.CreateCompatibleDC(hdc)
		if memDC != 0 {
			old := win.SelectObject(memDC, win.HGDIOBJ(aboutLogoBitmap))
			imgX := (cw - aboutLogoW) / 2
			win.BitBlt(hdc, imgX, 20, aboutLogoW, aboutLogoH, memDC, 0, 0, win.SRCCOPY)
			win.SelectObject(memDC, old)
			win.DeleteDC(memDC)
		}
	}

	titleY := int32(20) + aboutLogoH + 16
	lf := &win.LOGFONT{
		LfHeight:        -24,
		LfWeight:        win.FW_BOLD,
		LfCharSet:       win.DEFAULT_CHARSET,
		LfOutPrecision:  win.OUT_DEFAULT_PRECIS,
		LfClipPrecision: win.CLIP_DEFAULT_PRECIS,
		LfQuality:       win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	titleFont := win.CreateFontIndirect(lf)
	if titleFont != 0 {
		win.SelectObject(hdc, win.HGDIOBJ(titleFont))
	}
	titleText := syscall.StringToUTF16("Nimbus")
	tr := &win.RECT{Left: 0, Top: titleY, Right: cw, Bottom: titleY + 32}
	win.DrawTextEx(hdc, &titleText[0], -1, tr, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)
	if titleFont != 0 {
		win.SelectObject(hdc, win.GetStockObject(win.DEFAULT_GUI_FONT))
		win.DeleteObject(win.HGDIOBJ(titleFont))
	}

	subY := titleY + 36
	lf2 := &win.LOGFONT{
		LfHeight:        -15,
		LfWeight:        win.FW_NORMAL,
		LfCharSet:       win.DEFAULT_CHARSET,
		LfOutPrecision:  win.OUT_DEFAULT_PRECIS,
		LfClipPrecision: win.CLIP_DEFAULT_PRECIS,
		LfQuality:       win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	subFont := win.CreateFontIndirect(lf2)
	if subFont != 0 {
		win.SelectObject(hdc, win.HGDIOBJ(subFont))
	}
	subText := syscall.StringToUTF16("Мультиплатформний інформер погоди.")
	sr := &win.RECT{Left: 20, Top: subY, Right: cw - 20, Bottom: subY + 40}
	win.DrawTextEx(hdc, &subText[0], -1, sr, win.DT_CENTER|win.DT_WORDBREAK, nil)
	if subFont != 0 {
		win.DeleteObject(win.HGDIOBJ(subFont))
	}
}
