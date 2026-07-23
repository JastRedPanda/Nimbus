//go:build windows

package ui

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/lxn/win"
)

const (
	ID_CITY_EDIT         = 101
	ID_SEARCH            = 102
	ID_CITY_LIST         = 103
	ID_LAT_EDIT          = 104
	ID_LON_EDIT          = 105
	ID_TEMP_C            = 106
	ID_TEMP_F            = 107
	ID_PRES_H            = 108
	ID_PRES_M            = 109
	ID_PRES_I            = 110
	ID_THEME_A           = 111
	ID_THEME_D           = 112
	ID_THEME_L           = 113
	ID_LANG_EN           = 114
	ID_LANG_UK           = 115
	ID_SAVE              = 116
	ID_CANCEL            = 117
	ID_WIND_MS           = 118
	ID_WIND_KMH          = 119
	ID_FONT_SCALE        = 120
	ID_DEL_CFG           = 121
	ID_INTERVAL          = 122
	WM_APP_SEARCH_RESULT = win.WM_APP + 1

	TBM_CLEARTICS  = win.WM_USER + 9
	TBM_SETTICFREQ = win.WM_USER + 20
	TBM_SETBKCOLOR = win.WM_USER + 30
)

var (
	settingsClassOnce sync.Once
	settingsClassOK   bool
)

var intervals = []struct {
	minutes int
	label   string
}{
	{5, "5 min"},
	{30, "30 min"},
	{60, "1 hour"},
	{720, "12 hours"},
	{1440, "24 hours"},
}

type setDlg struct {
	hwnd           win.HWND
	inst           win.HINSTANCE
	cfg            *config.Config
	lang           i18n.Lang
	results        []weather.GeoResult
	result         chan *config.Config
	dark           bool
	bgBrush        win.HBRUSH
	edBrush        win.HBRUSH
	onFontChange   func(int)
	fontScaleLabel win.HWND
}

func ShowSettings(cfg *config.Config, onFontChange func(int)) *config.Config {
	dark := cfg.IconTheme == "dark"
	d := &setDlg{cfg: cfg, lang: i18n.ParseLang(cfg.Language), result: make(chan *config.Config, 1), dark: dark, onFontChange: onFontChange}
	if dark {
		d.bgBrush = createDarkBrush()
		d.edBrush = createEditBrush()
	}
	go d.run()
	return <-d.result
}

func initCommon() {
	var ice win.INITCOMMONCONTROLSEX
	ice.DwSize = uint32(unsafe.Sizeof(ice))
	ice.DwICC = win.ICC_STANDARD_CLASSES
	win.InitCommonControlsEx(&ice)
}

func (d *setDlg) run() {
	d.inst = win.GetModuleHandle(nil)
	if d.inst == 0 {
		d.result <- nil
		return
	}

	// Register window class once
	settingsClassOnce.Do(func() {
		cn := syscall.StringToUTF16("NimbusSettingsClass")
		wc := &win.WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			Style:         win.CS_HREDRAW | win.CS_VREDRAW,
			LpfnWndProc:   syscall.NewCallback(d.wndProc),
			HInstance:     d.inst,
			HIcon:         win.LoadIcon(d.inst, win.MAKEINTRESOURCE(1)),
			HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			HbrBackground: win.COLOR_BTNFACE + 1,
			LpszClassName: &cn[0],
		}
		if win.RegisterClassEx(wc) != 0 {
			settingsClassOK = true
		}
	})

	if !settingsClassOK {
		d.result <- nil
		return
	}

	initCommon()

	contentH := 700
	var rect win.RECT
	rect.Left = 0
	rect.Top = 0
	rect.Right = 440
	rect.Bottom = int32(contentH)
	win.AdjustWindowRect(&rect, win.WS_CAPTION|win.WS_SYSMENU, false)
	winW := int(rect.Right - rect.Left)
	winH := int(rect.Bottom - rect.Top)

	title := syscall.StringToUTF16(d.lang.SettingsTitle())
	d.hwnd = win.CreateWindowEx(
		0, syscall.StringToUTF16Ptr("NimbusSettingsClass"), &title[0],
		win.WS_CAPTION|win.WS_SYSMENU|win.WS_VISIBLE,
		win.CW_USEDEFAULT, win.CW_USEDEFAULT, int32(winW), int32(winH),
		0, 0, d.inst, unsafe.Pointer(d),
	)
	if d.hwnd == 0 {
		d.result <- nil
		return
	}

	d.createControls()
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

func (d *setDlg) createControls() {
	y := 10

	createStatic(d.hwnd, d.lang.CityLabel(), 12, y+4, 70, 20)
	createEdit(d.hwnd, d.cfg.CityName, 86, y, 200, 24, ID_CITY_EDIT)
	y += 30

	createButton(d.hwnd, d.lang.SearchBtn(), 294, y+2, 70, 24, ID_SEARCH)
	y += 30
	createListBox(d.hwnd, 86, y, 278, 100, ID_CITY_LIST)
	y += 108

	createStatic(d.hwnd, d.lang.LatLabel(), 12, y+4, 70, 20)
	createEdit(d.hwnd, fmt.Sprintf("%.4f", d.cfg.Latitude), 86, y, 120, 24, ID_LAT_EDIT)
	y += 30

	createStatic(d.hwnd, d.lang.LonLabel(), 12, y+4, 70, 20)
	createEdit(d.hwnd, fmt.Sprintf("%.4f", d.cfg.Longitude), 86, y, 120, 24, ID_LON_EDIT)
	y += 40

	createGroup(d.hwnd, d.lang.TemperatureGroup(), 12, y, 150, 48)
	createRadio(d.hwnd, "\u00B0C", 22, y+18, 60, 22, ID_TEMP_C, d.cfg.Units == "celsius", true)
	createRadio(d.hwnd, "\u00B0F", 82, y+18, 60, 22, ID_TEMP_F, d.cfg.Units == "fahrenheit", false)
	y += 56

	createGroup(d.hwnd, d.lang.PressureGroup(), 12, y, 280, 48)
	createRadio(d.hwnd, d.lang.HPa(), 22, y+18, 60, 22, ID_PRES_H, d.cfg.PressureUnit == "hpa", true)
	createRadio(d.hwnd, d.lang.MmHg(), 90, y+18, 70, 22, ID_PRES_M, d.cfg.PressureUnit == "mmhg", false)
	createRadio(d.hwnd, d.lang.InHg(), 170, y+18, 110, 22, ID_PRES_I, d.cfg.PressureUnit == "inhg", false)
	y += 56

	createGroup(d.hwnd, d.lang.WindGroup(), 12, y, 180, 48)
	createRadio(d.hwnd, d.lang.WindMS(), 22, y+18, 60, 22, ID_WIND_MS, d.cfg.WindUnit == "ms", true)
	createRadio(d.hwnd, d.lang.WindKMH(), 90, y+18, 70, 22, ID_WIND_KMH, d.cfg.WindUnit == "kmh", false)
	y += 56

	createGroup(d.hwnd, d.lang.ThemeGroup(), 12, y, 280, 48)
	createRadio(d.hwnd, d.lang.ThemeAuto(), 22, y+18, 60, 22, ID_THEME_A, d.cfg.IconTheme == "auto", true)
	createRadio(d.hwnd, d.lang.ThemeDark(), 90, y+18, 60, 22, ID_THEME_D, d.cfg.IconTheme == "dark", false)
	createRadio(d.hwnd, d.lang.ThemeLight(), 170, y+18, 60, 22, ID_THEME_L, d.cfg.IconTheme == "light", false)
	y += 56

	createGroup(d.hwnd, d.lang.LanguageGroup(), 12, y, 270, 48)
	createRadio(d.hwnd, "English", 22, y+18, 70, 22, ID_LANG_EN, d.cfg.Language == "en", true)
	createRadio(d.hwnd, "Українська", 100, y+18, 150, 22, ID_LANG_UK, d.cfg.Language == "uk", false)
	y += 56

	createGroup(d.hwnd, d.lang.FontScaleGroup(), 12, y, 340, 48)
	d.createSlider(86, y+14, 180, 24, ID_FONT_SCALE, d.cfg.FontScale, 1, 100)
	d.fontScaleLabel = createStatic(d.hwnd, fmt.Sprintf("%d%%", d.cfg.FontScale), 272, y+16, 40, 20)
	y += 56

	createGroup(d.hwnd, d.lang.UpdateInterval(), 12, y, 340, 48)
	d.createCombo(86, y+14, 180, 200, ID_INTERVAL, d.cfg.UpdateInterval)
	y += 66

	createButton(d.hwnd, d.lang.SaveBtn(), 60, y, 90, 28, ID_SAVE)
	createButton(d.hwnd, d.lang.CancelBtn(), 160, y, 90, 28, ID_CANCEL)
	createButton(d.hwnd, d.lang.DeleteCfgBtn(), 260, y, 130, 28, ID_DEL_CFG)
}

func (d *setDlg) createSlider(x, y, w, h int, id int32, pos, min, max int) {
	cls := syscall.StringToUTF16Ptr("msctls_trackbar32")
	hwnd := win.CreateWindowEx(0, cls, nil,
		win.WS_CHILD|win.WS_VISIBLE|win.WS_TABSTOP,
		int32(x), int32(y), int32(w), int32(h),
		d.hwnd, win.HMENU(id), 0, nil)
	if hwnd != 0 {
		win.SendMessage(hwnd, win.TBM_SETRANGEMIN, 0, uintptr(min))
		win.SendMessage(hwnd, win.TBM_SETRANGEMAX, 0, uintptr(max))
		win.SendMessage(hwnd, win.TBM_SETPOS, 1, uintptr(pos))
		win.SendMessage(hwnd, TBM_CLEARTICS, 1, 0)
		win.SendMessage(hwnd, TBM_SETTICFREQ, 25, 0)
		if d.dark {
			win.SendMessage(hwnd, TBM_SETBKCOLOR, 0, uintptr(win.RGB(45, 45, 45)))
		}
	}
}

func (d *setDlg) createCombo(x, y, w, dropH int, id int32, curMinutes int) {
	cls := syscall.StringToUTF16Ptr("COMBOBOX")
	hwnd := win.CreateWindowEx(0, cls, nil,
		win.WS_CHILD|win.WS_VISIBLE|win.WS_VSCROLL|win.CBS_DROPDOWNLIST|win.WS_TABSTOP,
		int32(x), int32(y), int32(w), int32(dropH),
		d.hwnd, win.HMENU(id), 0, nil)
	if hwnd != 0 {
		sel := 0
		for i, iv := range intervals {
			lb := syscall.StringToUTF16(iv.label)
			win.SendMessage(hwnd, win.CB_ADDSTRING, 0, uintptr(unsafe.Pointer(&lb[0])))
			if iv.minutes == curMinutes {
				sel = i
			}
		}
		win.SendMessage(hwnd, win.CB_SETCURSEL, uintptr(sel), 0)
	}
}

func (d *setDlg) getSlider(id int32) int {
	hwnd := win.GetDlgItem(d.hwnd, id)
	return int(win.SendMessage(hwnd, win.TBM_GETPOS, 0, 0))
}

func (d *setDlg) getComboSel(id int32) int {
	hwnd := win.GetDlgItem(d.hwnd, id)
	return int(win.SendMessage(hwnd, win.CB_GETCURSEL, 0, 0))
}

func (d *setDlg) wndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == win.WM_NCCREATE {
		cs := (*win.CREATESTRUCT)(unsafe.Pointer(lParam))
		win.SetWindowLongPtr(hwnd, win.GWLP_USERDATA, uintptr(cs.CreateParams))
		return 1
	}
	ptr := win.GetWindowLongPtr(hwnd, win.GWLP_USERDATA)
	if ptr == 0 {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	dlg := (*setDlg)(unsafe.Pointer(ptr))

	switch msg {
	case win.WM_ERASEBKGND:
		if dlg.dark {
			eraseBg(hwnd, wParam, dlg.bgBrush)
			return 1
		}
		return 0
	case win.WM_CTLCOLORSTATIC, win.WM_CTLCOLORBTN, win.WM_CTLCOLOREDIT, win.WM_CTLCOLORLISTBOX:
		if !dlg.dark {
			return win.DefWindowProc(hwnd, msg, wParam, lParam)
		}
		return handleCtlColor(hwnd, wParam, lParam, dlg.dark, dlg.bgBrush, dlg.edBrush)
	case win.WM_HSCROLL:
		code := win.LOWORD(uint32(wParam))
		fs := dlg.getSlider(ID_FONT_SCALE)
		if dlg.fontScaleLabel != 0 {
			t := syscall.StringToUTF16(fmt.Sprintf("%d%%", fs))
			win.SendMessage(dlg.fontScaleLabel, win.WM_SETTEXT, 0, uintptr(unsafe.Pointer(&t[0])))
		}
		if code != win.SB_THUMBTRACK && dlg.onFontChange != nil {
			dlg.onFontChange(fs)
		}
	case win.WM_COMMAND:
		low := win.LOWORD(uint32(wParam))
		switch low {
		case ID_SEARCH:
			dlg.onSearch()
		case ID_CITY_LIST:
			if win.HIWORD(uint32(wParam)) == win.LBN_SELCHANGE {
				dlg.onCitySelect()
			}
		case ID_SAVE:
			dlg.onSave()
		case ID_CANCEL:
			win.DestroyWindow(hwnd)
		case ID_DEL_CFG:
			dlg.onDeleteCfg()
		}
	case WM_APP_SEARCH_RESULT:
		dlg.populateList()
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

func (d *setDlg) onSearch() {
	buf := make([]uint16, 256)
	win.SendMessage(win.GetDlgItem(d.hwnd, ID_CITY_EDIT), win.WM_GETTEXT, 256, uintptr(unsafe.Pointer(&buf[0])))
	query := syscall.UTF16ToString(buf)
	if query == "" {
		return
	}
	go func() {
		res, err := weather.SearchCity(query, d.lang.String())
		if err != nil || len(res) == 0 {
			hList := win.GetDlgItem(d.hwnd, ID_CITY_LIST)
			no := syscall.StringToUTF16(d.lang.NoResults())
			win.SendMessage(hList, win.LB_RESETCONTENT, 0, 0)
			win.SendMessage(hList, win.LB_ADDSTRING, 0, uintptr(unsafe.Pointer(&no[0])))
			return
		}
		d.results = res
		win.PostMessage(d.hwnd, WM_APP_SEARCH_RESULT, 0, 0)
	}()
}

func (d *setDlg) populateList() {
	hList := win.GetDlgItem(d.hwnd, ID_CITY_LIST)
	win.SendMessage(hList, win.LB_RESETCONTENT, 0, 0)
	for _, r := range d.results {
		text := r.Name + ", " + r.Country + " | " + fmt.Sprintf("%.4f,%.4f", r.Latitude, r.Longitude)
		t := syscall.StringToUTF16(text)
		win.SendMessage(hList, win.LB_ADDSTRING, 0, uintptr(unsafe.Pointer(&t[0])))
	}
}

func (d *setDlg) onCitySelect() {
	hList := win.GetDlgItem(d.hwnd, ID_CITY_LIST)
	sel := int32(win.SendMessage(hList, win.LB_GETCURSEL, 0, 0))
	if sel < 0 || sel >= int32(len(d.results)) {
		return
	}
	r := d.results[sel]
	setText(d.hwnd, ID_CITY_EDIT, r.Name)
	setText(d.hwnd, ID_LAT_EDIT, fmt.Sprintf("%.4f", r.Latitude))
	setText(d.hwnd, ID_LON_EDIT, fmt.Sprintf("%.4f", r.Longitude))
}

func (d *setDlg) getText(id int32) string {
	buf := make([]uint16, 256)
	win.SendMessage(win.GetDlgItem(d.hwnd, id), win.WM_GETTEXT, 256, uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

func (d *setDlg) isChecked(id int32) bool {
	return win.SendMessage(win.GetDlgItem(d.hwnd, id), win.BM_GETCHECK, 0, 0) == 1
}

func (d *setDlg) onDeleteCfg() {
	config.Delete()
	d.result <- config.Default()
	win.DestroyWindow(d.hwnd)
}

func (d *setDlg) onSave() {
	nc := *d.cfg
	nc.CityName = d.getText(ID_CITY_EDIT)
	fmt.Sscanf(d.getText(ID_LAT_EDIT), "%f", &nc.Latitude)
	fmt.Sscanf(d.getText(ID_LON_EDIT), "%f", &nc.Longitude)

	if d.isChecked(ID_TEMP_F) {
		nc.Units = "fahrenheit"
	} else {
		nc.Units = "celsius"
	}
	if d.isChecked(ID_PRES_M) {
		nc.PressureUnit = "mmhg"
	} else if d.isChecked(ID_PRES_I) {
		nc.PressureUnit = "inhg"
	} else {
		nc.PressureUnit = "hpa"
	}
	if d.isChecked(ID_THEME_D) {
		nc.IconTheme = "dark"
	} else if d.isChecked(ID_THEME_L) {
		nc.IconTheme = "light"
	} else {
		nc.IconTheme = "auto"
	}
	if d.isChecked(ID_LANG_UK) {
		nc.Language = "uk"
	} else {
		nc.Language = "en"
	}
	if d.isChecked(ID_WIND_KMH) {
		nc.WindUnit = "kmh"
	} else {
		nc.WindUnit = "ms"
	}
	nc.FontScale = d.getSlider(ID_FONT_SCALE)

	sel := d.getComboSel(ID_INTERVAL)
	if sel >= 0 && sel < len(intervals) {
		nc.UpdateInterval = intervals[sel].minutes
	}

	nc.Save()
	d.result <- &nc
	win.DestroyWindow(d.hwnd)
}

func createStatic(parent win.HWND, text string, x, y, w, h int) win.HWND {
	t := syscall.StringToUTF16(text)
	return win.CreateWindowEx(0, syscall.StringToUTF16Ptr("STATIC"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.SS_LEFT,
		int32(x), int32(y), int32(w), int32(h),
		parent, 0, 0, nil)
}

func createEdit(parent win.HWND, text string, x, y, w, h int, id int32) win.HWND {
	t := syscall.StringToUTF16(text)
	return win.CreateWindowEx(win.WS_EX_CLIENTEDGE, syscall.StringToUTF16Ptr("EDIT"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.ES_LEFT|win.ES_AUTOHSCROLL,
		int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
}

func createButton(parent win.HWND, text string, x, y, w, h int, id int32) win.HWND {
	t := syscall.StringToUTF16(text)
	return win.CreateWindowEx(0, syscall.StringToUTF16Ptr("BUTTON"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.BS_PUSHBUTTON|win.WS_TABSTOP,
		int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
}

func createGroup(parent win.HWND, text string, x, y, w, h int) win.HWND {
	t := syscall.StringToUTF16(text)
	return win.CreateWindowEx(0, syscall.StringToUTF16Ptr("BUTTON"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.BS_GROUPBOX,
		int32(x), int32(y), int32(w), int32(h),
		parent, 0, 0, nil)
}

func createRadio(parent win.HWND, text string, x, y, w, h int, id int32, checked, first bool) win.HWND {
	style := uint32(win.WS_CHILD | win.WS_VISIBLE | win.BS_AUTORADIOBUTTON | win.WS_TABSTOP)
	if first {
		style |= win.WS_GROUP
	}
	t := syscall.StringToUTF16(text)
	hwnd := win.CreateWindowEx(0, syscall.StringToUTF16Ptr("BUTTON"), &t[0],
		style, int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
	if checked {
		win.SendMessage(hwnd, win.BM_SETCHECK, 1, 0)
	}
	return hwnd
}

func createListBox(parent win.HWND, x, y, w, h int, id int32) win.HWND {
	return win.CreateWindowEx(win.WS_EX_CLIENTEDGE, syscall.StringToUTF16Ptr("LISTBOX"), nil,
		win.WS_CHILD|win.WS_VISIBLE|win.WS_VSCROLL|win.WS_BORDER|win.LBS_NOTIFY,
		int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
}

func setText(hwnd win.HWND, id int32, text string) {
	hCtrl := win.GetDlgItem(hwnd, id)
	t := syscall.StringToUTF16(text)
	win.SendMessage(hCtrl, win.WM_SETTEXT, 0, uintptr(unsafe.Pointer(&t[0])))
}
