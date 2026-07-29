// The whole surface Nimbus needs from Qt, and nothing else.
//
// Qt is C++ and exports nothing a C caller can use: libQt6Widgets exports 7780
// symbols and every one of them is a mangled C++ method. So the Go side does not
// bind Qt at all - it binds THIS, a handful of plain C functions, and every
// QWidget, QString and event loop stays on the C++ side of the line where it
// belongs.
//
// That is why this file is small and must stay small. Each function takes plain
// data - ints and C strings - and returns plain data. Nothing here hands a C++
// object, a pointer to one, or anything whose layout depends on the Qt version
// across the boundary, because the loader on the other side has no way to know
// what any of that means.
//
// NO STRUCTS CROSS EITHER, which is why the two windows that need a lot of data
// are BUILT rather than described: a call per row, a call per field, then one
// call to show it. A struct would have to agree on padding and field order
// between a C++ compiler and Go's reflection-based FFI, with nothing checking
// that it still does; a sequence of scalar calls cannot disagree about anything.
// The cost is a few more symbols and it buys the one property that matters here,
// which is that a mismatch is impossible rather than merely unlikely.
#ifndef NIMBUS_QT_SHIM_H
#define NIMBUS_QT_SHIM_H

#ifdef __cplusplus
extern "C" {
#endif

// What a window reports back through its event callback. The callback carries
// two integers whose meaning depends on the code, spelled out per code below;
// anything that has to carry TEXT goes through the separate field callback,
// because a C string does not fit in a long long.
#define NIMBUS_QT_EV_CLOSED 1 // the forecast panel is gone. a=1 if focus loss did it
#define NIMBUS_QT_EV_MOVED 2  // the panel's final position. a,b = x,y
#define NIMBUS_QT_EV_ACTION 3 // the settings form is done. a = NIMBUS_QT_ACTION_*
#define NIMBUS_QT_EV_SEARCH 4 // a text field's own button was pressed. a = its key
#define NIMBUS_QT_EV_PICK 5   // a list row was chosen. a = the list's key, b = the row
#define NIMBUS_QT_EV_SLIDE 6  // a slider moved. a = its key, b = the new value

#define NIMBUS_QT_ACTION_CANCEL 0
#define NIMBUS_QT_ACTION_SAVE 1
#define NIMBUS_QT_ACTION_RESET 2

// Column alignment, passed with each forecast header and applied to the whole
// column. Named here rather than assumed on either side.
#define NIMBUS_QT_ALIGN_START 0
#define NIMBUS_QT_ALIGN_CENTER 1
#define NIMBUS_QT_ALIGN_END 2

// nimbus_qt_init creates the QApplication. Returns 1 on success, 0 if Qt could
// not open a display - which is a normal answer, not a fault: the caller then
// falls back to another backend exactly as it does when GTK is missing.
//
// Must be called before anything else here, and from the thread that will later
// call nimbus_qt_run. Qt has the same thread affinity GTK does.
int nimbus_qt_init(void);

// nimbus_qt_run enters the Qt event loop and blocks until nimbus_qt_quit. The
// caller owns the thread for the whole life of the process, which is the same
// contract gtk.Main has.
void nimbus_qt_run(void);

// nimbus_qt_quit leaves the loop. Safe to call from any thread: it posts to the
// application rather than touching widgets.
void nimbus_qt_quit(void);

// nimbus_qt_invoke runs fn(id) on the thread that owns the Qt loop, and returns
// at once without waiting for it.
//
// It is the counterpart of gtk.Invoke and it exists for the same reason: Qt, like
// GTK, allows widget calls only from the thread that owns the loop, and the tray
// dispatches its menu on a different goroutine entirely. Everything below that
// touches a widget is called through this.
//
// fn is one fixed trampoline on the Go side dispatching by id, not a callback per
// call - purego's callback budget is fixed and never reclaimed, which is why the
// GTK binding has exactly three trampolines and this has three.
void nimbus_qt_invoke(void (*fn)(unsigned long long), unsigned long long id);

// nimbus_qt_about shows the About window. title and body are UTF-8; the shim
// copies them, so the caller may free them on return.
void nimbus_qt_about(const char *title, const char *body, const char *version);

// nimbus_qt_error shows an error dialog. Non-modal and one at a time, matching
// what the GTK and Win32 backends do: errors arrive in bursts when a weather
// service is down, and a stack of identical dialogs is worse than the first.
void nimbus_qt_error(const char *title, const char *message);

// nimbus_qt_font registers the weather symbol face from memory, once. data has to
// stay valid only for the duration of the call - Qt copies it.
//
// From memory rather than from a path on purpose: the typeface is embedded in the
// Go binary, so there is no file to point at, and writing one out would undo the
// property that the program is a single file.
void nimbus_qt_font(const void *data, int len);

// nimbus_qt_icon adds one size of the application icon, from an encoded image in
// memory. Call it once per size, smallest first; each call adds to the icon the
// windows already carry rather than replacing it, so a window manager can pick
// the size it wants for the title bar and another for the switcher.
//
// It applies to every window this process opens, including the ones already on
// screen, which is why it is called once at startup and never again.
void nimbus_qt_icon(const void *data, int len);

// ---- the forecast panel ----
//
// Built in four steps: begin, then the five headers, then a call per day, then
// show. State between the calls belongs to the shim and is discarded by the next
// begin, so an abandoned half-built panel cannot leak into the next one. All four
// must be called on the Qt thread, which means from inside a nimbus_qt_invoke.

// nimbus_qt_forecast_begin starts a new panel. The title is all it takes, and
// that is the point: the panel is an ordinary application window - the manager's
// frame, the desktop theme's colours - that merely stays above the others and off
// the taskbar, so it has no look for the caller to choose between.
void nimbus_qt_forecast_begin(const char *title);

// nimbus_qt_forecast_header adds one column caption. align is NIMBUS_QT_ALIGN_*
// and governs the whole column, caption and cells alike, so a column reads as one
// thing.
void nimbus_qt_forecast_header(const char *caption, int align);

// nimbus_qt_forecast_row adds one day. symbol is the weather glyph as UTF-8 - a
// single private-use codepoint from the face nimbus_qt_font registered - or an
// empty string for a code with no glyph, which leaves the cell empty rather than
// substituting a symbol that means something else.
void nimbus_qt_forecast_row(const char *day, const char *symbol, const char *temp,
                            const char *wind, const char *precip);

// nimbus_qt_forecast_show puts the panel on screen. have_at asks for the
// remembered position in x,y; without it the panel is anchored at the work-area
// corner nearest the pointer.
//
// pinned is asked at the moment of each dismissal rather than read once, so that
// unticking the box in settings frees the panel already on screen. It has to
// answer without blocking - it is called on the Qt thread.
//
// event reports what happened to the panel; see NIMBUS_QT_EV_* above. id is
// echoed back to both callbacks untouched.
void nimbus_qt_forecast_show(unsigned long long id, int have_at, int x, int y,
                             int (*pinned)(unsigned long long),
                             void (*event)(unsigned long long, long long, long long, long long));

// nimbus_qt_forecast_close dismisses the panel if one is up, reporting its
// position exactly as any other exit does. Returns 1 if there was one, which is
// what makes the tray icon a toggle.
int nimbus_qt_forecast_close(void);

// ---- the settings form ----
//
// Built the same way, field by field, and for the same reason. The shim knows
// nothing about what any field MEANS: every one carries an integer key chosen by
// the caller, which comes back with its value when the user saves.

// nimbus_qt_form_begin starts a new form and discards any half-built one.
void nimbus_qt_form_begin(const char *title);

// nimbus_qt_form_group starts a titled group; fields added after it go inside.
// An empty caption ends the current group, so what follows returns to the page.
void nimbus_qt_form_group(const char *caption);

// nimbus_qt_form_text adds a single-line text field. action, when it is not
// empty, adds a button beside it that reports NIMBUS_QT_EV_SEARCH with this
// field's key - which is how the city field gets its lookup without the shim
// knowing what a city is.
void nimbus_qt_form_text(int key, const char *label, const char *value, const char *action);

// nimbus_qt_form_list adds a results list, min_height pixels tall, whose rows are
// filled later by nimbus_qt_form_list_add. Choosing a row reports
// NIMBUS_QT_EV_PICK.
void nimbus_qt_form_list(int key, int min_height);

// nimbus_qt_form_choice adds a row of radio buttons, nimbus_qt_form_combo a
// dropdown. Both are followed by one nimbus_qt_form_option per alternative, and
// selected marks the current one.
void nimbus_qt_form_choice(int key, const char *label);
void nimbus_qt_form_combo(int key, const char *label);
void nimbus_qt_form_option(const char *label, int selected);

// nimbus_qt_form_check adds a checkbox, whose own label is its title.
void nimbus_qt_form_check(int key, const char *label, int checked);

// nimbus_qt_form_slider adds a slider with a percentage readout beside it. Every
// movement reports NIMBUS_QT_EV_SLIDE so the caller can preview live; the caller
// is expected to coalesce, because a drag across the range emits one per step.
void nimbus_qt_form_slider(int key, const char *label, int min, int max, int value);

// nimbus_qt_form_buttons adds the row that ends the form. It sits outside the
// scrolling part, so it stays reachable however tall the settings get.
void nimbus_qt_form_buttons(const char *save, const char *cancel, const char *reset);

// nimbus_qt_form_show puts the form on screen and returns at once. Exactly one
// NIMBUS_QT_EV_ACTION is reported however the window ends - including through the
// window manager's own close button - so a caller blocked on the answer can never
// be stranded.
//
// On save, and only on save, every field is reported through field(id, key,
// value) BEFORE the action arrives. Values are UTF-8 text in every case: a
// checkbox reports "0" or "1", a choice reports the index of the selected option,
// and parsing them back is the caller's business, since the caller is the only
// side that knows what the field was for.
void nimbus_qt_form_show(unsigned long long id,
                         void (*event)(unsigned long long, long long, long long, long long),
                         void (*field)(unsigned long long, long long, const char *));

// nimbus_qt_form_set writes a field's value from outside - which is what a picked
// search result does to the city and the two coordinates.
void nimbus_qt_form_set(int key, const char *value);

// nimbus_qt_form_enable greys a field out, which is how a search in flight tells
// the user that it started.
void nimbus_qt_form_enable(int key, int on);

// nimbus_qt_form_report sends one field's current value through the field
// callback right now, without waiting for a save. It is how the caller reads a
// field it needs before the form is over - the city to search for, say - and it
// reuses the save path's callback rather than returning a string, because a
// returned pointer would raise the question of who owns it and for how long.
void nimbus_qt_form_report(int key);

// nimbus_qt_form_list_clear and nimbus_qt_form_list_add fill a list after the
// fact, when the network has answered.
void nimbus_qt_form_list_clear(int key);
void nimbus_qt_form_list_add(int key, const char *label);

#ifdef __cplusplus
}
#endif

#endif
