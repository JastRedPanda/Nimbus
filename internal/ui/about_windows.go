//go:build windows

package ui

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

type aboutDlg struct {
	hwnd    win.HWND
	inst    win.HINSTANCE
	hicon   win.HANDLE
	dark    bool
	bgBrush win.HBRUSH
}

var (
	aboutClassOnce sync.Once
	aboutClassOK   bool
)

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
		}
	})

	if !aboutClassOK {
		return
	}

	d.hicon = win.LoadImage(d.inst, win.MAKEINTRESOURCE(1), win.IMAGE_ICON, 64, 64, win.LR_DEFAULTCOLOR)

	const winW, winH = 320, 240
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

	iconY := int32(24)
	iconSize := int32(64)
	iconX := (cw - iconSize) / 2
	if d.hicon != 0 {
		win.DrawIconEx(hdc, iconX, iconY, win.HICON(d.hicon), iconSize, iconSize, 0, 0, win.DI_NORMAL)
	}

	titleY := iconY + iconSize + 16
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
	fontH := -15
	lf2 := &win.LOGFONT{
		LfHeight:        int32(fontH),
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
