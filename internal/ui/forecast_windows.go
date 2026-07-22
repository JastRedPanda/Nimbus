//go:build windows

package ui

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/JastRedPanda/Nimbus/internal/fonts"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/lxn/win"
)

type fcsDlg struct {
	hwnd     win.HWND
	inst     win.HINSTANCE
	data     []weather.DailyForecast
	lang     string
	units    string
	windUnit string
	dark     bool
	bgBrush  win.HBRUSH
	edBrush  win.HBRUSH
}

var (
	forecastClassOnce sync.Once
	forecastClassOK   bool
)

func ShowForecast(lat, lon float64, units, lang, theme, windUnit string) {
	data, err := weather.FetchDaily(lat, lon)
	if err != nil || len(data) == 0 {
		showError("Failed to load forecast data.")
		return
	}
	dark := theme == "dark"
	d := &fcsDlg{data: data, lang: lang, units: units, windUnit: windUnit, dark: dark}
	if dark {
		d.bgBrush = createDarkBrush()
		d.edBrush = createEditBrush()
	}
	go d.run()
}

func showError(msg string) {
	title := syscall.StringToUTF16("Nimbus")
	t := syscall.StringToUTF16(msg)
	win.MessageBox(0, &t[0], &title[0], win.MB_OK|win.MB_ICONERROR)
}

func (d *fcsDlg) run() {
	d.inst = win.GetModuleHandle(nil)
	if d.inst == 0 {
		return
	}

	// Register window class once
	forecastClassOnce.Do(func() {
		cn := syscall.StringToUTF16("NimbusForecastClass")
		wc := &win.WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			Style:         win.CS_HREDRAW | win.CS_VREDRAW,
			LpfnWndProc:   syscall.NewCallback(forecastWndProc),
			HInstance:     d.inst,
			HIcon:         win.LoadIcon(0, win.MAKEINTRESOURCE(win.IDI_APPLICATION)),
			HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			HbrBackground: win.COLOR_BTNFACE + 1,
			LpszClassName: &cn[0],
		}
		if win.RegisterClassEx(wc) != 0 {
			forecastClassOK = true
		}
	})

	if !forecastClassOK {
		return
	}

	lang := i18n.ParseLang(d.lang)
	title := syscall.StringToUTF16(lang.ForecastTitle())
	d.lang = lang.String()

	fonts.Load()

	d.hwnd = win.CreateWindowEx(
		0, syscall.StringToUTF16Ptr("NimbusForecastClass"), &title[0],
		win.WS_CAPTION|win.WS_SYSMENU|win.WS_THICKFRAME|win.WS_MAXIMIZEBOX|win.WS_VISIBLE,
		win.CW_USEDEFAULT, win.CW_USEDEFAULT, 640, 480, 0, 0, d.inst, unsafe.Pointer(d),
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

func forecastWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_NCCREATE {
		cs := (*win.CREATESTRUCT)(unsafe.Pointer(lParam))
		win.SetWindowLongPtr(hwnd, win.GWLP_USERDATA, uintptr(cs.CreateParams))
		return 1
	}
	ptr := win.GetWindowLongPtr(hwnd, win.GWLP_USERDATA)
	if ptr == 0 {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	dlg := (*fcsDlg)(unsafe.Pointer(ptr))

	switch msg {
	case win.WM_ERASEBKGND:
		if dlg.dark {
			eraseBg(hwnd, wParam, dlg.bgBrush)
			return 1
		}
		return 0
	case win.WM_CTLCOLORSTATIC, win.WM_CTLCOLORBTN, win.WM_CTLCOLOREDIT, win.WM_CTLCOLORLISTBOX:
		return handleCtlColor(hwnd, wParam, lParam, dlg.dark, dlg.bgBrush, dlg.edBrush)
	case win.WM_SIZE:
		win.RedrawWindow(hwnd, nil, 0, win.RDW_INVALIDATE|win.RDW_ERASE|win.RDW_UPDATENOW)
	case win.WM_PAINT:
		dlg.onPaint(hwnd)
	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
	case win.WM_DESTROY:
		if dlg.dark {
			win.DeleteObject(win.HGDIOBJ(dlg.bgBrush))
			win.DeleteObject(win.HGDIOBJ(dlg.edBrush))
		}
		win.PostQuitMessage(0)
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

func (d *fcsDlg) onPaint(hwnd win.HWND) {
	var ps win.PAINTSTRUCT
	hdc := win.BeginPaint(hwnd, &ps)
	defer win.EndPaint(hwnd, &ps)

	var rc win.RECT
	win.GetClientRect(hwnd, &rc)
	cw := int32(rc.Right - rc.Left)
	ch := int32(rc.Bottom - rc.Top)

	// fill background in both modes
	bg := win.HBRUSH(win.COLOR_WINDOW + 1)
	if d.dark {
		bg = d.bgBrush
	}
	fillRect(hdc, &rc, bg)

	lang := i18n.ParseLang(d.lang)
	headers := lang.ForecastHeaders()

	nCol := int32(len(headers))

	margin := int32(8)
	colW := (cw - margin*2) / nCol

	colX := make([]int32, nCol)
	for i := range colX {
		colX[i] = margin + colW*int32(i)
	}

	headerH := int32(24)
	rowH := (ch - headerH - 10) / max(int32(len(d.data)), 1)
	if rowH < 24 {
		rowH = 24
	}
	if rowH > 100 {
		rowH = 100
	}

	txtColor := uint32(0x000000)
	if d.dark {
		txtColor = 0x00FFFFFF
	}
	win.SetTextColor(hdc, win.COLORREF(txtColor))
	win.SetBkMode(hdc, win.TRANSPARENT)

	// create a font for data rows proportional to row height
	dataFontH := int32(float64(rowH) * 0.45)
	if dataFontH < 9 {
		dataFontH = 9
	}
	if dataFontH > 20 {
		dataFontH = 20
	}
	lf := &win.LOGFONT{
		LfHeight:        -dataFontH,
		LfWeight:        win.FW_NORMAL,
		LfCharSet:       win.DEFAULT_CHARSET,
		LfOutPrecision:  win.OUT_DEFAULT_PRECIS,
		LfClipPrecision: win.CLIP_DEFAULT_PRECIS,
		LfQuality:       win.CLEARTYPE_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	dataFont := win.HGDIOBJ(win.CreateFontIndirect(lf))
	if dataFont == 0 {
		dataFont = win.GetStockObject(win.DEFAULT_GUI_FONT)
	}

	// header uses default GUI font
	win.SelectObject(hdc, win.GetStockObject(win.DEFAULT_GUI_FONT))
	for i, h := range headers {
		r := &win.RECT{Left: colX[i], Top: 6, Right: colX[i] + colW, Bottom: 6 + headerH}
		t := syscall.StringToUTF16(h)
		win.DrawTextEx(hdc, &t[0], -1, r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)
	}

	// switch to data font for rows
	win.SelectObject(hdc, win.HGDIOBJ(dataFont))

	// weather icons font
	wiH := int(rowH * 7 / 10)
	if wiH < 14 {
		wiH = 14
	}
	if wiH > 48 {
		wiH = 48
	}
	wiFace := "Weather Icons"
	wfl := &win.LOGFONT{
		LfHeight:        int32(-wiH),
		LfWeight:        win.FW_NORMAL,
		LfCharSet:       win.DEFAULT_CHARSET,
		LfOutPrecision:  win.OUT_DEFAULT_PRECIS,
		LfClipPrecision: win.CLIP_DEFAULT_PRECIS,
		LfQuality:       win.DEFAULT_QUALITY,
		LfPitchAndFamily: win.DEFAULT_PITCH | win.FF_DONTCARE,
	}
	copy(wfl.LfFaceName[:], syscall.StringToUTF16(wiFace))
	wiFont := win.CreateFontIndirect(wfl)
	if wiFont != 0 {
		defer win.DeleteObject(win.HGDIOBJ(wiFont))
	}

	for i, day := range d.data {
		y := int32(10) + headerH + rowH*int32(i)
		if y+rowH > ch {
			break
		}

		// date
		r := &win.RECT{Left: colX[0], Top: y, Right: colX[0] + colW, Bottom: y + rowH}
		t := syscall.StringToUTF16(day.Date)
		win.DrawTextEx(hdc, &t[0], -1, r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)

		// condition: weather icons font character
		iconChar := fonts.IconForCode(day.WeatherCode)
		if wiFont != 0 {
			win.SelectObject(hdc, win.HGDIOBJ(wiFont))
		}
		r2 := &win.RECT{Left: colX[1], Top: y, Right: colX[1] + colW, Bottom: y + rowH}
		iconText := syscall.StringToUTF16(iconChar)
		win.DrawTextEx(hdc, &iconText[0], -1, r2, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)
		if wiFont != 0 {
			win.SelectObject(hdc, win.HGDIOBJ(dataFont))
		}

		// temp
		r = &win.RECT{Left: colX[2], Top: y, Right: colX[2] + colW, Bottom: y + rowH}
		tmp := fmt.Sprintf("%+.0f/%+.0f%s", day.TempMax, day.TempMin, lang.TempUnit(d.units))
		t2 := syscall.StringToUTF16(tmp)
		win.DrawTextEx(hdc, &t2[0], -1, r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)

		// wind
		r = &win.RECT{Left: colX[3], Top: y, Right: colX[3] + colW, Bottom: y + rowH}
		ws := day.WindMax
		if d.windUnit == "ms" {
			ws = ws / 3.6
		}
		wind := fmt.Sprintf("%.1f %s", ws, lang.WindUnitCfg(d.windUnit))
		t3 := syscall.StringToUTF16(wind)
		win.DrawTextEx(hdc, &t3[0], -1, r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)

		// precip
		r = &win.RECT{Left: colX[4], Top: y, Right: colX[4] + colW, Bottom: y + rowH}
		precip := fmt.Sprintf("%.1f %s", day.PrecipSum, lang.PrecipUnit())
		t4 := syscall.StringToUTF16(precip)
		win.DrawTextEx(hdc, &t4[0], -1, r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)

		// separator
		sepY := y + rowH - 1
		win.MoveToEx(hdc, int(margin), int(sepY), nil)
		win.LineTo(hdc, cw-margin, sepY)
	}

	defaultFont := win.GetStockObject(win.DEFAULT_GUI_FONT)
	if dataFont != defaultFont {
		win.DeleteObject(dataFont)
	}
}

