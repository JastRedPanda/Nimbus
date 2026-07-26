//go:build linux

// Package gtk binds the small subset of GTK3 that Nimbus needs. The library is
// loaded at runtime with dlopen through purego, so there is no cgo in this
// package and no GTK development headers are required to build it.
//
// Nimbus owns the GTK main loop: main() locks a thread, calls Init and then
// Main, and the tray runs alongside on its own D-Bus goroutine. Every call into
// GTK therefore has to happen on that loop's thread, which is what Invoke is
// for. Touching any gtk_* function from another goroutine is not safe and will
// eventually corrupt GTK's internal state.
package gtk

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"sync"

	"github.com/ebitengine/purego"
)

// GTK / GDK enum values used by this package.
const (
	WindowToplevel        = 0 // GTK_WINDOW_TOPLEVEL
	OrientationHorizontal = 0 // GTK_ORIENTATION_HORIZONTAL
	OrientationVertical   = 1 // GTK_ORIENTATION_VERTICAL
	WinPosCenter          = 1 // GTK_WIN_POS_CENTER
	JustifyCenter         = 2 // GTK_JUSTIFY_CENTER

	// GtkAlign
	AlignFill   = 0
	AlignStart  = 1
	AlignEnd    = 2
	AlignCenter = 3

	// GtkPolicyType, for scrolled windows.
	policyAutomatic = 1 // GTK_POLICY_AUTOMATIC
	policyNever     = 2 // GTK_POLICY_NEVER

	colorspaceRGB = 0 // GDK_COLORSPACE_RGB
	typeBoolean   = 5 << 2
	typeString    = 16 << 2
	sourceRemove  = 0 // G_SOURCE_REMOVE
	stateNormal   = 0 // GTK_STATE_FLAG_NORMAL

	dialogModal       = 1 // GTK_DIALOG_MODAL
	dialogDestroyWith = 2 // GTK_DIALOG_DESTROY_WITH_PARENT
	messageError      = 3 // GTK_MESSAGE_ERROR
	buttonsNone       = 0 // GTK_BUTTONS_NONE
	responseClose     = -7
	connectSwapped    = 2 // G_CONNECT_SWAPPED

	// GTK_STYLE_PROVIDER_PRIORITY_APPLICATION. The desktop theme behaves as if
	// it sits at SETTINGS (400): measured against BlackMATE, every priority up
	// to 399 lost and 400 and above won, so THEME (200) - the name that sounds
	// right for a stylesheet - is useless here.
	stylePriorityApp = 600

	// gtk_css_provider_load_from_data takes a gssize length; -1 asks it to run
	// strlen, which is the only count that cannot be wrong. A byte count that
	// is too short truncates the sheet mid-rule and STILL returns TRUE - the
	// provider then holds nothing at all - so counting is a silent failure
	// waiting to happen for no gain.
	cssLenStrlen = -1
)

var (
	initCheck       func(uintptr, uintptr) int32
	mainLevel       func() uint32
	mainRun         func()
	mainQuit        func()
	windowNew       func(int32) uintptr
	windowTitle     func(uintptr, string)
	windowSize      func(uintptr, int32, int32)
	windowResize    func(uintptr, int32)
	windowPos       func(uintptr, int32)
	windowPresent   func(uintptr)
	widgetShowAll   func(uintptr)
	widgetDestroy   func(uintptr)
	containerAdd    func(uintptr, uintptr)
	containerBorder func(uintptr, uint32)
	boxNew          func(int32, int32) uintptr
	boxPackStart    func(uintptr, uintptr, int32, int32, uint32)
	labelNew        func(string) uintptr
	labelMarkup     func(uintptr, string)
	labelText       func(uintptr, string)
	labelWrap       func(uintptr, int32)
	labelJustify    func(uintptr, int32)
	imageFromPixbuf func(uintptr) uintptr
	settingsDefault func() uintptr
	setDefaultIcons func(uintptr)

	gridNew       func() uintptr
	gridAttach    func(uintptr, uintptr, int32, int32, int32, int32)
	gridRowSpace  func(uintptr, uint32)
	gridColSpace  func(uintptr, uint32)
	separatorNew  func(int32) uintptr
	widgetHalign  func(uintptr, int32)
	widgetValign  func(uintptr, int32)
	widgetHexpand func(uintptr, int32)
	widgetVexpand func(uintptr, int32)
	styleContext  func(uintptr) uintptr
	styleGetColor func(uintptr, uint32, *float64)

	cssProviderNew func() uintptr
	cssLoadData    func(uintptr, string, int64, uintptr) int32
	addProviderScr func(uintptr, uintptr, uint32)
	screenDefault  func() uintptr
	styleAddClass  func(uintptr, string)
	widgetName     func(uintptr, string)
	boxHomogeneous func(uintptr, int32)

	windowDecorated func(uintptr, int32)
	windowSkipTask  func(uintptr, int32)
	windowSkipPager func(uintptr, int32)
	windowKeepAbove func(uintptr, int32)
	windowStick     func(uintptr)
	windowMove      func(uintptr, int32, int32)
	windowGetSize   func(uintptr, *int32, *int32)
	widgetShow      func(uintptr)
	widgetSetVisual func(uintptr, uintptr)
	buttonNew       func(string) uintptr

	screenComposited func(uintptr) int32
	screenRGBAVisual func(uintptr) uintptr

	displayDefault  func() uintptr
	displaySeat     func(uintptr) uintptr
	seatPointer     func(uintptr) uintptr
	devicePosition  func(uintptr, *uintptr, *int32, *int32)
	monitorAtPoint  func(uintptr, int32, int32) uintptr
	monitorWorkarea func(uintptr, *int32)
	eventKeyval     func(uintptr, *uint32) int32

	widgetScale    func(uintptr) int32
	imageFromSurf  func(uintptr) uintptr
	surfaceFromPB  func(uintptr, int32, uintptr) uintptr
	surfaceDestroy func(uintptr)

	msgDialogNew  func(uintptr, int32, int32, int32, uintptr) uintptr
	dialogAddBtn  func(uintptr, string, int32) uintptr
	dialogDefResp func(uintptr, int32)
	destroyAddr   uintptr

	signalConnect func(uintptr, string, uintptr, uintptr, uintptr, int32) uint64
	objSetProp    func(uintptr, string, *byte)
	objGetProp    func(uintptr, string, *byte)
	valueInit     func(*byte, uintptr) *byte
	valueSetBool  func(*byte, int32)
	valueGetBool  func(*byte) int32
	valueSetStr   func(*byte, string)
	valueUnset    func(*byte)
	objUnref      func(uintptr)

	idleAdd     func(uintptr, uintptr) uint32
	listAppend  func(uintptr, uintptr) uintptr
	listForeach func(uintptr, uintptr, uintptr)
	listFree    func(uintptr)

	pixbufFromData func(data *byte, cs, alpha, bps, w, h, stride int32, destroy, destroyData uintptr) uintptr

	// The settings form. Everything above this point serves a window Nimbus
	// only ever writes to; these are the widgets it also has to read back.
	entryNew         func() uintptr
	entrySetText     func(uintptr, string)
	entryGetText     func(uintptr) string
	radioNewLabel    func(uintptr, string) uintptr
	radioGetGroup    func(uintptr) uintptr
	toggleSetActive  func(uintptr, int32)
	toggleGetActive  func(uintptr) int32
	scaleNewRange    func(int32, float64, float64, float64) uintptr
	scaleDrawValue   func(uintptr, int32)
	rangeRoundDigits func(uintptr, int32)
	rangeGetValue    func(uintptr) float64
	rangeSetValue    func(uintptr, float64)
	comboTextNew     func() uintptr
	comboTextAppend  func(uintptr, string)
	comboSetActive   func(uintptr, int32)
	comboGetActive   func(uintptr) int32
	scrolledNew      func(uintptr, uintptr) uintptr
	scrolledPolicy   func(uintptr, int32, int32)
	scrolledMinH     func(uintptr, int32)
	frameNew         func(string) uintptr
	widgetSensitive  func(uintptr, int32)
	containerChild   func(uintptr) uintptr
)

var (
	loadOnce sync.Once
	loadErr  error
)

// Load resolves libgtk-3 and its companions. It is idempotent and safe to call
// from any goroutine. dlopen on an already-loaded soname returns the same
// library instance systray's cgo code initialised, so no second copy of GTK
// enters the process.
func Load() error {
	loadOnce.Do(load)
	return loadErr
}

func load() {
	gtk, err := purego.Dlopen("libgtk-3.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = fmt.Errorf("gtk: load libgtk-3.so.0: %w", err)
		return
	}
	gobject, err := purego.Dlopen("libgobject-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = fmt.Errorf("gtk: load libgobject-2.0.so.0: %w", err)
		return
	}
	glib, err := purego.Dlopen("libglib-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = fmt.Errorf("gtk: load libglib-2.0.so.0: %w", err)
		return
	}
	pixbuf, err := purego.Dlopen("libgdk_pixbuf-2.0.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = fmt.Errorf("gtk: load libgdk_pixbuf-2.0.so.0: %w", err)
		return
	}

	// RegisterLibFunc panics when a symbol is missing, which turns version skew
	// into one clear failure here instead of a crash at an arbitrary call site.
	defer func() {
		if r := recover(); r != nil {
			loadErr = fmt.Errorf("gtk: bind symbol: %v", r)
		}
	}()

	purego.RegisterLibFunc(&initCheck, gtk, "gtk_init_check")
	purego.RegisterLibFunc(&mainLevel, gtk, "gtk_main_level")
	purego.RegisterLibFunc(&mainRun, gtk, "gtk_main")
	purego.RegisterLibFunc(&mainQuit, gtk, "gtk_main_quit")
	purego.RegisterLibFunc(&windowNew, gtk, "gtk_window_new")
	purego.RegisterLibFunc(&windowTitle, gtk, "gtk_window_set_title")
	purego.RegisterLibFunc(&windowSize, gtk, "gtk_window_set_default_size")
	purego.RegisterLibFunc(&windowResize, gtk, "gtk_window_set_resizable")
	purego.RegisterLibFunc(&windowPos, gtk, "gtk_window_set_position")
	purego.RegisterLibFunc(&windowPresent, gtk, "gtk_window_present")
	purego.RegisterLibFunc(&widgetShowAll, gtk, "gtk_widget_show_all")
	purego.RegisterLibFunc(&widgetDestroy, gtk, "gtk_widget_destroy")
	purego.RegisterLibFunc(&containerAdd, gtk, "gtk_container_add")
	purego.RegisterLibFunc(&containerBorder, gtk, "gtk_container_set_border_width")
	purego.RegisterLibFunc(&boxNew, gtk, "gtk_box_new")
	purego.RegisterLibFunc(&boxPackStart, gtk, "gtk_box_pack_start")
	purego.RegisterLibFunc(&labelNew, gtk, "gtk_label_new")
	purego.RegisterLibFunc(&labelMarkup, gtk, "gtk_label_set_markup")
	purego.RegisterLibFunc(&labelText, gtk, "gtk_label_set_text")
	purego.RegisterLibFunc(&labelWrap, gtk, "gtk_label_set_line_wrap")
	purego.RegisterLibFunc(&labelJustify, gtk, "gtk_label_set_justify")
	purego.RegisterLibFunc(&imageFromPixbuf, gtk, "gtk_image_new_from_pixbuf")
	purego.RegisterLibFunc(&settingsDefault, gtk, "gtk_settings_get_default")
	purego.RegisterLibFunc(&setDefaultIcons, gtk, "gtk_window_set_default_icon_list")
	purego.RegisterLibFunc(&gridNew, gtk, "gtk_grid_new")
	purego.RegisterLibFunc(&gridAttach, gtk, "gtk_grid_attach")
	purego.RegisterLibFunc(&gridRowSpace, gtk, "gtk_grid_set_row_spacing")
	purego.RegisterLibFunc(&gridColSpace, gtk, "gtk_grid_set_column_spacing")
	purego.RegisterLibFunc(&separatorNew, gtk, "gtk_separator_new")
	purego.RegisterLibFunc(&widgetHalign, gtk, "gtk_widget_set_halign")
	purego.RegisterLibFunc(&widgetValign, gtk, "gtk_widget_set_valign")
	purego.RegisterLibFunc(&widgetHexpand, gtk, "gtk_widget_set_hexpand")
	purego.RegisterLibFunc(&widgetVexpand, gtk, "gtk_widget_set_vexpand")
	purego.RegisterLibFunc(&styleContext, gtk, "gtk_widget_get_style_context")
	purego.RegisterLibFunc(&styleGetColor, gtk, "gtk_style_context_get_color")
	purego.RegisterLibFunc(&windowDecorated, gtk, "gtk_window_set_decorated")
	purego.RegisterLibFunc(&windowSkipTask, gtk, "gtk_window_set_skip_taskbar_hint")
	purego.RegisterLibFunc(&windowSkipPager, gtk, "gtk_window_set_skip_pager_hint")
	purego.RegisterLibFunc(&windowKeepAbove, gtk, "gtk_window_set_keep_above")
	purego.RegisterLibFunc(&windowStick, gtk, "gtk_window_stick")
	purego.RegisterLibFunc(&windowMove, gtk, "gtk_window_move")
	purego.RegisterLibFunc(&windowGetSize, gtk, "gtk_window_get_size")
	purego.RegisterLibFunc(&widgetShow, gtk, "gtk_widget_show")
	purego.RegisterLibFunc(&widgetSetVisual, gtk, "gtk_widget_set_visual")
	purego.RegisterLibFunc(&buttonNew, gtk, "gtk_button_new_with_label")

	// All of these are GDK, but dlsym walks GTK's dependency chain, so the
	// libgtk-3 handle resolves them - the same reason gdk_screen_get_default
	// already works without a second dlopen.
	purego.RegisterLibFunc(&screenComposited, gtk, "gdk_screen_is_composited")
	purego.RegisterLibFunc(&screenRGBAVisual, gtk, "gdk_screen_get_rgba_visual")
	purego.RegisterLibFunc(&displayDefault, gtk, "gdk_display_get_default")
	purego.RegisterLibFunc(&displaySeat, gtk, "gdk_display_get_default_seat")
	purego.RegisterLibFunc(&seatPointer, gtk, "gdk_seat_get_pointer")
	purego.RegisterLibFunc(&devicePosition, gtk, "gdk_device_get_position")
	purego.RegisterLibFunc(&monitorAtPoint, gtk, "gdk_display_get_monitor_at_point")
	purego.RegisterLibFunc(&monitorWorkarea, gtk, "gdk_monitor_get_workarea")
	purego.RegisterLibFunc(&eventKeyval, gtk, "gdk_event_get_keyval")

	purego.RegisterLibFunc(&cssProviderNew, gtk, "gtk_css_provider_new")
	purego.RegisterLibFunc(&cssLoadData, gtk, "gtk_css_provider_load_from_data")
	purego.RegisterLibFunc(&addProviderScr, gtk, "gtk_style_context_add_provider_for_screen")
	purego.RegisterLibFunc(&styleAddClass, gtk, "gtk_style_context_add_class")
	purego.RegisterLibFunc(&widgetName, gtk, "gtk_widget_set_name")
	purego.RegisterLibFunc(&boxHomogeneous, gtk, "gtk_box_set_homogeneous")
	purego.RegisterLibFunc(&widgetScale, gtk, "gtk_widget_get_scale_factor")
	purego.RegisterLibFunc(&imageFromSurf, gtk, "gtk_image_new_from_surface")

	// The settings form's widgets.
	//
	// gtk_entry_get_text is the only binding in this package that returns a
	// string. purego copies the const gchar* into a Go string at the NUL, so the
	// UTF-8 GTK stores arrives byte for byte and nothing here owns the buffer.
	//
	// gtk_range_* rather than gtk_scale_*: value and rounding live on GtkRange,
	// which GtkScale derives from, and binding the base class is what lets one
	// pair of accessors serve any range widget.
	purego.RegisterLibFunc(&entryNew, gtk, "gtk_entry_new")
	purego.RegisterLibFunc(&entrySetText, gtk, "gtk_entry_set_text")
	purego.RegisterLibFunc(&entryGetText, gtk, "gtk_entry_get_text")
	purego.RegisterLibFunc(&radioNewLabel, gtk, "gtk_radio_button_new_with_label")
	purego.RegisterLibFunc(&radioGetGroup, gtk, "gtk_radio_button_get_group")
	purego.RegisterLibFunc(&toggleSetActive, gtk, "gtk_toggle_button_set_active")
	purego.RegisterLibFunc(&toggleGetActive, gtk, "gtk_toggle_button_get_active")
	purego.RegisterLibFunc(&scaleNewRange, gtk, "gtk_scale_new_with_range")
	purego.RegisterLibFunc(&scaleDrawValue, gtk, "gtk_scale_set_draw_value")
	purego.RegisterLibFunc(&rangeRoundDigits, gtk, "gtk_range_set_round_digits")
	purego.RegisterLibFunc(&rangeGetValue, gtk, "gtk_range_get_value")
	purego.RegisterLibFunc(&rangeSetValue, gtk, "gtk_range_set_value")
	purego.RegisterLibFunc(&comboTextNew, gtk, "gtk_combo_box_text_new")
	purego.RegisterLibFunc(&comboTextAppend, gtk, "gtk_combo_box_text_append_text")
	purego.RegisterLibFunc(&comboSetActive, gtk, "gtk_combo_box_set_active")
	purego.RegisterLibFunc(&comboGetActive, gtk, "gtk_combo_box_get_active")
	purego.RegisterLibFunc(&scrolledNew, gtk, "gtk_scrolled_window_new")
	purego.RegisterLibFunc(&scrolledPolicy, gtk, "gtk_scrolled_window_set_policy")
	purego.RegisterLibFunc(&scrolledMinH, gtk, "gtk_scrolled_window_set_min_content_height")
	purego.RegisterLibFunc(&frameNew, gtk, "gtk_frame_new")
	purego.RegisterLibFunc(&widgetSensitive, gtk, "gtk_widget_set_sensitive")
	purego.RegisterLibFunc(&containerChild, gtk, "gtk_container_get_children")

	// These three live in libgdk-3.so.0 and libcairo.so.2, not in libgtk-3, and
	// are still resolved through the libgtk-3 handle: dlsym searches the
	// library's own dependency chain, and GTK3 links both hard. Verified with
	// dlsym on this machine, so no fifth dlopen and no extra failure mode.
	purego.RegisterLibFunc(&screenDefault, gtk, "gdk_screen_get_default")
	purego.RegisterLibFunc(&surfaceFromPB, gtk, "gdk_cairo_surface_create_from_pixbuf")
	purego.RegisterLibFunc(&surfaceDestroy, gtk, "cairo_surface_destroy")

	// message_format is bound as a plain uintptr and always passed 0: GTK
	// guards its va_start behind an if(message_format) test, so a NULL keeps
	// the C varargs machinery from running at all. purego has no contractual
	// varargs support, so the text is set afterwards through GObject
	// properties instead.
	purego.RegisterLibFunc(&msgDialogNew, gtk, "gtk_message_dialog_new")
	purego.RegisterLibFunc(&dialogAddBtn, gtk, "gtk_dialog_add_button")
	purego.RegisterLibFunc(&dialogDefResp, gtk, "gtk_dialog_set_default_response")

	// The raw address of gtk_widget_destroy is connected to "response" with
	// G_CONNECT_SWAPPED, the standard C idiom for self-closing dialogs. It
	// costs no Go callback, so the fixed trampoline budget stays at two.
	destroyAddr, err = purego.Dlsym(gtk, "gtk_widget_destroy")
	if err != nil {
		loadErr = fmt.Errorf("gtk: resolve gtk_widget_destroy: %w", err)
		return
	}

	purego.RegisterLibFunc(&signalConnect, gobject, "g_signal_connect_data")
	purego.RegisterLibFunc(&objSetProp, gobject, "g_object_set_property")
	purego.RegisterLibFunc(&objGetProp, gobject, "g_object_get_property")
	purego.RegisterLibFunc(&valueInit, gobject, "g_value_init")
	purego.RegisterLibFunc(&valueSetBool, gobject, "g_value_set_boolean")
	purego.RegisterLibFunc(&valueGetBool, gobject, "g_value_get_boolean")
	purego.RegisterLibFunc(&valueSetStr, gobject, "g_value_set_string")
	purego.RegisterLibFunc(&valueUnset, gobject, "g_value_unset")
	purego.RegisterLibFunc(&objUnref, gobject, "g_object_unref")

	purego.RegisterLibFunc(&idleAdd, glib, "g_idle_add")
	purego.RegisterLibFunc(&listAppend, glib, "g_list_append")
	purego.RegisterLibFunc(&listForeach, glib, "g_list_foreach")
	purego.RegisterLibFunc(&listFree, glib, "g_list_free")

	purego.RegisterLibFunc(&pixbufFromData, pixbuf, "gdk_pixbuf_new_from_data")

	// purego.NewCallback allocates a trampoline that is never reclaimed and the
	// process-wide budget is under 2000. These two are created once and every
	// callback afterwards is dispatched through them by id, so the budget is
	// not consumed as the user opens and closes windows.
	idleTrampoline = purego.NewCallback(dispatchIdle)
	voidTrampoline = purego.NewCallback(dispatchVoid)
	eventTrampoline = purego.NewCallback(dispatchEvent)

	if initCheck(0, 0) == 0 {
		loadErr = errors.New("gtk: gtk_init_check failed (no display?)")
	}
}

// Ready reports whether GTK is usable. It deliberately does NOT require the
// loop to be running yet: g_idle_add queues work perfectly well beforehand, and
// demanding a live loop would make every window opened during startup fall back
// to the browser UI - silently, and indistinguishably from a broken port.
func Ready() bool {
	return Init() == nil
}

var (
	idleTrampoline  uintptr
	voidTrampoline  uintptr
	eventTrampoline uintptr

	cbMu     sync.Mutex
	cbSeq    uintptr
	idleFns  = map[uintptr]func(){}
	voidFns  = map[uintptr]func(){}
	eventFns = map[uintptr]func(event uintptr) bool{}
)

func register(m map[uintptr]func(), fn func()) uintptr {
	cbMu.Lock()
	defer cbMu.Unlock()
	cbSeq++
	m[cbSeq] = fn
	return cbSeq
}

func dispatchIdle(id uintptr) int32 {
	cbMu.Lock()
	fn := idleFns[id]
	delete(idleFns, id)
	cbMu.Unlock()
	if fn != nil {
		fn()
	}
	return sourceRemove
}

func dispatchVoid(_ uintptr, id uintptr) {
	cbMu.Lock()
	fn := voidFns[id]
	cbMu.Unlock()
	if fn != nil {
		fn()
	}
}

// dispatchEvent serves every gboolean(GtkWidget*, GdkEvent*, gpointer) signal -
// key-press-event, focus-out-event and any other - through one trampoline, so
// the process-wide NewCallback budget stays at three no matter how many windows
// are opened over a session.
func dispatchEvent(_ uintptr, event uintptr, id uintptr) int32 {
	cbMu.Lock()
	fn := eventFns[id]
	cbMu.Unlock()
	if fn != nil && fn(event) {
		return 1 // handled; stop the signal here
	}
	return 0
}

// Invoke schedules fn on the GTK main loop thread. It is safe to call from any
// goroutine and returns immediately without waiting for fn to run. This is the
// only supported way to reach GTK from Nimbus code.
func Invoke(fn func()) error {
	if err := Load(); err != nil {
		return err
	}
	idleAdd(idleTrampoline, register(idleFns, fn))
	return nil
}

// Connect attaches fn to a signal that carries no arguments Nimbus cares about.
// The registration lives as long as the process. Must be called on the GTK
// thread.
func Connect(obj uintptr, signal string, fn func()) {
	signalConnect(obj, signal, voidTrampoline, register(voidFns, fn), 0, 0)
}

// ConnectOnce is Connect for a signal that can only fire once, such as
// "destroy". It drops its own registration afterwards so a long-running tray
// session does not accumulate an entry per window it ever opened.
func ConnectOnce(obj uintptr, signal string, fn func()) {
	var id uintptr
	id = register(voidFns, func() {
		fn()
		cbMu.Lock()
		delete(voidFns, id)
		cbMu.Unlock()
	})
	signalConnect(obj, signal, voidTrampoline, id, 0, 0)
}

// ConnectScoped is Connect for a widget that does not live as long as the
// window holding it: a row of the city-search list, which is thrown away and
// rebuilt on every lookup. It releases its registration when obj is destroyed,
// so a session's worth of searches does not leave a closure per result behind
// for the rest of the process.
//
// It costs no extra trampoline - the release is itself a ConnectOnce on
// "destroy", and that entry deletes itself when it fires. Use plain Connect for
// anything that outlives the widget tree it is built into; the bookkeeping here
// is only worth it for widgets created in a loop.
func ConnectScoped(obj uintptr, signal string, fn func()) {
	id := register(voidFns, fn)
	signalConnect(obj, signal, voidTrampoline, id, 0, 0)
	ConnectOnce(obj, "destroy", func() {
		cbMu.Lock()
		delete(voidFns, id)
		cbMu.Unlock()
	})
}

const preferDarkProp = "gtk-application-prefer-dark-theme"

// gvalue is a scratch GValue reused by the property helpers. It lives at
// package scope on purpose: Go stacks move when they grow, and handing GTK a
// pointer into a moving stack is a use-after-move waiting to happen. Only the
// GTK thread touches it.
var gvalue [24]byte // GType plus two 8-byte data slots

// PreferDark drives the GTK theme variant. Any value other than dark or light
// leaves the setting alone so the desktop's own preference wins, which is what
// a native app should do. Must be called on the GTK thread.
func PreferDark(theme string) {
	if theme != "dark" && theme != "light" {
		return
	}
	settings := settingsDefault()
	if settings == 0 {
		return
	}
	valueInit(&gvalue[0], typeBoolean)
	valueSetBool(&gvalue[0], b2i(theme == "dark"))
	objSetProp(settings, preferDarkProp, &gvalue[0])
	valueUnset(&gvalue[0])
}

// PrefersDark reports the current value of the GTK dark-variant setting. Must
// be called on the GTK thread.
func PrefersDark() bool {
	settings := settingsDefault()
	if settings == 0 {
		return false
	}
	valueInit(&gvalue[0], typeBoolean)
	objGetProp(settings, preferDarkProp, &gvalue[0])
	got := valueGetBool(&gvalue[0]) != 0
	valueUnset(&gvalue[0])
	return got
}

var (
	cssMu     sync.Mutex
	cssLoaded bool
)

// LoadCSS installs css as the application stylesheet for the whole screen. The
// first call wins: a provider added to the screen stays there, and the forecast
// window is opened and closed all session, so installing one per open would
// stack providers GTK then has to consult on every style lookup. Must be called
// on the GTK thread.
//
// Because the sheet is screen-wide it MUST be scoped to a widget id - give the
// window a name with SetName and write every selector under "#that-name" - or
// it restyles the tray menu and the error dialog too.
//
// Two details are load-bearing. The length is cssLenStrlen (-1), for the reason
// that constant documents. And the GError** is bound as a plain
// uintptr and always passed 0, the same idiom ShowError uses for
// message_format: handed a real error pointer GTK discards the ENTIRE
// stylesheet on the first declaration it does not recognise and returns FALSE,
// leaving the window in the raw desktop theme, whereas with NULL it keeps every
// rule that parsed and logs the rest to stderr as "Theme parsing error". That
// also makes the return value worthless - it is TRUE either way - so it is not
// reported.
func LoadCSS(css string) {
	cssMu.Lock()
	defer cssMu.Unlock()
	if cssLoaded {
		return
	}
	prov, screen := cssProviderNew(), screenDefault()
	if prov == 0 || screen == 0 {
		// Leave cssLoaded false so a later call can retry: spending the flag on
		// a transient failure would silently unstyle the app for the rest of
		// the session with no signal at all.
		return
	}
	cssLoadData(prov, css, cssLenStrlen, 0)
	addProviderScr(screen, prov, stylePriorityApp)
	cssLoaded = true
}

// AddClass tags a widget with CSS classes, so one stylesheet can say what a
// "card" or "today" looks like instead of scattering the design across Pango
// markup in Go string literals. Must be called on the GTK thread.
func AddClass(widget uintptr, classes ...string) {
	if widget == 0 {
		return
	}
	ctx := styleContext(widget)
	for _, c := range classes {
		styleAddClass(ctx, c)
	}
}

// SetName gives a widget a CSS id, matched by "#name". This is what scopes an
// application stylesheet to one window - see LoadCSS. Must be called on the GTK
// thread.
func SetName(widget uintptr, name string) {
	if widget == 0 {
		return
	}
	widgetName(widget, name)
}

// retained keeps pixel buffers handed to GDK alive. gdk_pixbuf_new_from_data
// does not copy, so the backing array has to outlive the pixbuf or the image
// renders from freed memory. Keying on the backing array means a caller that
// reuses one cached buffer - the forecast draws the same weather glyph on
// several days, and reopens the window all session - is recorded once.
var (
	retainMu sync.Mutex
	retained = map[*byte][]byte{}
)

// Window is a GtkWindow. All methods must be called on the GTK thread.
type Window uintptr

// NewWindow creates a centred, fixed-size toplevel window.
func NewWindow(title string, w, h int, resizable bool) Window {
	win := windowNew(WindowToplevel)
	windowTitle(win, title)
	windowSize(win, int32(w), int32(h))
	windowPos(win, WinPosCenter)
	if resizable {
		windowResize(win, 1)
	} else {
		windowResize(win, 0)
	}
	return Window(win)
}

func (w Window) SetBorder(px int)  { containerBorder(uintptr(w), uint32(px)) }
func (w Window) Add(child uintptr) { containerAdd(uintptr(w), child) }
func (w Window) ShowAll()          { widgetShowAll(uintptr(w)) }
func (w Window) Present()          { windowPresent(uintptr(w)) }
func (w Window) Destroy()          { widgetDestroy(uintptr(w)) }

// OnDestroy runs fn after the window is gone, which is where a cached handle
// has to be cleared so the next open creates a fresh window.
func (w Window) OnDestroy(fn func()) { ConnectOnce(uintptr(w), "destroy", fn) }

// NewVBox creates a vertical GtkBox.
func NewVBox(spacing int) uintptr { return boxNew(OrientationVertical, int32(spacing)) }

// NewHBox creates a horizontal GtkBox.
func NewHBox(spacing int) uintptr { return boxNew(OrientationHorizontal, int32(spacing)) }

// SetHomogeneous makes a box give every child the same size. It is what keeps
// the seven day cards equal in width without hardcoding one: they stay equal as
// the window is resized, and GTK still derives the window's minimum width from
// the widest card's content, so nothing clips.
func SetHomogeneous(box uintptr) { boxHomogeneous(box, 1) }

// PackStart appends child to a box.
func PackStart(box, child uintptr, expand, fill bool, padding int) {
	if child == 0 {
		return
	}
	boxPackStart(box, child, b2i(expand), b2i(fill), uint32(padding))
}

// NewLabel creates a label from Pango markup, wrapped and centred. The caller
// is responsible for escaping any dynamic text it interpolates.
func NewLabel(markup string) uintptr {
	l := labelNew("")
	labelMarkup(l, markup)
	labelWrap(l, 1)
	labelJustify(l, JustifyCenter)
	return l
}

// NewText creates a wrapped, centred label from literal text. Nothing in s is
// interpreted as markup.
func NewText(s string) uintptr {
	l := labelNew("")
	labelText(l, s)
	labelWrap(l, 1)
	labelJustify(l, JustifyCenter)
	return l
}

// NewCell creates a table-cell label: literal text, aligned, and deliberately
// NOT wrapping. A wrapping label reports a near-zero minimum width, which lets
// its grid column collapse.
func NewCell(s string, align int) uintptr {
	l := labelNew("")
	labelText(l, s)
	widgetHalign(l, int32(align))
	widgetHexpand(l, 1)
	return l
}

// NewMarkupCell is NewCell for text that carries Pango markup. The caller is
// responsible for escaping anything dynamic.
func NewMarkupCell(markup string, align int) uintptr {
	l := labelNew("")
	labelMarkup(l, markup)
	widgetHalign(l, int32(align))
	widgetHexpand(l, 1)
	return l
}

// SetLabel replaces a label's literal text, nothing in it treated as markup.
// The forecast builds every label once, but a form does not: the readout beside
// the font-scale slider has to follow the thumb, so its text cannot be baked in
// when the window is built.
func SetLabel(label uintptr, text string) {
	if label == 0 {
		return
	}
	labelText(label, text)
}

// ShowAll makes a widget and everything under it visible. A window that is
// already on screen does not show children added afterwards - a fresh set of
// search result rows is invisible until this is called on the box holding them.
func ShowAll(widget uintptr) {
	if widget == 0 {
		return
	}
	widgetShowAll(widget)
}

// SetVExpand lets a widget claim vertical slack, which spreads table rows over
// the window instead of stacking them against the top edge.
func SetVExpand(widget uintptr) {
	if widget == 0 {
		return
	}
	widgetVexpand(widget, 1)
}

// SetHAlign overrides a widget's horizontal alignment. A GtkImage in an
// expanding cell stretches its allocation unless told to centre.
func SetHAlign(widget uintptr, align int) {
	if widget == 0 {
		return
	}
	widgetHalign(widget, int32(align))
}

// SetVAlign overrides a widget's vertical alignment. A column of labels packed
// against a tall neighbour - the header text beside a large icon - sits at the
// top edge unless told to centre.
func SetVAlign(widget uintptr, align int) {
	if widget == 0 {
		return
	}
	widgetValign(widget, int32(align))
}

// Grid is a GtkGrid. Column widths are deliberately not homogeneous: forcing
// them equal makes every column as wide as the widest header and puts a large
// floor under the window width.
type Grid uintptr

// NewGrid creates a grid with the given spacing in pixels.
func NewGrid(rowSpacing, colSpacing int) Grid {
	g := gridNew()
	gridRowSpace(g, uint32(rowSpacing))
	gridColSpace(g, uint32(colSpacing))
	return Grid(g)
}

// Attach places child at a column and row, spanning w columns and h rows.
func (g Grid) Attach(child uintptr, col, row, w, h int) {
	if child == 0 {
		return
	}
	gridAttach(uintptr(g), child, int32(col), int32(row), int32(w), int32(h))
}

// NewHSeparator creates a horizontal rule, for use as a full-width grid row.
func NewHSeparator() uintptr { return separatorNew(OrientationHorizontal) }

// grgba is scratch for a GdkRGBA, four doubles, at package scope for the same
// reason gvalue is. Only the GTK thread touches it.
var grgba [4]float64

// Foreground is the colour the theme draws label text in, which is what a
// generated icon must be tinted with to look like part of the window rather
// than pasted onto it. The widget has to belong to a window already - an orphan
// widget's style context resolves to GTK's built-in default instead of the
// active theme. Must be called on the GTK thread.
func (w Window) Foreground() color.NRGBA {
	styleGetColor(styleContext(uintptr(w)), stateNormal, &grgba[0])
	return color.NRGBA{
		R: uint8(grgba[0]*255 + 0.5),
		G: uint8(grgba[1]*255 + 0.5),
		B: uint8(grgba[2]*255 + 0.5),
		A: uint8(grgba[3]*255 + 0.5),
	}
}

// ScaleFactor is the display scale GTK will draw this window at: 1 on an
// ordinary screen, 2 on HiDPI. It picks which size of artwork to hand
// NewImageRGBAScaled.
//
// The same caveat as Foreground applies, for the same reason and with a
// different consequence: until the window is realized GTK has no GdkWindow to
// ask, and answers from the first monitor of the display instead of the one
// this window will actually land on. That is the right answer on a single-head
// or uniformly scaled desktop and can be wrong on a mixed-DPI multi-head one.
// Must be called on the GTK thread.
func (w Window) ScaleFactor() int { return int(widgetScale(uintptr(w))) }

func setStringProp(obj uintptr, name, val string) {
	valueInit(&gvalue[0], typeString)
	valueSetStr(&gvalue[0], val)
	objSetProp(obj, name, &gvalue[0])
	valueUnset(&gvalue[0])
}

// ShowError puts up a modal error dialog and returns immediately. It never
// calls gtk_dialog_run: that spins a nested main loop, and while the nested
// loop is up systray's gtk_main_quit is silently deferred, so the tray's Quit
// item stops working until the dialog is dismissed.
//
// The button label is passed in rather than using GTK_BUTTONS_CLOSE, whose text
// comes from GTK's own catalogue keyed to the system locale rather than to the
// language the user picked in Nimbus. Must be called on the GTK thread.
func ShowError(title, text, detail, button string) {
	dlg := msgDialogNew(0, dialogModal|dialogDestroyWith, messageError, buttonsNone, 0)
	if dlg == 0 {
		return
	}
	// GNOME's guidelines leave message dialogs untitled, but window managers
	// that draw a title bar render that as a blank strip.
	windowTitle(dlg, title)
	// The text properties assign the label directly. gtk_message_dialog_set_markup
	// would parse the string as Pango markup and silently blank the label on
	// any text containing an ampersand.
	setStringProp(dlg, "text", text)
	if detail != "" {
		setStringProp(dlg, "secondary-text", detail)
	}
	dialogAddBtn(dlg, button, responseClose)
	dialogDefResp(dlg, responseClose)
	signalConnect(dlg, "response", destroyAddr, dlg, 0, connectSwapped)
	widgetShowAll(dlg)
}

// Image is a non-premultiplied RGBA buffer ready to hand to gdk-pixbuf.
type Image struct {
	Pix    []byte
	W, H   int
	Stride int
}

// newPixbuf wraps pixels in a GdkPixbuf. The buffer is retained because
// gdk_pixbuf_new_from_data does not copy it.
func newPixbuf(img Image) uintptr {
	if len(img.Pix) == 0 {
		return 0
	}
	retainMu.Lock()
	retained[&img.Pix[0]] = img.Pix
	retainMu.Unlock()

	return pixbufFromData(&img.Pix[0], colorspaceRGB, 1, 8,
		int32(img.W), int32(img.H), int32(img.Stride), 0, 0)
}

// NewImageRGBA wraps a runtime-generated RGBA buffer in a GtkImage.
func NewImageRGBA(pix []byte, w, h, stride int) uintptr {
	pb := newPixbuf(Image{Pix: pix, W: w, H: h, Stride: stride})
	if pb == 0 {
		return 0
	}
	return imageFromPixbufOwned(pb)
}

// imageFromPixbufOwned builds the GtkImage and drops the caller's reference.
// gdk_pixbuf_new_from_data hands back a pixbuf at refcount 1 and GtkImage takes
// its own, so without this every icon orphans a GdkPixbuf for the life of the
// process - once per icon per window open.
func imageFromPixbufOwned(pb uintptr) uintptr {
	img := imageFromPixbuf(pb)
	objUnref(pb)
	return img
}

// NewImageRGBAScaled wraps a buffer that holds scale times as many pixels per
// side as the layout should reserve: pass the 128px artwork with scale 2 to get
// a 64pt image drawn at full resolution.
//
// gtk_image_new_from_pixbuf is not HiDPI-correct. It treats the pixbuf as
// logical pixels, so on a scale-2 display GTK doubles it and the result is
// visibly blurry no matter how large the source was. A cairo surface can carry
// the scale as part of its identity, and GTK then lays the image out at
// pixels/scale while drawing every device pixel. scale <= 1 takes the plain
// pixbuf path, so an ordinary screen behaves exactly as NewImageRGBA does.
//
// The surface is released straight after the GtkImage is built. That is not
// premature: gtk_image_new_from_surface takes its own reference (measured going
// from 1 to 2, and back to 1 here), and unlike a pixbuf - which points at the
// Go buffer - the surface holds its own premultiplied copy of the pixels, so
// keeping the extra reference would leak a full bitmap per icon per window
// open. Must be called on the GTK thread.
func NewImageRGBAScaled(pix []byte, w, h, stride, scale int) uintptr {
	pb := newPixbuf(Image{Pix: pix, W: w, H: h, Stride: stride})
	if pb == 0 {
		return 0
	}
	if scale <= 1 {
		return imageFromPixbufOwned(pb)
	}
	surface := surfaceFromPB(pb, int32(scale), 0)
	if surface == 0 {
		return imageFromPixbufOwned(pb)
	}
	img := imageFromSurf(surface)
	surfaceDestroy(surface)
	objUnref(pb)
	return img
}

// SetDefaultIcons installs the icon list every window inherits: the title bar,
// the window switcher and the taskbar each pick whichever size fits the slot
// they are drawing, so supplying several beats letting the window manager
// rescale one. Must be called on the GTK thread.
func SetDefaultIcons(imgs ...Image) {
	var list uintptr
	for _, img := range imgs {
		if pb := newPixbuf(img); pb != 0 {
			list = listAppend(list, pb)
		}
	}
	if list != 0 {
		setDefaultIcons(list)
	}
}

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

const keyEscape = 0xff1b // GDK_KEY_Escape

// ConnectEvent attaches fn to a signal carrying a GdkEvent. Returning true
// stops the signal. Must be called on the GTK thread.
func ConnectEvent(obj uintptr, signal string, fn func(event uintptr) bool) {
	cbMu.Lock()
	cbSeq++
	id := cbSeq
	eventFns[id] = fn
	cbMu.Unlock()
	signalConnect(obj, signal, eventTrampoline, id, 0, 0)
}

// NewPanel creates an undecorated, always-on-top window that stays out of the
// taskbar and the pager and follows the user across workspaces - a tray popup
// rather than a document window.
//
// It is deliberately a normal toplevel with the decorations turned off, not a
// GTK_WINDOW_POPUP and not a UTILITY or DOCK type hint. Those are override
// redirect or otherwise unfocusable, and under Marco a window carrying any of
// them receives no key events at all - which would leave the panel with no way
// to be closed, since it has no title bar either. The two skip hints remove the
// taskbar button on their own and leave focus intact.
//
// The caller is responsible for dismissal: see OnEscape and OnFocusOut.
func NewPanel(title string, w, h int) Window {
	win := windowNew(WindowToplevel)
	windowTitle(win, title) // the WM still uses it for alt-tab
	windowSize(win, int32(w), int32(h))
	windowDecorated(win, 0)
	windowSkipTask(win, 1)
	windowSkipPager(win, 1)
	windowKeepAbove(win, 1)
	windowStick(win)
	// No gtk_window_set_position: a placement hint would overrule Move.
	return Window(win)
}

// SetTranslucent gives the window an RGBA visual so CSS alpha composites
// against the desktop instead of black, and reports whether it took effect.
//
// It must be called before the window is realised, because a visual is fixed
// when the underlying GdkWindow is created - there is no way to add an alpha
// channel to a window that is already on screen.
//
// The compositor check is the whole point. gdk_screen_get_rgba_visual returns
// the same non-NULL depth-32 visual whether or not a compositing manager is
// running, because the visual belongs to the X server rather than the session,
// so testing it alone is exactly how an app ends up painting a solid black
// rectangle where its transparency should be. Callers must style the window
// from what this returns, not from what they hoped for.
func (w Window) SetTranslucent() bool {
	screen := screenDefault()
	if screen == 0 || screenComposited(screen) == 0 {
		return false
	}
	visual := screenRGBAVisual(screen)
	if visual == 0 {
		return false
	}
	widgetSetVisual(uintptr(w), visual)
	return true
}

// OnEscape closes the window when Escape is pressed. For a panel with no title
// bar this is the dismissal path that always works, whatever the window manager
// does with focus.
func (w Window) OnEscape(fn func()) {
	ConnectEvent(uintptr(w), "key-press-event", func(event uintptr) bool {
		var keyval uint32
		if eventKeyval(event, &keyval) == 0 || keyval != keyEscape {
			return false
		}
		fn()
		return true
	})
}

// OnFocusOut runs fn when the window loses focus, which is how a panel gets
// dismissed by clicking elsewhere. It fires for any window that takes focus,
// including another of this application's own, so it is a convenience on top of
// OnEscape rather than the only way out.
func (w Window) OnFocusOut(fn func()) {
	ConnectEvent(uintptr(w), "focus-out-event", func(uintptr) bool {
		fn()
		return false
	})
}

// ShowContent makes the widget tree visible while the toplevel itself stays
// unmapped. That is what makes Size answer truthfully: GTK measures a
// content-hugging layout from its visible children, so a window whose children
// are still hidden reports a placeholder size, and a panel positioned from that
// number lands in the wrong place. Show the content, measure, move, and only
// then call Show.
func (w Window) ShowContent(child uintptr) { widgetShowAll(child) }

// Size reports the window's current size. Meaningful only after ShowContent.
func (w Window) Size() (int, int) {
	var width, height int32
	windowGetSize(uintptr(w), &width, &height)
	return int(width), int(height)
}

// Move places the window's top-left corner. X11 only: Wayland gives a client no
// way to position its own toplevel, and the call is silently ignored there.
func (w Window) Move(x, y int) { windowMove(uintptr(w), int32(x), int32(y)) }

// Show maps the window itself, without touching its children.
func (w Window) Show() { widgetShow(uintptr(w)) }

// NewButton creates a push button that runs fn when clicked.
func NewButton(label string, fn func()) uintptr {
	b := buttonNew(label)
	Connect(b, "clicked", fn)
	return b
}

// PointerPosition reports where the pointer is in root coordinates. Must be
// called on the GTK thread - this reaches into GDK, which is not thread-safe.
// Returns ok=false on Wayland, where a client cannot see global coordinates.
func PointerPosition() (x, y int, ok bool) {
	display := displayDefault()
	if display == 0 {
		return 0, 0, false
	}
	seat := displaySeat(display)
	if seat == 0 {
		return 0, 0, false
	}
	pointer := seatPointer(seat)
	if pointer == 0 {
		return 0, 0, false
	}
	var screen uintptr
	var px, py int32
	devicePosition(pointer, &screen, &px, &py)
	return int(px), int(py), true
}

// Rect is a GdkRectangle: four ints, in root coordinates.
type Rect struct{ X, Y, W, H int }

// WorkAreaAt returns the usable area of the monitor containing the given point:
// the monitor's geometry minus whatever the panels and docks have reserved.
// This is not the same rectangle as the monitor itself, and using the wrong one
// puts a corner-hugging window underneath the desktop panel.
func WorkAreaAt(x, y int) (Rect, bool) {
	display := displayDefault()
	if display == 0 {
		return Rect{}, false
	}
	monitor := monitorAtPoint(display, int32(x), int32(y))
	if monitor == 0 {
		return Rect{}, false
	}
	var r [4]int32
	monitorWorkarea(monitor, &r[0])
	if r[2] <= 0 || r[3] <= 0 {
		return Rect{}, false
	}
	return Rect{X: int(r[0]), Y: int(r[1]), W: int(r[2]), H: int(r[3])}, true
}

// SetSensitive greys a widget out and stops it reacting. It is how Search says
// a lookup is already in flight, instead of letting a second click start
// another one over the top of it.
func SetSensitive(widget uintptr, on bool) {
	if widget == 0 {
		return
	}
	widgetSensitive(widget, b2i(on))
}

// SetHExpand lets a widget claim horizontal slack, the mirror of SetVExpand. An
// entry has a modest natural width, so without this it sits at that width with
// the rest of its grid column empty beside it.
func SetHExpand(widget uintptr) {
	if widget == 0 {
		return
	}
	widgetHexpand(widget, 1)
}

// NewFrame groups child inside a titled border - the GTK spelling of the group
// boxes the Windows settings dialog draws around each cluster of options, and
// of the fieldsets the browser fallback uses. Grouping this way rather than
// with NewHSeparator keeps the label attached to what it labels, so the form
// still reads correctly when GTK reflows it.
func NewFrame(title string, child uintptr) uintptr {
	f := frameNew(title)
	if child != 0 {
		containerAdd(f, child)
	}
	return f
}

// NewScrolled puts child in a viewport that scrolls vertically and is at least
// minHeight tall.
//
// Horizontal scrolling is off deliberately. With NEVER, GTK asks for the width
// the widest row needs and the window is sized to fit its content; with
// AUTOMATIC a long city name would instead hide behind a scrollbar the user has
// to discover. The minimum content height does the other half of the job: it
// stops the list collapsing to nothing while it is empty, and stops twenty
// results from growing the window past the screen.
//
// A GtkBox is not scrollable, and gtk_container_add wraps such a child in the
// GtkViewport it needs. That has been automatic since GTK 3.8; this package
// needs a far newer GTK 3 than that for other reasons.
func NewScrolled(child uintptr, minHeight int) uintptr {
	sw := scrolledNew(0, 0)
	scrolledPolicy(sw, policyNever, policyAutomatic)
	scrolledMinH(sw, int32(minHeight))
	if child != 0 {
		containerAdd(sw, child)
	}
	return sw
}

// NewListRow creates one row of a pick-list: a full-width button that runs fn
// when it is clicked or activated from the keyboard.
//
// The rows are buttons in a plain GtkBox rather than a GtkListBox, and the
// reason is the callback budget. "clicked" is void(GtkButton*, gpointer) and
// rides the two-argument void trampoline this package already owns, whereas
// GtkListBox's row-selected is void(GtkListBox*, GtkListBoxRow*, gpointer) and
// matches neither existing trampoline - it would cost a fourth NewCallback,
// permanently, for one signal. Buttons also report the user's intent more
// precisely: row-selected fires again with a NULL row when the selection is
// cleared and once more each time the list is repopulated, so a search that
// returns results would look like a pick nobody made.
//
// The handler is scoped to the row because a results list is rebuilt on every
// search - see ConnectScoped.
func NewListRow(label string, fn func()) uintptr {
	b := buttonNew(label)
	ConnectScoped(b, "clicked", fn)
	return b
}

// ClearContainer destroys every child of a container, which is how the results
// list is emptied before the next search fills it.
//
// gtk_container_get_children hands back a list the caller owns even though the
// widgets in it are only borrowed, so the list - and just the list - is freed
// here. Destroying a child mutates the container's own list and not this copy,
// which is what makes it safe to walk one while destroying the other.
//
// gtk_widget_destroy is handed to g_list_foreach as a bare GFunc, the same idiom
// ShowError uses to close a dialog: GFunc passes a second gpointer that
// gtk_widget_destroy never reads, and an ignored extra argument costs nothing on
// the ABIs Nimbus builds for. It keeps the walk inside GLib, so clearing a list
// spends no Go callback at all.
func ClearContainer(container uintptr) {
	if container == 0 {
		return
	}
	children := containerChild(container)
	if children == 0 {
		return
	}
	listForeach(children, destroyAddr, 0)
	listFree(children)
}

// Entry is a GtkEntry: one line of editable text. All methods must be called on
// the GTK thread.
type Entry uintptr

// NewEntry creates a text entry holding text.
//
// Enter inside the entry emits "activate", which carries nothing beyond the
// widget itself and so connects with plain Connect - that is how the city field
// starts a search without a detour through the Search button.
func NewEntry(text string) Entry {
	e := entryNew()
	entrySetText(e, text)
	return Entry(e)
}

// Text reads the entry back. GTK stores UTF-8 and owns the buffer it returns;
// purego copies it to the terminating NUL, so the bytes arrive exactly as the
// user typed them - Ukrainian, apostrophes and all - and nothing here may free
// it.
func (e Entry) Text() string { return entryGetText(uintptr(e)) }

// SetText replaces the entry's contents, which is what filling the city, latitude
// and longitude fields from a picked search result amounts to.
func (e Entry) SetText(s string) { entrySetText(uintptr(e), s) }

// SetActive drives any GtkToggleButton - a check button, or one radio button of
// a group. Setting a radio button clears whichever of its group was set before.
func SetActive(toggle uintptr, on bool) {
	if toggle == 0 {
		return
	}
	toggleSetActive(toggle, b2i(on))
}

// IsActive reports whether a GtkToggleButton is pressed in.
func IsActive(toggle uintptr) bool {
	return toggle != 0 && toggleGetActive(toggle) != 0
}

// RadioGroup is one set of mutually exclusive radio buttons, in the order their
// labels were given. It is a slice so a caller can range over it and pack the
// buttons into whatever layout the form wants; a horizontal row is only the
// most common one.
type RadioGroup []uintptr

// NewRadioGroup builds a radio button per label, all in one group, and selects
// the active-th.
//
// GTK3 threads a group through the buttons themselves instead of through a
// container: each button is created with the group its predecessor already
// belongs to, and NULL starts a fresh one. gtk_radio_button_get_group returns
// that group as a GSList which GTK owns and rewrites as members join - it must
// never be freed, and it is in the REVERSE of creation order, which is why the
// caller's index is answered from this slice and never from the list itself.
//
// The first button of a group is active from the moment it exists, so a group
// that should open on any other entry has to say so; an out-of-range index
// leaves that default in place rather than panicking, which is what a config
// file holding a value this build no longer offers should do.
//
// Note for anyone connecting "toggled": it fires twice per change, once for the
// button going off and once for the one coming on, so a handler has to consult
// IsActive rather than assume it was called about a selection.
func NewRadioGroup(labels []string, active int) RadioGroup {
	g := make(RadioGroup, 0, len(labels))
	var group uintptr // NULL: the first button starts a new group
	for _, label := range labels {
		b := radioNewLabel(group, label)
		group = radioGetGroup(b)
		g = append(g, b)
	}
	g.SetActive(active)
	return g
}

// Active reports which button is selected, or -1 in the impossible case that
// none is.
func (g RadioGroup) Active() int {
	for i, b := range g {
		if toggleGetActive(b) != 0 {
			return i
		}
	}
	return -1
}

// SetActive selects the i-th button; an index outside the group is ignored.
func (g RadioGroup) SetActive(i int) {
	if i < 0 || i >= len(g) {
		return
	}
	toggleSetActive(g[i], 1)
}

// Slider is a GtkScale over an integer range. All methods must be called on the
// GTK thread.
type Slider uintptr

// NewSlider creates a horizontal slider stepping by one over min..max.
//
// GTK keeps the position as a double, and two calls are what make the control
// behave like the integer it represents. The scale draws no number of its own,
// because "47" is not what the user should read where Nimbus means "47%" - the
// caller owns that label. Turning the number off is also what breaks GTK's
// rounding: gtk_scale_set_digits only forwards to the range while draw-value is
// set, so round-digits is set directly instead. Measured on this machine, a
// thumb dragged four pixels reports 42.340659, 43.156593, 43.972527, 44.788462
// with digits alone - the same values as with no rounding call at all - and 43,
// 44, 45 once round-digits is set.
func NewSlider(min, max, value int) Slider {
	s := scaleNewRange(OrientationHorizontal, float64(min), float64(max), 1)
	scaleDrawValue(s, 0)
	rangeRoundDigits(s, 0)
	rangeSetValue(s, float64(value))
	return Slider(s)
}

// Value reports the slider position. It rounds rather than truncates: the
// adjustment underneath is floating point whatever round-digits says, and
// truncation would turn a 47 that arrived as 46.999999 into 46.
func (s Slider) Value() int { return int(math.Round(rangeGetValue(uintptr(s)))) }

// SetValue moves the thumb. It emits value-changed like a drag does, so a
// handler installed with OnChange will see it - which is worth knowing before
// calling this from inside one.
func (s Slider) SetValue(v int) { rangeSetValue(uintptr(s), float64(v)) }

// OnChange runs fn with the new value every time the slider moves, including
// while the thumb is still held. That is the point of it: dragging the font
// scale regenerates the tray icon live, so the user sees the size they are
// choosing before anything is saved.
//
// GTK emits value-changed for every integer the thumb crosses, so fn must be
// cheap or must coalesce its own work. The signal is void(GtkRange*, gpointer)
// and rides the existing void trampoline, costing no part of the NewCallback
// budget.
func (s Slider) OnChange(fn func(value int)) {
	Connect(uintptr(s), "value-changed", func() { fn(s.Value()) })
}

// Combo is a GtkComboBoxText: a dropdown of plain strings addressed by index.
// All methods must be called on the GTK thread.
type Combo uintptr

// NewCombo creates a dropdown holding items, showing the active-th.
func NewCombo(items []string, active int) Combo {
	c := comboTextNew()
	for _, item := range items {
		comboTextAppend(c, item)
	}
	comboSetActive(c, int32(active))
	return Combo(c)
}

// Active reports the selected index, or -1 when nothing is selected - which is
// what GTK answers for an empty dropdown and what an out-of-range SetActive
// leaves behind, so a caller mapping the index back to its own table must check
// the range rather than trust it.
func (c Combo) Active() int { return int(comboGetActive(uintptr(c))) }

// SetActive selects the i-th entry; -1 clears the selection.
func (c Combo) SetActive(i int) { comboSetActive(uintptr(c), int32(i)) }
