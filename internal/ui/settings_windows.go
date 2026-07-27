//go:build windows

package ui

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
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
	ID_PIN_FORECAST      = 123
	ID_LOOK_MODERN       = 124
	ID_LOOK_SYSTEM       = 125
	WM_APP_SEARCH_RESULT = win.WM_APP + 1

	// Trackbar messages lxn/win does not declare. There is deliberately no
	// TBM_SETBKCOLOR here: no such message exists - WM_USER+30 is
	// TBM_GETTOOLTIPS - and a trackbar's background comes from the
	// WM_CTLCOLORSTATIC its parent answers.
	TBM_CLEARTICS  = win.WM_USER + 9
	TBM_SETTICFREQ = win.WM_USER + 20
)

const (
	settingsClassName = "NimbusSettingsClass"

	// The window has a caption and a close button and nothing else: it is not
	// resizable, so the layout below never has to reflow.
	settingsStyle = win.WS_CAPTION | win.WS_SYSMENU

	// Client size in layout units, which are pixels at 96 DPI. Everything in
	// createControls is written in the same units and goes through dp() on its
	// way to Win32, so at 100% scaling these are literally the numbers Windows
	// receives.
	//
	// The height is NOT derived from createControls' running y, so adding a row
	// there means growing it by the same amount here. Nothing catches the
	// mismatch: layoutDPI shrinks the layout to fit the SCREEN, never to fit the
	// content, so a window left too short simply clips whatever is at the bottom
	// - which is the Save button.
	//
	// Where 790 comes from: createControls' running y reaches 740 at the button
	// row, the buttons are 28 tall, and the remaining 22 is the bottom margin this
	// layout has always left below them. It was 734 until the appearance group
	// added its 56 - a 48-tall group box plus the 8 of clearance every group in
	// this window is followed by - which pushed the button row down from 684.
	settingsContentW = 440
	settingsContentH = 790
)

var (
	settingsClassOnce sync.Once
	settingsClassOK   bool

	// settingsBusy guards against a second settings window. Two of them would
	// not strand anybody - each caller has its own channel - but they would
	// race to write the same config file, and the loser's edits would vanish
	// without a word.
	settingsBusy atomic.Bool
	settingsHWND atomic.Uintptr
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
	hwnd win.HWND
	inst win.HINSTANCE
	cfg  *config.Config
	lang i18n.Lang
	dark bool

	// dpi is the DPI the layout is scaled by. It is the window's own DPI
	// unless that would make the window taller than the screen, in which case
	// it is whatever does fit.
	dpi int32

	// result carries the caller's answer, buffered so that resolving it never
	// blocks, and once so that it happens exactly one time.
	result chan *config.Config
	once   sync.Once

	bgBrush win.HBRUSH
	edBrush win.HBRUSH
	font    win.HFONT

	onFontChange   func(int)
	fontScaleLabel win.HWND

	// pinCheck is kept because a missing control cannot be told from an unticked
	// one: BM_GETCHECK on a zero HWND answers 0, which reads as false. For every
	// other control in this window the false answer happens to be what
	// config.Default() says, so a creation failure is invisible - but
	// ForecastPinned defaults to TRUE, so reading it back from a control that was
	// never created would silently turn off an option the user was never shown.
	pinCheck win.HWND

	// lookModern and lookSystem are kept for the same reason as pinCheck, and both
	// halves are needed: BM_GETCHECK on a zero HWND answers 0, so a radio that was
	// never created cannot be told from an unselected one, and reading the pair
	// back would answer "modern" - moving a user who had chosen the system look
	// back to Modern without either button ever appearing. Appearance defaults to
	// "modern", so as with ForecastPinned the wrong answer is the plausible one and
	// nothing else would show it up.
	lookModern win.HWND
	lookSystem win.HWND

	// results belongs to the UI thread. pending is the hand-off from the
	// search goroutine, which touches nothing else in here.
	results   []weather.GeoResult
	searching bool
	mu        sync.Mutex
	pending   []weather.GeoResult
}

// setDialogs maps a window to its dialog.
//
// The obvious alternative - stash the Go pointer in GWLP_USERDATA and convert
// it back in the window procedure - hides a live Go pointer inside memory the
// garbage collector cannot see, and needs a uintptr-to-pointer conversion that
// `go vet` rightly refuses to believe. A map costs one lock per message on a
// window that handles a few hundred in its lifetime.
var (
	setDialogsMu sync.Mutex
	setDialogs   = map[win.HWND]*setDlg{}
)

func registerSetDlg(hwnd win.HWND, d *setDlg) {
	setDialogsMu.Lock()
	defer setDialogsMu.Unlock()
	setDialogs[hwnd] = d
}

func unregisterSetDlg(hwnd win.HWND) {
	setDialogsMu.Lock()
	defer setDialogsMu.Unlock()
	delete(setDialogs, hwnd)
}

func setDlgFor(hwnd win.HWND) *setDlg {
	setDialogsMu.Lock()
	defer setDialogsMu.Unlock()
	return setDialogs[hwnd]
}

// showSettings opens the settings window and blocks until the user is done,
// returning the configuration to adopt or nil to change nothing.
//
// The caller is a goroutine the tray spawned for exactly this, so blocking is
// correct - but it must never block forever, which is what happens if any exit
// from the window forgets to answer. See finish.
func showSettings(cfg *config.Config, onFontChange func(int)) *config.Config {
	if !settingsBusy.CompareAndSwap(false, true) {
		if h := win.HWND(settingsHWND.Load()); h != 0 {
			win.SetForegroundWindow(h)
		}
		return nil
	}
	d := &setDlg{
		cfg:          cfg,
		lang:         i18n.ParseLang(cfg.Language),
		dark:         resolveDark(cfg.IconTheme),
		dpi:          baseDPI,
		result:       make(chan *config.Config, 1),
		onFontChange: onFontChange,
	}
	go d.run()
	return <-d.result
}

func initCommon() {
	var ice win.INITCOMMONCONTROLSEX
	ice.DwSize = uint32(unsafe.Sizeof(ice))
	// The font-scale slider is a trackbar, which lives in comctl32 and is
	// covered by ICC_BAR_CLASSES; ICC_STANDARD_CLASSES alone would not
	// guarantee the class is registered.
	ice.DwICC = win.ICC_STANDARD_CLASSES | win.ICC_BAR_CLASSES
	win.InitCommonControlsEx(&ice)
}

// finish answers the caller exactly once.
//
// Every way out of this window routes through here - Save, Cancel, Escape, the
// close button, delete-config, and every failure before the window even
// appears. Missing one strands the goroutine blocked on the channel for the
// life of the process, and with it the Settings menu item, which is precisely
// what Cancel used to do.
func (d *setDlg) finish(nc *config.Config) {
	// The destroy stays OUTSIDE this Once, and moving it in would wedge this
	// backend the way the GTK one was wedged: a window procedure that reaches
	// finish again from WM_DESTROY re-enters once.Do on the same thread, and
	// sync.Once holds a non-reentrant mutex until its function returns. Here the
	// callers destroy the window after finish has returned, so the nesting cannot
	// happen - see the comment in settings_linux.go for what it cost when it did.
	d.once.Do(func() { d.result <- nc })
}

func (d *setDlg) run() {
	// A Win32 message queue belongs to a thread. Without this lock the Go
	// runtime is free to move this goroutine between OS threads: the window
	// could be created on one and GetMessage called on another, leaving its
	// messages queued where nobody is reading, and PostQuitMessage - which
	// posts WM_QUIT to the calling thread and no other - could land anywhere,
	// including on the thread running the tray's own loop.
	//
	// The lock is never released on purpose. A goroutine that exits while
	// locked takes its thread with it, which is what should happen to a thread
	// that owns a message queue and has just destroyed its window.
	runtime.LockOSThread()

	defer d.finish(nil)
	defer settingsBusy.Store(false)
	defer settingsHWND.Store(0)

	d.inst = win.GetModuleHandle(nil)
	if d.inst == 0 {
		log.Print("settings: GetModuleHandle failed")
		return
	}

	settingsClassOnce.Do(func() {
		cn := syscall.StringToUTF16(settingsClassName)
		wc := &win.WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(win.WNDCLASSEX{})),
			Style:         win.CS_HREDRAW | win.CS_VREDRAW,
			LpfnWndProc:   syscall.NewCallback(settingsWndProc),
			HInstance:     d.inst,
			HIcon:         win.LoadIcon(d.inst, win.MAKEINTRESOURCE(1)),
			HCursor:       win.LoadCursor(0, win.MAKEINTRESOURCE(win.IDC_ARROW)),
			HbrBackground: win.COLOR_BTNFACE + 1,
			LpszClassName: &cn[0],
		}
		settingsClassOK = win.RegisterClassEx(wc) != 0
	})
	if !settingsClassOK {
		log.Print("settings: RegisterClassEx failed")
		return
	}

	initCommon()

	// The window is created at a placeholder size and hidden: its DPI cannot
	// be asked for until it exists, and its real size depends on the answer.
	// syscall.StringToUTF16 rather than utf16Of: this one is a compiled-in caption
	// from internal/i18n, so it cannot contain the NUL byte that makes the former
	// panic. Every helper below that can see a city name from the config or a
	// geocoding result uses utf16Of instead.
	title := syscall.StringToUTF16(d.lang.SettingsTitle())
	d.hwnd = win.CreateWindowEx(
		0, syscall.StringToUTF16Ptr(settingsClassName), &title[0],
		settingsStyle,
		win.CW_USEDEFAULT, win.CW_USEDEFAULT, 100, 100,
		0, 0, d.inst, nil,
	)
	if d.hwnd == 0 {
		log.Print("settings: CreateWindowEx failed")
		return
	}
	registerSetDlg(d.hwnd, d)
	defer unregisterSetDlg(d.hwnd)
	settingsHWND.Store(uintptr(d.hwnd))

	if d.dark {
		d.bgBrush = createDarkBrush()
		d.edBrush = createEditBrush()
	}

	d.dpi = layoutDPI(d.hwnd, settingsContentW, settingsContentH, settingsStyle)
	d.font = uiFont(d.dpi)
	d.layout()
	d.createControls()

	if d.dark {
		setDarkTitleBar(d.hwnd, true)
	}
	win.ShowWindow(d.hwnd, win.SW_SHOW)
	win.UpdateWindow(d.hwnd)

	d.pump()
}

// pump runs the window's message loop.
func (d *setDlg) pump() {
	var msg win.MSG
	for {
		// GetMessage answers 0 for WM_QUIT and -1 for a broken queue. Treating
		// -1 as "keep going", which `!= 0` does, spins forever.
		switch win.GetMessage(&msg, 0, 0, 0) {
		case 0:
			return
		case -1:
			log.Print("settings: GetMessage failed")
			return
		}

		// Escape has to be caught here. IsDialogMessage gives a plain window
		// the dialog keyboard interface - Tab, arrows, mnemonics - but the
		// Escape-means-cancel rule belongs to the real dialog manager in
		// DefDlgProc, which this window does not use. The child test keeps a
		// dropped-down combo box's own Escape to itself.
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

func (d *setDlg) owns(hwnd win.HWND) bool {
	return hwnd == d.hwnd || win.IsChild(d.hwnd, hwnd)
}

// dp scales a layout unit to device pixels.
func (d *setDlg) dp(v int) int { return int(scaleDPI(v, d.dpi)) }

// layout sizes the frame so its client area holds the layout at this DPI, and
// puts it in the middle of the screen.
func (d *setDlg) layout() {
	ow, oh := frameOverhead(settingsStyle)
	centreOn(d.hwnd, scaleDPI(settingsContentW, d.dpi)+ow, scaleDPI(settingsContentH, d.dpi)+oh)
}

func (d *setDlg) createControls() {
	y := 10

	d.static(d.lang.CityLabel(), 12, y+4, 70, 20)
	d.edit(d.cfg.CityName, 86, y, 200, 24, ID_CITY_EDIT)
	y += 30

	d.button(d.lang.SearchBtn(), 294, y+2, 70, 24, ID_SEARCH)
	y += 30
	d.listBox(86, y, 278, 100, ID_CITY_LIST)
	y += 108

	d.static(d.lang.LatLabel(), 12, y+4, 70, 20)
	d.edit(fmt.Sprintf("%.4f", d.cfg.Latitude), 86, y, 120, 24, ID_LAT_EDIT)
	y += 30

	d.static(d.lang.LonLabel(), 12, y+4, 70, 20)
	d.edit(fmt.Sprintf("%.4f", d.cfg.Longitude), 86, y, 120, 24, ID_LON_EDIT)
	y += 40

	d.group(d.lang.TemperatureGroup(), 12, y, 150, 48)
	d.radio("°C", 22, y+18, 60, 22, ID_TEMP_C, d.cfg.Units == "celsius", true)
	d.radio("°F", 82, y+18, 60, 22, ID_TEMP_F, d.cfg.Units == "fahrenheit", false)
	y += 56

	d.group(d.lang.PressureGroup(), 12, y, 280, 48)
	d.radio(d.lang.HPa(), 22, y+18, 60, 22, ID_PRES_H, d.cfg.PressureUnit == "hpa", true)
	d.radio(d.lang.MmHg(), 90, y+18, 70, 22, ID_PRES_M, d.cfg.PressureUnit == "mmhg", false)
	d.radio(d.lang.InHg(), 170, y+18, 110, 22, ID_PRES_I, d.cfg.PressureUnit == "inhg", false)
	y += 56

	d.group(d.lang.WindGroup(), 12, y, 180, 48)
	d.radio(d.lang.WindMS(), 22, y+18, 60, 22, ID_WIND_MS, d.cfg.WindUnit == "ms", true)
	d.radio(d.lang.WindKMH(), 90, y+18, 70, 22, ID_WIND_KMH, d.cfg.WindUnit == "kmh", false)
	y += 56

	d.group(d.lang.ThemeGroup(), 12, y, 280, 48)
	d.radio(d.lang.ThemeAuto(), 22, y+18, 60, 22, ID_THEME_A, d.cfg.IconTheme == "auto", true)
	d.radio(d.lang.ThemeDark(), 90, y+18, 60, 22, ID_THEME_D, d.cfg.IconTheme == "dark", false)
	d.radio(d.lang.ThemeLight(), 170, y+18, 60, 22, ID_THEME_L, d.cfg.IconTheme == "light", false)
	y += 56

	d.group(d.lang.LanguageGroup(), 12, y, 270, 48)
	d.radio("English", 22, y+18, 70, 22, ID_LANG_EN, d.cfg.Language == "en", true)
	d.radio("Українська", 100, y+18, 150, 22, ID_LANG_UK, d.cfg.Language == "uk", false)
	y += 56

	d.group(d.lang.FontScaleGroup(), 12, y, 340, 48)
	d.slider(86, y+14, 180, 24, ID_FONT_SCALE, d.cfg.FontScale, 1, 100)
	d.fontScaleLabel = d.static(fmt.Sprintf("%d%%", d.cfg.FontScale), 272, y+16, 40, 20)
	y += 56

	// Directly above the pin checkbox: both settings are about the forecast panel
	// and nothing else. Modern carries WS_GROUP so this pair is its own
	// arrow-key group rather than an extension of whatever came before, and Modern
	// is checked for every value that is not exactly "system" - an unrecognised
	// value in the file means Modern, which is the same rule the panel applies.
	d.group(d.lang.AppearanceGroup(), 12, y, 280, 48)
	d.lookModern = d.radio(d.lang.LookModern(), 22, y+18, 90, 22, ID_LOOK_MODERN, d.cfg.Appearance != "system", true)
	d.lookSystem = d.radio(d.lang.LookSystem(), 120, y+18, 160, 22, ID_LOOK_SYSTEM, d.cfg.Appearance == "system", false)
	if d.lookModern == 0 || d.lookSystem == 0 {
		// Logged here because onSave's handling of it is to keep the stored value,
		// which is correct and completely silent.
		log.Print("settings: appearance radios could not be created")
	}
	y += 56

	// No group box round a single checkbox: a titled frame whose only content is
	// one labelled control says the same thing twice, and the caption it would add
	// is not free here. This layout is not scrolled and not resizable, so height
	// is paid for out of legibility: layoutDPI shrinks the WHOLE window - every
	// label, every radio - until it fits the work area, so 22 more units of chrome
	// makes the text smaller on precisely the small screens that can least afford
	// it. The checkbox's own label is its title.
	//
	// 22 for the box, 12 of clearance below it. The 12 is what the frame used to
	// provide: without it the interval group's caption sits on the checkbox's
	// label, since a group box's caption is drawn ON its top border.
	d.pinCheck = d.check(d.lang.PinForecast(), 12, y, 340, 22, ID_PIN_FORECAST, d.cfg.ForecastPinned)
	y += 34

	d.group(d.lang.UpdateInterval(), 12, y, 340, 48)
	d.combo(86, y+14, 180, 200, ID_INTERVAL, d.cfg.UpdateInterval)
	y += 66

	d.button(d.lang.SaveBtn(), 60, y, 90, 28, ID_SAVE)
	d.button(d.lang.CancelBtn(), 160, y, 90, 28, ID_CANCEL)
	d.button(d.lang.DeleteCfgBtn(), 260, y, 130, 28, ID_DEL_CFG)
}

// The layout helpers below all take 96-DPI units and hand every new control
// the window font, which is the only way it gets one: a control created with
// CreateWindowEx and left alone draws in the Windows 3.1 bitmap face.

func (d *setDlg) adopt(hwnd win.HWND) win.HWND {
	if hwnd != 0 && d.font != 0 {
		win.SendMessage(hwnd, win.WM_SETFONT, uintptr(d.font), 1)
	}
	return hwnd
}

func (d *setDlg) static(text string, x, y, w, h int) win.HWND {
	return d.adopt(createStatic(d.hwnd, text, d.dp(x), d.dp(y), d.dp(w), d.dp(h)))
}

func (d *setDlg) edit(text string, x, y, w, h int, id int32) win.HWND {
	return d.adopt(createEdit(d.hwnd, text, d.dp(x), d.dp(y), d.dp(w), d.dp(h), id))
}

func (d *setDlg) button(text string, x, y, w, h int, id int32) win.HWND {
	return d.adopt(createButton(d.hwnd, text, d.dp(x), d.dp(y), d.dp(w), d.dp(h), id))
}

func (d *setDlg) group(text string, x, y, w, h int) win.HWND {
	return d.adopt(unthemeForDark(createGroup(d.hwnd, text, d.dp(x), d.dp(y), d.dp(w), d.dp(h)), d.dark))
}

func (d *setDlg) radio(text string, x, y, w, h int, id int32, checked, first bool) win.HWND {
	return d.adopt(unthemeForDark(
		createRadio(d.hwnd, text, d.dp(x), d.dp(y), d.dp(w), d.dp(h), id, checked, first), d.dark))
}

// check makes a checkbox. The unthemeForDark treatment is not optional and not
// copied for symmetry: a checkbox is a BUTTON like the radios and the group
// boxes, so Common-Controls 6 paints its caption in the theme's own colour over
// the dark brush this window supplies, and the label goes near-black on
// near-black. See unthemeForDark.
//
// A zero return is the caller's to handle, and the log line is here because the
// caller's handling is to leave the stored value alone - which is correct and
// completely silent.
func (d *setDlg) check(text string, x, y, w, h int, id int32, checked bool) win.HWND {
	hwnd := d.adopt(unthemeForDark(
		createCheck(d.hwnd, text, d.dp(x), d.dp(y), d.dp(w), d.dp(h), id, checked), d.dark))
	if hwnd == 0 {
		log.Printf("settings: checkbox %d creation failed", id)
	}
	return hwnd
}

func (d *setDlg) listBox(x, y, w, h int, id int32) win.HWND {
	return d.adopt(createListBox(d.hwnd, d.dp(x), d.dp(y), d.dp(w), d.dp(h), id))
}

func (d *setDlg) slider(x, y, w, h int, id int32, pos, lo, hi int) {
	cls := syscall.StringToUTF16Ptr("msctls_trackbar32")
	hwnd := win.CreateWindowEx(0, cls, nil,
		win.WS_CHILD|win.WS_VISIBLE|win.WS_TABSTOP,
		scaleDPI(x, d.dpi), scaleDPI(y, d.dpi), scaleDPI(w, d.dpi), scaleDPI(h, d.dpi),
		d.hwnd, win.HMENU(id), 0, nil)
	if hwnd == 0 {
		log.Print("settings: trackbar creation failed")
		return
	}
	win.SendMessage(hwnd, win.TBM_SETRANGEMIN, 0, uintptr(lo))
	win.SendMessage(hwnd, win.TBM_SETRANGEMAX, 0, uintptr(hi))
	win.SendMessage(hwnd, win.TBM_SETPOS, 1, uintptr(pos))
	win.SendMessage(hwnd, TBM_CLEARTICS, 1, 0)
	win.SendMessage(hwnd, TBM_SETTICFREQ, 25, 0)
	d.adopt(hwnd)
}

func (d *setDlg) combo(x, y, w, dropH int, id int32, curMinutes int) {
	cls := syscall.StringToUTF16Ptr("COMBOBOX")
	// The height given to a combo box is the height of its dropped-down list;
	// the closed control sizes itself to its font.
	hwnd := win.CreateWindowEx(0, cls, nil,
		win.WS_CHILD|win.WS_VISIBLE|win.WS_VSCROLL|win.CBS_DROPDOWNLIST|win.WS_TABSTOP,
		scaleDPI(x, d.dpi), scaleDPI(y, d.dpi), scaleDPI(w, d.dpi), scaleDPI(dropH, d.dpi),
		d.hwnd, win.HMENU(id), 0, nil)
	if hwnd == 0 {
		log.Print("settings: combo box creation failed")
		return
	}
	d.adopt(hwnd)
	sel := 0
	for i, iv := range intervals {
		lb := syscall.StringToUTF16(iv.label)
		win.SendMessage(hwnd, win.CB_ADDSTRING, 0, uintptr(unsafe.Pointer(&lb[0])))
		runtime.KeepAlive(lb)
		if iv.minutes == curMinutes {
			sel = i
		}
	}
	win.SendMessage(hwnd, win.CB_SETCURSEL, uintptr(sel), 0)
}

func (d *setDlg) getSlider(id int32) int {
	hwnd := win.GetDlgItem(d.hwnd, id)
	return int(win.SendMessage(hwnd, win.TBM_GETPOS, 0, 0))
}

func (d *setDlg) getComboSel(id int32) int {
	hwnd := win.GetDlgItem(d.hwnd, id)
	return int(win.SendMessage(hwnd, win.CB_GETCURSEL, 0, 0))
}

func settingsWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	dlg := setDlgFor(hwnd)
	if dlg == nil {
		// Everything Windows sends while CreateWindowEx is still running
		// arrives before the window can be registered. None of it needs the
		// dialog: the window is invisible and childless until run() says
		// otherwise.
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case win.WM_ERASEBKGND:
		if dlg.dark {
			eraseBg(hwnd, wParam, dlg.bgBrush)
			return 1
		}
		// Nothing in this window paints its own client area, so the light case
		// must let DefWindowProc erase with the class brush. Answering 0 here
		// claims the background was not erased and leaves whatever was in the
		// backing store showing through.
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	case win.WM_CTLCOLORSTATIC, win.WM_CTLCOLORBTN, win.WM_CTLCOLOREDIT, win.WM_CTLCOLORLISTBOX:
		if brush := handleCtlColor(hwnd, wParam, lParam, dlg.dark, dlg.bgBrush, dlg.edBrush); brush != 0 {
			return brush
		}
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	case win.WM_HSCROLL:
		dlg.onFontScale(win.LOWORD(uint32(wParam)))
	case win.WM_COMMAND:
		switch win.LOWORD(uint32(wParam)) {
		case ID_SEARCH:
			dlg.onSearch()
		case ID_CITY_LIST:
			if win.HIWORD(uint32(wParam)) == win.LBN_SELCHANGE {
				dlg.onCitySelect()
			}
		case ID_SAVE:
			dlg.onSave()
		case ID_CANCEL, win.IDCANCEL:
			win.DestroyWindow(hwnd)
		case ID_DEL_CFG:
			dlg.onDeleteCfg()
		}
	case WM_APP_SEARCH_RESULT:
		dlg.onSearchDone()
	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
	case win.WM_DESTROY:
		// The last word on the way out. Save and delete-config have already
		// answered by now, and once keeps their answer; the close button,
		// Escape and Cancel arrive here with nothing said, and this is what
		// stops the caller waiting for ever.
		dlg.finish(nil)
		win.PostQuitMessage(0)
	case win.WM_NCDESTROY:
		// The children still exist during WM_DESTROY and still hold the font
		// and the brushes. By WM_NCDESTROY they are gone, so this is where the
		// objects can be released without anything else referring to them.
		dlg.release()
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}

// release frees the GDI objects the window owns.
func (d *setDlg) release() {
	if d.font != 0 {
		win.DeleteObject(win.HGDIOBJ(d.font))
		d.font = 0
	}
	if d.bgBrush != 0 {
		win.DeleteObject(win.HGDIOBJ(d.bgBrush))
		d.bgBrush = 0
	}
	if d.edBrush != 0 {
		win.DeleteObject(win.HGDIOBJ(d.edBrush))
		d.edBrush = 0
	}
}

// onFontScale previews the tray font size live, so the user can judge it
// against the real notification area instead of a number. Nothing is saved.
func (d *setDlg) onFontScale(code uint16) {
	fs := d.getSlider(ID_FONT_SCALE)
	if d.fontScaleLabel != 0 {
		t := syscall.StringToUTF16(fmt.Sprintf("%d%%", fs))
		win.SendMessage(d.fontScaleLabel, win.WM_SETTEXT, 0, uintptr(unsafe.Pointer(&t[0])))
		runtime.KeepAlive(t)
	}
	// Regenerating the icon on every pixel of a drag would hammer the tray, so
	// the preview waits until the thumb is let go.
	if code != win.SB_THUMBTRACK && d.onFontChange != nil {
		d.onFontChange(fs)
	}
}

func (d *setDlg) onSearch() {
	if d.searching {
		return
	}
	query := d.getText(ID_CITY_EDIT)
	if query == "" {
		return
	}

	// The button greys out for the duration: without that the user cannot tell
	// a slow search from one that never started.
	d.searching = true
	win.EnableWindow(win.GetDlgItem(d.hwnd, ID_SEARCH), false)

	hwnd, lang := d.hwnd, d.lang.String()
	go func() {
		res, err := weather.SearchCity(query, lang)
		if err != nil {
			log.Printf("settings: city search for %q failed: %v", query, err)
			res = nil
		}
		d.mu.Lock()
		d.pending = res
		d.mu.Unlock()

		// Post, never send. A control belongs to the thread that created it,
		// and this is not that thread: SendMessage from here would block this
		// goroutine until the UI thread got round to it, and calling
		// LB_ADDSTRING directly would be worse still. PostMessage puts the
		// work on the queue and the UI thread does it in onSearchDone.
		win.PostMessage(hwnd, WM_APP_SEARCH_RESULT, 0, 0)
	}()
}

// onSearchDone runs on the UI thread, which is the only place the list box may
// be touched. An empty result and a failed lookup land here identically -
// there is nothing useful to say about the difference in a list box.
func (d *setDlg) onSearchDone() {
	d.searching = false
	win.EnableWindow(win.GetDlgItem(d.hwnd, ID_SEARCH), true)

	d.mu.Lock()
	d.results, d.pending = d.pending, nil
	d.mu.Unlock()

	hList := win.GetDlgItem(d.hwnd, ID_CITY_LIST)
	win.SendMessage(hList, win.LB_RESETCONTENT, 0, 0)
	if len(d.results) == 0 {
		listAdd(hList, d.lang.NoResults())
		return
	}
	for _, r := range d.results {
		listAdd(hList, fmt.Sprintf("%s, %s | %.4f,%.4f", r.Name, r.Country, r.Latitude, r.Longitude))
	}
}

func listAdd(hList win.HWND, text string) {
	t := utf16Of(text)
	win.SendMessage(hList, win.LB_ADDSTRING, 0, uintptr(unsafe.Pointer(&t[0])))
	runtime.KeepAlive(t)
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
	// WM_GETTEXT counts the terminator, so the buffer size is the right wParam.
	win.SendMessage(win.GetDlgItem(d.hwnd, id), win.WM_GETTEXT, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	return syscall.UTF16ToString(buf)
}

func (d *setDlg) isChecked(id int32) bool {
	return win.SendMessage(win.GetDlgItem(d.hwnd, id), win.BM_GETCHECK, 0, 0) == 1
}

func (d *setDlg) onDeleteCfg() {
	if err := config.Delete(); err != nil {
		log.Printf("settings: deleting the config failed: %v", err)
	}
	d.finish(config.Default())
	win.DestroyWindow(d.hwnd)
}

func (d *setDlg) onSave() {
	nc := *d.cfg
	nc.CityName = d.getText(ID_CITY_EDIT)
	nc.Latitude = parseCoord(d.getText(ID_LAT_EDIT), d.cfg.Latitude)
	nc.Longitude = parseCoord(d.getText(ID_LON_EDIT), d.cfg.Longitude)

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
	// Only when both radios were actually built, for the reason lookModern and
	// lookSystem record; nc carries the stored value forward when they were not.
	if d.lookModern != 0 && d.lookSystem != 0 {
		if d.isChecked(ID_LOOK_SYSTEM) {
			nc.Appearance = "system"
		} else {
			nc.Appearance = "modern"
		}
	}
	// Only when the box was actually built, for the reason pinCheck records; nc
	// carries the stored value forward when it was not. The GTK backend guards its
	// own checkbox the same way.
	if d.pinCheck != 0 {
		nc.ForecastPinned = d.isChecked(ID_PIN_FORECAST)
	}

	if sel := d.getComboSel(ID_INTERVAL); sel >= 0 && sel < len(intervals) {
		nc.UpdateInterval = intervals[sel].minutes
	}

	if err := nc.Save(); err != nil {
		log.Printf("settings: saving the config failed: %v", err)
	}
	d.finish(&nc)
	win.DestroyWindow(d.hwnd)
}

// parseCoord keeps the previous value when the field holds nonsense, rather
// than silently moving the user to the Gulf of Guinea. It is the same rule the
// GTK backend applies, down to using the same parser.

// The createX helpers below take device pixels. Everything in this file goes
// through the setDlg methods above, which scale for DPI first.

func createStatic(parent win.HWND, text string, x, y, w, h int) win.HWND {
	t := utf16Of(text)
	return win.CreateWindowEx(0, syscall.StringToUTF16Ptr("STATIC"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.SS_LEFT,
		int32(x), int32(y), int32(w), int32(h),
		parent, 0, 0, nil)
}

func createEdit(parent win.HWND, text string, x, y, w, h int, id int32) win.HWND {
	t := utf16Of(text)
	return win.CreateWindowEx(win.WS_EX_CLIENTEDGE, syscall.StringToUTF16Ptr("EDIT"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.ES_LEFT|win.ES_AUTOHSCROLL,
		int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
}

func createButton(parent win.HWND, text string, x, y, w, h int, id int32) win.HWND {
	t := utf16Of(text)
	return win.CreateWindowEx(0, syscall.StringToUTF16Ptr("BUTTON"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.BS_PUSHBUTTON|win.WS_TABSTOP,
		int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
}

// unthemeForDark detaches a control from the visual style engine.
//
// The manifest binds Common-Controls 6, so a themed BS_GROUPBOX or
// BS_AUTORADIOBUTTON paints its own caption with the THEME's text colour and
// ignores the SetTextColor that WM_CTLCOLORSTATIC applies - while still using
// the dark brush the same handler returns. The result on a dark palette is
// near-black text on a near-black background. Detaching restores the
// pre-manifest behaviour for exactly the controls whose text this code
// colours itself, and only when the dark palette is in use.
func unthemeForDark(hwnd win.HWND, dark bool) win.HWND {
	if !dark || hwnd == 0 {
		return hwnd
	}
	empty := syscall.StringToUTF16Ptr("")
	win.SetWindowTheme(hwnd, empty, empty)
	return hwnd
}

func createGroup(parent win.HWND, text string, x, y, w, h int) win.HWND {
	t := utf16Of(text)
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
	t := utf16Of(text)
	hwnd := win.CreateWindowEx(0, syscall.StringToUTF16Ptr("BUTTON"), &t[0],
		style, int32(x), int32(y), int32(w), int32(h),
		parent, win.HMENU(id), 0, nil)
	if checked {
		win.SendMessage(hwnd, win.BM_SETCHECK, 1, 0)
	}
	return hwnd
}

// createCheck makes an auto checkbox, which toggles itself on a click. Nothing
// therefore answers its WM_COMMAND: the state is read back with BM_GETCHECK when
// Save runs, exactly as the radios are.
//
// There is no "first" argument as createRadio has because WS_GROUP is not
// optional here, and the reasoning in the earlier version of this comment had the
// flag backwards. WS_GROUP marks the START of a group, not its end: it is set on
// the FIRST radio of each set, and every control created after it belongs to that
// set until the next WS_GROUP appears. So a checkbox without it does not stand
// outside the radio groups - it joins the last one that was opened, which is
// ID_LANG_EN's. The dialog manager's arrow keys move within a group and CHECK the
// auto-radio they land on, so an arrow key pressed on an unflagged checkbox
// silently changes the language. Setting WS_GROUP closes that set at the checkbox
// and starts a new one, which is exactly what a control belonging to no set
// wants. WS_TABSTOP is separate and is what makes it reachable by Tab.
//
// What this does NOT fix, because it predates the checkbox: nothing between
// ID_LANG_EN and here carries WS_GROUP either, so the font-scale trackbar is
// still inside the language group and an arrow key on "English" can still walk
// focus onto it. Fixing that means auditing the group boundaries of the whole
// window, which is its own change.
func createCheck(parent win.HWND, text string, x, y, w, h int, id int32, checked bool) win.HWND {
	t := utf16Of(text)
	hwnd := win.CreateWindowEx(0, syscall.StringToUTF16Ptr("BUTTON"), &t[0],
		win.WS_CHILD|win.WS_VISIBLE|win.BS_AUTOCHECKBOX|win.WS_TABSTOP|win.WS_GROUP,
		int32(x), int32(y), int32(w), int32(h),
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
	t := utf16Of(text)
	win.SendMessage(hCtrl, win.WM_SETTEXT, 0, uintptr(unsafe.Pointer(&t[0])))
	runtime.KeepAlive(t)
}
