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
	"log"
	"math"
	"sync"
	"time"

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

	// GdkEventMask, and GDK's pointer button numbering. A GtkWindow subscribes
	// to no button presses of its own, so the mask is what makes a press on the
	// window body observable at all - see DragOnPress.
	buttonPressMask = 1 << 8 // GDK_BUTTON_PRESS_MASK
	primaryButton   = 1      // GDK_BUTTON_PRIMARY

	// GdkModifierType. The same number as GDK_BUTTON_PRESS_MASK above and a
	// different enumeration entirely: this one appears in the modifier mask
	// gdk_window_get_device_position writes, where it means the primary button
	// is physically down at this instant - see PrimaryButtonHeld.
	button1Mask = 1 << 8 // GDK_BUTTON1_MASK

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
	widgetSizeReq func(uintptr, int32, int32)
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
	monitorGeometry func(uintptr, *int32)
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
	timeoutAdd  func(uint32, uintptr, uintptr) uint32
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
	scrolledMaxH     func(uintptr, int32)
	scrolledNatural  func(uintptr, int32)
	scrolledPolicy   func(uintptr, int32, int32)
	scrolledMinH     func(uintptr, int32)
	frameNew         func(string) uintptr
	widgetSensitive  func(uintptr, int32)
	containerChild   func(uintptr) uintptr

	// The draggable, pinnable panel: a press on the window body starts a window
	// manager move, and the position it ends up at is read back so it can be
	// remembered. Unlike everything above, these are bound with bindOptional and
	// are nil on a GTK that does not export them, so every caller MUST nil-check
	// them - see DragOnPress, Position and NewCheck.
	widgetAddEvents func(uintptr, int32)
	windowMoveDrag  func(uintptr, int32, int32, int32, uint32)
	eventButton     func(uintptr, *uint32) int32
	eventRoot       func(uintptr, *float64, *float64) int32
	eventTime       func(uintptr) uint32
	windowGetPos    func(uintptr, *int32, *int32)
	checkNewLabel   func(string) uintptr

	// The other half of the draggable panel: whether the primary button is still
	// held, which is the only honest answer to "is the window manager still
	// moving this window" - GTK emits no signal for the end of a move. Bound
	// optionally and nil-checked in PrimaryButtonHeld, which reports "cannot
	// tell" when either is missing.
	screenRootWindow func(uintptr) uintptr
	windowDevicePos  func(uintptr, uintptr, *int32, *int32, *uint32) uintptr
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
	purego.RegisterLibFunc(&widgetSizeReq, gtk, "gtk_widget_set_size_request")
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
	purego.RegisterLibFunc(&monitorGeometry, gtk, "gdk_monitor_get_geometry")
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
	purego.RegisterLibFunc(&scrolledMaxH, gtk, "gtk_scrolled_window_set_max_content_height")
	purego.RegisterLibFunc(&scrolledNatural, gtk, "gtk_scrolled_window_set_propagate_natural_height")
	purego.RegisterLibFunc(&frameNew, gtk, "gtk_frame_new")
	purego.RegisterLibFunc(&widgetSensitive, gtk, "gtk_widget_set_sensitive")
	purego.RegisterLibFunc(&containerChild, gtk, "gtk_container_get_children")

	// The pinned-panel bindings. Every one of them has existed since GTK 2, so a
	// library missing one is not a case anyone expects to meet - but the cost of
	// surviving it is a nil check at three call sites, and the cost of not
	// surviving it is the entire GTK UI falling back to the browser over a panel
	// that cannot be dragged. bindOptional is what buys that, so these do not go
	// through RegisterLibFunc.
	bindOptional(&widgetAddEvents, gtk, "gtk_widget_add_events")
	bindOptional(&windowMoveDrag, gtk, "gtk_window_begin_move_drag")
	bindOptional(&eventButton, gtk, "gdk_event_get_button")
	bindOptional(&eventRoot, gtk, "gdk_event_get_root_coords")
	bindOptional(&eventTime, gtk, "gdk_event_get_time")
	bindOptional(&windowGetPos, gtk, "gtk_window_get_position")
	bindOptional(&checkNewLabel, gtk, "gtk_check_button_new_with_label")
	bindOptional(&screenRootWindow, gtk, "gdk_screen_get_root_window")
	bindOptional(&windowDevicePos, gtk, "gdk_window_get_device_position")

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
	purego.RegisterLibFunc(&timeoutAdd, glib, "g_timeout_add")
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

// bindOptional binds a symbol Nimbus can manage without, leaving fptr nil when
// the loaded library does not export it.
//
// purego.RegisterLibFunc panics on a missing symbol and load's recover turns
// that panic into loadErr, which aborts the rest of load and makes Ready report
// false. That is exactly right for a symbol without which no window can be
// drawn, and exactly wrong for one that costs a single feature: it would drop the
// user into the browser fallback because a panel could not be dragged. Dlsym is
// the same escape hatch destroyAddr already uses, for the same reason - it
// reports rather than panics.
//
// The failure is logged because nothing else can report it. The symbol name is a
// string literal, so a typo in one is invisible to the compiler, to go vet and to
// every test - the func var is simply nil and the feature quietly is not there.
// The loudest case is not the drag: NewCheck returns Check(0), and the settings
// window then cannot change ForecastPinned at all, for anyone, forever, with no
// error anywhere. A line on stderr is what turns that into something a bug report
// can carry.
//
// Anything bound this way MUST be nil-checked before it is called.
func bindOptional(fptr any, lib uintptr, name string) {
	addr, err := purego.Dlsym(lib, name)
	if err == nil && addr == 0 {
		err = errors.New("dlsym returned a NULL address")
	}
	if err != nil {
		log.Printf("gtk: optional symbol %s is missing, the feature it serves is off: %v", name, err)
		return
	}
	purego.RegisterFunc(fptr, addr)
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

// After runs fn on the GTK main loop once, no sooner than d from now. It is how
// a caller asks GDK something again in a moment - see PrimaryButtonHeld, whose
// caller has to keep asking because GTK emits no signal for the end of a window
// manager move.
//
// It costs no new trampoline, which is the whole reason it is g_timeout_add and
// not a goroutine that sleeps and calls Invoke: g_timeout_add takes the same
// GSourceFunc as g_idle_add, and dispatchIdle already returns G_SOURCE_REMOVE,
// so the source removes itself after its single call. The fixed callback budget
// stays at three. A timeout also sits at G_PRIORITY_DEFAULT rather than
// G_PRIORITY_DEFAULT_IDLE, so a busy event stream - a drag, which is exactly the
// case this serves - cannot defer it indefinitely.
//
// Timing is best effort, like every GLib timeout: it runs no earlier than d and
// later if the loop is busy, so callers must treat it as "ask again soon" and
// never as a deadline. d is rounded up to whole milliseconds because
// g_timeout_add counts in them, and rounding down would turn a sub-millisecond
// request into a zero-interval source that spins the loop.
//
// Must be called on the GTK thread.
func After(d time.Duration, fn func()) {
	ms := (d + time.Millisecond - 1) / time.Millisecond
	if ms < 0 {
		ms = 0
	}
	timeoutAdd(uint32(ms), idleTrampoline, register(idleFns, fn))
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

// ConnectEventScoped is ConnectEvent for a handler that should not outlive the
// widget it is attached to. It is what every window-level event handler here uses,
// because a window is the most frequently created widget in this program: the
// forecast panel is opened and closed all session, and each opening used to leave
// its Escape, focus-in, focus-out and button-press closures in eventFns for the
// life of the process, along with everything those closures captured.
//
// Like ConnectScoped it costs no extra trampoline: the release is a ConnectOnce on
// "destroy", which deletes its own entry when it fires. Prefer plain ConnectEvent
// only for a widget that genuinely lives as long as the process.
func ConnectEventScoped(obj uintptr, signal string, fn func(event uintptr) bool) {
	cbMu.Lock()
	cbSeq++
	id := cbSeq
	eventFns[id] = fn
	cbMu.Unlock()
	signalConnect(obj, signal, eventTrampoline, id, 0, 0)
	ConnectOnce(obj, "destroy", func() {
		cbMu.Lock()
		delete(eventFns, id)
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
	// cssMu must not be held across anything that can emit a signal Nimbus
	// handles. It is taken on the GTK thread and everything under it is a GTK
	// call, which is exactly the shape that deadlocked the settings window: a
	// handler re-entering the guard it is already inside blocks on it forever, on
	// the one thread the whole toolkit depends on. Nothing reaches it today; the
	// note is here so the next thing added under the lock is weighed first.
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

// SetHAlign overrides a widget's horizontal alignment. A GtkImage in an
// expanding cell stretches its allocation unless told to centre.
func SetHAlign(widget uintptr, align int) {
	if widget == 0 {
		return
	}
	widgetHalign(widget, int32(align))
}

// SetSizeRequest asks for a minimum size in logical pixels. Pass -1 for a
// dimension that should keep its natural size.
//
// It is a MINIMUM, not a size: GTK gives a widget more when its container has
// room and the widget is set to expand, and never less than the content needs, so
// a request narrower than the label will simply be ignored. That makes it the
// right tool for "this button should be a third of the window wide" and the wrong
// one for pinning a widget to an exact size.
func SetSizeRequest(widget uintptr, w, h int) {
	if widget == 0 {
		return
	}
	widgetSizeReq(widget, int32(w), int32(h))
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

// ShowError puts up an error dialog and returns immediately.
//
// It never calls gtk_dialog_run: that spins a nested main loop, and while the
// nested loop is up gtk_main_quit is silently deferred, so the tray's Quit item
// stops working until the dialog is dismissed.
//
// It is NOT modal, and that is a deliberate change from what it was. GTK modality
// on a dialog with no transient parent adds an application-wide gtk_grab_add, and
// GTK then routes input to the grab widget only: every other window this process
// owns stops taking clicks, and gtk_main.c ignores their WM close buttons too. The
// dialog is small and the window manager places it wherever it likes, so on a
// large desktop the user gets an application that appears frozen with no visible
// cause. Nothing here needs an answer before the program can continue, which is
// the only thing modality is for.
//
// At most one is on screen at a time. Errors here arrive in bursts - one per
// failed fetch, and a weather service that is down fails every attempt - and a
// stack of identical dialogs is worse than the first one: each has to be
// dismissed separately, and until they are, the newest hides that the older ones
// are the same message.
//
// The button label is passed in rather than using GTK_BUTTONS_CLOSE, whose text
// comes from GTK's own catalogue keyed to the system locale rather than to the
// language the user picked in Nimbus. Must be called on the GTK thread.
func ShowError(title, text, detail, button string) {
	if errorDialog != 0 {
		// Update the text, do not just raise it: a second failure can carry a
		// different message, and without this only the first one would ever be
		// read. Not modal any more either, so the window manager may have it
		// behind something and gtk_window_present may change nothing visible -
		// which makes writing the current message into it the only thing that is
		// guaranteed to be seen when it does come forward.
		setStringProp(errorDialog, "text", text)
		if detail != "" {
			setStringProp(errorDialog, "secondary-text", detail)
		}
		windowPresent(errorDialog)
		return
	}
	dlg := msgDialogNew(0, dialogDestroyWith, messageError, buttonsNone, 0)
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
	// Cleared on destroy so the next failure can put up a fresh dialog. Read and
	// written only on the GTK thread, like the rest of this function.
	errorDialog = dlg
	ConnectOnce(dlg, "destroy", func() { errorDialog = 0 })
	widgetShowAll(dlg)
}

// errorDialog is the error dialog currently on screen, or 0. GTK thread only.
var errorDialog uintptr

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

// newPanelWindow creates the toplevel that both looks of the forecast panel are
// built on, which is everything about it except whether the window manager frames
// it. NewPanel turns the frame off; NewFramedPanel leaves it on.
//
// The hints live here rather than in each constructor because not one of them
// depends on the decorations: in either look this is a tray popup, so it stays
// above other windows (an explicit request of its own, not a side effect of being
// undecorated), keeps its button out of the taskbar and the pager, and sticks to
// every workspace so it follows the user. Copying the list into a second
// constructor is how the two end up a hint apart after the next change to either.
//
// Every call below is a property set on a window that is not realised yet, so GTK
// stores the value and applies it when the GdkWindow is created. The order they
// are made in therefore carries no meaning, which is what lets the one call that
// differs be left to the caller.
func newPanelWindow(title string, w, h int) uintptr {
	win := windowNew(WindowToplevel)
	windowTitle(win, title) // the WM uses it for alt-tab, and for the title bar
	windowSize(win, int32(w), int32(h))
	windowSkipTask(win, 1)
	windowSkipPager(win, 1)
	windowKeepAbove(win, 1)
	windowStick(win)
	// No gtk_window_set_position: a placement hint would overrule Move.
	return win
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
	win := newPanelWindow(title, w, h)
	windowDecorated(win, 0)
	return Window(win)
}

// NewFramedPanel is NewPanel with the window manager's frame left on: a title
// bar, a border, and the manager's own close button. It is what the forecast
// panel's system look is built from, where the panel is meant to read as an
// ordinary application window instead of a sheet floating over the desktop.
//
// It differs from NewPanel in one call it does NOT make,
// gtk_window_set_decorated(FALSE) - GTK's default is decorated, which is the same
// default NewWindow already relies on for the settings and About windows. Every
// hint newPanelWindow sets is kept, and none of them is a consequence of being
// undecorated: a framed panel is still a tray popup, so it still belongs above
// other windows, out of the taskbar and the pager, and stuck across workspaces.
// The user asked for a title bar, not for a different window.
//
// Two caller obligations follow from the frame. Do not call SetTranslucent on
// this window - an RGBA visual exists to composite the Modern look's alpha
// against the desktop, and a framed window is opaque by definition. And wire
// OnDeleteEvent: the frame's close button does not go through the same path as
// the in-window one, and on its own it reports nothing.
func NewFramedPanel(title string, w, h int) Window {
	return Window(newPanelWindow(title, w, h))
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
	ConnectEventScoped(uintptr(w), "key-press-event", func(event uintptr) bool {
		var keyval uint32
		if eventKeyval(event, &keyval) == 0 || keyval != keyEscape {
			return false
		}
		fn()
		return true
	})
}

// OnDeleteEvent runs fn when the window manager asks the window to close: the
// title bar's close button, the frame menu, or a session shutdown. Only a framed
// window can reach it - an undecorated panel has nothing that emits it - so it is
// the system look's counterpart to the in-window close button, which the title bar
// replaces.
//
// It exists because the default behaviour reports nothing. GTK's own
// "delete-event" handler destroys the window directly, which never runs the
// panel's dismissal, so the position the user dragged the panel to is not written
// out. That is harmless for an undecorated panel, where a window manager close is
// something that essentially never happens, and wrong for a framed one, where the
// title bar is the ordinary way the panel gets closed - it would lose the
// remembered position on almost every close.
//
// The handler returns TRUE, which stops that default destroy. fn therefore owns
// destroying the window: an fn that does not is a window whose close button does
// nothing. Hand it the same dismissal OnEscape gets and every exit reports alike.
//
// Scoped to the window like every other window-level handler here, because the
// panel is opened and closed all session and an unscoped registration would leave
// one closure per open in eventFns for the life of the process.
func (w Window) OnDeleteEvent(fn func()) {
	ConnectEventScoped(uintptr(w), "delete-event", func(uintptr) bool {
		fn()
		return true
	})
}

// OnFocusIn runs fn when the window gains focus. A panel uses it to decide that
// a later focus LOSS is genuine: an undecorated window is not guaranteed to be
// given focus when it is mapped, so a focus-out can arrive before the window was
// ever focused at all.
func (w Window) OnFocusIn(fn func()) {
	ConnectEventScoped(uintptr(w), "focus-in-event", func(uintptr) bool {
		fn()
		return false
	})
}

// OnFocusOut runs fn when the window loses focus, which is how a panel gets
// dismissed by clicking elsewhere. It fires for any window that takes focus,
// including another of this application's own, so it is a convenience on top of
// OnEscape rather than the only way out.
func (w Window) OnFocusOut(fn func()) {
	ConnectEventScoped(uintptr(w), "focus-out-event", func(uintptr) bool {
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

// gbutton, groot, gpos, gdevXY and gmask are scratch out-parameters for the
// accessors that write through a pointer instead of returning:
// gdk_event_get_button takes a guint*, gdk_event_get_root_coords two gdouble*,
// gtk_window_get_position two gint*, gdk_window_get_device_position two gint*
// and a GdkModifierType*. They live at package scope for the same reason gvalue
// does - a Go stack moves when it grows, and a pointer into one is not an
// address to hand to C - and each call gets its own so no accessor can scribble
// over a value another is still reading. Only the GTK thread touches them.
var (
	gbutton uint32
	groot   [2]float64
	gpos    [2]int32
	gdevXY  [2]int32
	gmask   uint32
)

// DragOnPress makes a press anywhere on the window body start a window manager
// move. It is how a panel with no title bar can be moved at all, and it works
// whether or not the panel is pinned.
//
// gtk_widget_add_events is load-bearing: a GtkWindow does not ask GDK for button
// presses, so without GDK_BUTTON_PRESS_MASK the handler is connected and then
// never fires once.
//
// The event is read through gdk_event_get_button, gdk_event_get_root_coords and
// gdk_event_get_time, never by reaching into GdkEventButton at a struct offset -
// offsets into a GDK struct are how this package got burned once already, and the
// accessors are what OnEscape already does with gdk_event_get_keyval.
//
// A press on the close button never arrives here, and that is precisely what
// keeps the button working: a GtkButton has its own GdkWindow and consumes its
// own press, so button-press-event on the toplevel only fires where no child
// claimed the event. The drag therefore hit-tests nothing, and pressing the
// close button cannot start a drag. For the same reason the handler returns
// false: an event that reached the toplevel was refused by everything below it,
// so there is nothing to be gained by stopping the chain here.
//
// started, if not nil, runs just before a press is handed to the window manager.
// It says that a press became a drag, which is NOT the same as saying the window
// moved - a bare click on the panel body reaches here too, and so does a drag the
// user cancels with Escape. So it is only half of "should the position be
// persisted"; the caller pairs it with Position, recording the position here and
// comparing it at close, and persists nothing unless the two differ. Reading both
// through Position is also what makes this correct on Wayland, where it always
// answers 0,0: the two reads compare equal, so nothing is written.
//
// It fires for exactly the presses that become drags, so not for button 2 or 3,
// and not at all when a binding is missing. It runs on the GTK thread inside the
// event handler, so it must not block; recording a couple of ints is what it is
// for.
//
// Must be called on the GTK thread. Degrades to "cannot drag" rather than
// crashing when any symbol dragReady checks is missing.
func (w Window) DragOnPress(started func()) {
	if !dragReady() {
		return
	}
	widgetAddEvents(uintptr(w), buttonPressMask)
	ConnectEventScoped(uintptr(w), "button-press-event", func(event uintptr) bool {
		if eventButton(event, &gbutton) == 0 || gbutton != primaryButton {
			return false
		}
		if eventRoot(event, &groot[0], &groot[1]) == 0 {
			return false
		}
		// Before the handoff, not after, because the position has to be read
		// while it is still the position the drag starts from.
		//
		// It is NOT because the call can block - it cannot, on either X11 path.
		// gtk_window_begin_move_drag is a one-line wrapper over
		// gdk_window_begin_move_drag, and gdk/x11/gdkwindow-x11.c splits on
		// whether the manager advertises _NET_WM_MOVERESIZE: wmspec_moveresize
		// ungrabs the seat, sends one ClientMessage to the root window and
		// returns, while emulate_move_drag/create_moveresize_window grabs an
		// InputOnly helper window of GDK's own and returns, leaving the move to
		// _gdk_x11_moveresize_handle_event in the ordinary event dispatch.
		// Neither waits for the drop, so this handler returns and the main loop
		// keeps dispatching for the whole move. An earlier version of this
		// comment claimed the opposite; forecast_linux.go depends on the truth,
		// since a call that blocked would make its mid-move deferral pointless.
		if started != nil {
			started()
		}
		windowMoveDrag(uintptr(w), primaryButton,
			int32(groot[0]), int32(groot[1]), eventTime(event))
		return false
	})
}

// dragReady reports whether every optional symbol DragOnPress needs resolved.
// It exists so the set is written down once: the guard used to be inline with the
// count repeated in prose above it, and the prose fell a symbol behind the code
// the first time one was added - which quietly undercut the whole argument for
// binding these optionally. Anyone adding a symbol to the drag path adds it here
// and nowhere else.
func dragReady() bool {
	return widgetAddEvents != nil && windowMoveDrag != nil &&
		eventButton != nil && eventRoot != nil && eventTime != nil
}

// Position reports where the window actually is, which is what a panel the user
// has dragged must be asked before its place can be remembered: the coordinates
// handed to Move are a request, and the window manager is free to honour them
// only approximately.
//
// X11 only, with the same caveat as Move: on Wayland a client cannot know its own
// toplevel's global position, so GTK answers 0,0 - not as an error but as the only
// thing it can say. It answers 0,0 for a missing gtk_window_get_position too, and
// neither case is distinguishable from a window genuinely at the top-left corner.
//
// It answers, and Move accepts, the position of the window FRAME when the window
// manager draws one, and of the window itself when it does not. That is what GTK
// documents - the pair round-trips, so what Move is given is what Position gives
// back - and it is the right behaviour for remembering a place. What it is not is
// comparable between a decorated window and an undecorated one: measured under
// Marco, the same forecast panel sitting in the same visible spot reads 500,550
// undecorated and 499,528 framed, the difference being the frame extents
// 1,1,22,1. A position saved in one appearance and reused in the other is
// therefore off by the title bar, once, until the next drag corrects it. Storing
// which look wrote it would fix that and is not worth a config field for a
// one-time 22px nudge.
//
// The signature stays (int, int) rather than growing an ok result because that
// 0,0 costs nothing: the caller reads this once when a drag begins and once when
// the panel closes, and persists a position only when the two differ. On Wayland
// both reads are 0,0, so a Wayland session never writes ForecastX or ForecastY at
// all. That is the wanted outcome and not a gap to plug - a Wayland client has no
// global coordinates to remember, and a remembered position could not be honoured
// there either, since Move is equally ignored.
func (w Window) Position() (int, int) {
	if windowGetPos == nil {
		return 0, 0
	}
	windowGetPos(uintptr(w), &gpos[0], &gpos[1])
	return int(gpos[0]), int(gpos[1])
}

// Show maps the window itself, without touching its children.
func (w Window) Show() { widgetShow(uintptr(w)) }

// NewButton creates a push button that runs fn when clicked.
// The handler is scoped to the button: a push button never outlives the window it
// was packed into, and the settings window alone builds five of them every time it
// opens.
func NewButton(label string, fn func()) uintptr {
	b := buttonNew(label)
	ConnectScoped(b, "clicked", fn)
	return b
}

// pointerDevice resolves the default seat's pointer, or 0 when any link in the
// chain is missing. Written once because both pointer questions this package
// answers - where it is and which buttons it holds - have to walk the same
// display, seat, device chain first.
func pointerDevice() uintptr {
	display := displayDefault()
	if display == 0 {
		return 0
	}
	seat := displaySeat(display)
	if seat == 0 {
		return 0
	}
	return seatPointer(seat)
}

// PointerPosition reports where the pointer is in root coordinates. Must be
// called on the GTK thread - this reaches into GDK, which is not thread-safe.
// Returns ok=false on Wayland, where a client cannot see global coordinates.
func PointerPosition() (x, y int, ok bool) {
	pointer := pointerDevice()
	if pointer == 0 {
		return 0, 0, false
	}
	var screen uintptr
	var px, py int32
	devicePosition(pointer, &screen, &px, &py)
	return int(px), int(py), true
}

// PrimaryButtonHeld reports whether the pointer's primary button is physically
// down at this instant, and whether the question could be answered at all.
//
// It exists because GTK has no signal for the end of a window manager move. The
// panel hands a press to the manager with gtk_window_begin_move_drag and is then
// told nothing further: the button release goes to whoever holds the pointer
// grab for the move, which is the window manager on the EWMH path and GDK's own
// hidden helper window on the emulated one, and never the panel's toplevel. The
// button state is the one fact about the move that can still be read, and it can
// be read at any moment rather than waited for.
//
// ok=false MUST be read as "assume it is not held". A caller that suppresses
// behaviour while the button is down has to fall back to not suppressing it,
// because the alternative - assuming a drag is in progress because GDK would not
// say - is a window that can never be dismissed.
//
// The mask is asked of the ROOT window rather than of the panel: the root always
// exists, needs no realise, and is the same rectangle the pointer coordinates
// are already reported in. gmask is cleared first because a failed query leaves
// it untouched, and zero is the answer that fails towards "not held".
//
// It works during another client's pointer grab, which is the only reason it can
// serve this purpose: GDK asks the X server (XIQueryPointer, or XQueryPointer on
// core input), and the server reports the real button state to any client
// regardless of who holds a grab.
//
// Must be called on the GTK thread.
func PrimaryButtonHeld() (held, ok bool) {
	if screenRootWindow == nil || windowDevicePos == nil {
		return false, false
	}
	screen := screenDefault()
	if screen == 0 {
		return false, false
	}
	root := screenRootWindow(screen)
	if root == 0 {
		return false, false
	}
	pointer := pointerDevice()
	if pointer == 0 {
		return false, false
	}
	gmask = 0
	windowDevicePos(root, pointer, &gdevXY[0], &gdevXY[1], &gmask)
	return gmask&button1Mask != 0, true
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

// GeometryAt returns the full extent of the monitor containing the given point,
// panels and docks included. It answers a different question from WorkAreaAt, and
// a remembered panel position needs both: is this point still on a monitor at all,
// and where on that monitor may a window be placed.
//
// Testing containment against the work area instead is what made the two backends
// disagree. Win32 asks MonitorFromRect - which is geometry, taskbar included -
// whether the remembered point exists, then clamps it into the work area. GTK
// tested the point against the work area itself, so a panel the user dropped with
// its top-left corner over the desktop panel was clamped back into view on Windows
// and silently forgotten on Linux. Contain against geometry, clamp into the work
// area.
//
// The containment test belongs to the caller, not here: gdk_display_get_monitor_at_point
// answers with a NEARBY monitor for a point that is on none, so a monitor handle
// coming back is not evidence the point was on it. The rectangle is, once the
// caller checks the point against it.
//
// Must be called on the GTK thread, like everything else reaching into GDK.
func GeometryAt(x, y int) (Rect, bool) {
	display := displayDefault()
	if display == 0 {
		return Rect{}, false
	}
	monitor := monitorAtPoint(display, int32(x), int32(y))
	if monitor == 0 {
		return Rect{}, false
	}
	var r [4]int32
	monitorGeometry(monitor, &r[0])
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

// SetHExpand lets a widget claim horizontal slack in its container. An
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

// NewScrolledPage wraps a whole window page so that it scrolls only when it has
// to. propagate-natural-height makes the scroller ask for exactly the height its
// child wants, so a page that fits looks and measures as though there were no
// scroller at all; max-content-height is the ceiling past which it starts
// scrolling instead of growing.
//
// It exists because a non-resizable window around an unscrolled page has its
// content's natural height as its ONLY height, and nothing can shrink it. The
// settings window reached 758 pixels of content on a 1366x768 screen, which put
// its Save, Cancel and Delete buttons below the bottom edge with no way to reach
// them - so the caller keeps those buttons OUTSIDE this scroller. Both properties
// are GTK 3.22, which this binding already requires elsewhere for
// gdk_monitor_get_workarea.
func NewScrolledPage(child uintptr, maxHeight int) uintptr {
	sw := scrolledNew(0, 0)
	scrolledPolicy(sw, policyNever, policyAutomatic)
	scrolledNatural(sw, 1)
	if maxHeight > 0 {
		scrolledMaxH(sw, int32(maxHeight))
	}
	if child != 0 {
		containerAdd(sw, child)
	}
	return sw
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

// Check is a GtkCheckButton: one box that is independently on or off, as opposed
// to a RadioGroup where setting one member clears another. All methods must be
// called on the GTK thread.
type Check uintptr

// NewCheck creates a labelled check box in the given state.
//
// A GtkCheckButton is a GtkToggleButton, so its state is read and written with
// the very gtk_toggle_button_* accessors NewRadioGroup already uses - there is no
// check-button specific API to bind for it.
func NewCheck(label string, active bool) Check {
	if checkNewLabel == nil {
		return 0
	}
	c := checkNewLabel(label)
	toggleSetActive(c, b2i(active))
	return Check(c)
}

// Active reports whether the box is ticked. A Check that could not be created
// reads as unticked, so a form built without one saves the option off rather than
// dereferencing a widget that is not there.
func (c Check) Active() bool { return c != 0 && toggleGetActive(uintptr(c)) != 0 }

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
	ConnectScoped(uintptr(s), "value-changed", func() { fn(s.Value()) })
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
