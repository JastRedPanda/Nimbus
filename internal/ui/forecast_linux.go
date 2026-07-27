//go:build linux

package ui

import (
	"image/color"
	"log"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/fonts"
	"github.com/JastRedPanda/Nimbus/internal/gtk"
	"github.com/JastRedPanda/Nimbus/internal/gui"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

// The 7-day forecast panel, GTK edition.
//
// The CONTENT is a plain five-column table: one header row, a rule, then seven
// data rows separated by hairlines. No summary header and no cards - that layout
// was tried and reverted. The CHROME around it is the panel work and stays: off
// the taskbar and the pager, above other windows, sticky across workspaces,
// draggable by its body, placed where the user last dragged it or else at the
// work-area corner nearest the click, and dismissed by Escape, by focus loss, or
// by the window's own close button - the first two only while the panel is not
// pinned, and neither of them while the window manager is moving the panel.
//
// TWO LOOKS, and what differs between them is chrome and nothing else. MODERN,
// the default, is the undecorated sheet: no frame, translucent wherever a
// compositor can actually composite it, rounded corners, coloured from this
// package's own palettes, and closed by an × button it draws itself because it
// has no title bar. SYSTEM is an ordinary application window: the window
// manager's frame and title bar, opaque, square, coloured by the desktop theme,
// and closed by the title bar's button - which is routed through the same
// dismissal the × button uses, so it reports the panel's position the way every
// other exit does. Everything else is identical in both on purpose: the hints
// listed above, the placement and the remembered position, the tray toggle, the
// pinning policy, the drag handling, and the table with every metric in it.
//
// Every metric is shared with the Win32 backend value for value - see the
// constants below and the stylesheet in style_linux.go, whose numbers
// forecast_windows.go carries as named constants of its own. The point of the
// two files is that the two platforms look like one product.

const (
	forecastWidth = 620
	// -1 lets the panel hug its content. A fixed height would stretch the table
	// rows over whatever height was guessed.
	forecastHeight = -1

	// Weather-symbol size in layout points. Unlike the colour artwork this
	// replaced, the glyph is rasterised at whatever size is asked for, so a
	// HiDPI screen simply gets scale times as many pixels rather than the
	// nearest available asset.
	symbolPt = 20

	// Grid metrics. These live in Go rather than in the stylesheet because GTK
	// grid spacing is a widget property, not a style property - CSS cannot set
	// it. Counterparts in forecast_windows.go: rowGapY, colGapX.
	rowGapY = 6
	colGapX = 18

	// Page VBox spacing: the close-button row to the table. Counterpart:
	// pageGapY.
	pageGapY = 2

	forecastWindowID = "nimbus-forecast"

	// Clearance from the work-area edges the panel hugs.
	panelMargin = 12
)

// How the settler for a suppressed dismissal is paced - see settleDrag in
// buildForecast.
//
// dragPoll is how often it re-asks GDK whether the primary button is still down.
// Fast enough that the panel closes as the user lets go rather than visibly
// after, slow enough that even a long drag costs ten pointer queries a second
// and nothing else.
//
// dragSettleMax is how long it keeps asking before acting anyway. It is
// deliberately far longer than any drag a person performs, because it is not a
// time limit on dragging: it is the backstop for a pointer that reports a button
// as held forever, which is the one way the button state can lie.
const (
	dragPoll = 100 * time.Millisecond
	// placeSettle is how long the system look waits before taking the window's
	// position as the baseline a later title-bar drag is measured against. Long
	// enough for a window manager to finish placing a window it has just been given,
	// short enough that a user cannot drag the panel before it elapses.
	placeSettle   = 400 * time.Millisecond
	dragSettleMax = 20 * time.Second
)

// closeGlyph is the panel's own close affordance in the MODERN look, which has no
// title bar and therefore no system close button, so the panel supplies one -
// exactly as the Win32 panel does. The system look does not draw it: its title bar
// already carries a close button, and two of them in one window is one too many.
const closeGlyph = "×" // MULTIPLICATION SIGN

// colAlign is the horizontal alignment of each table column, applied to both the
// header caption and the cells under it so a column reads as one thing. Day
// reads as a label and sits left; the symbol is centred in its column; the three
// numeric columns are right aligned so their digits line up, which is the whole
// reason a table beats a row of cards.
var colAlign = [...]int{
	gtk.AlignStart,  // Day
	gtk.AlignCenter, // Condition
	gtk.AlignEnd,    // Temp
	gtk.AlignEnd,    // Wind
	gtk.AlignEnd,    // Precip
}

// All three are read and written only on the GTK thread. forecastClosedAt is
// when the panel last went away on its own, which the tray toggle needs to tell
// a closing click from an opening one.
//
// forecastDismiss is the open panel's own closer rather than a bare Destroy,
// because the panel's final position has to be read off the window before it is
// destroyed and only the closure built with the panel knows where to report it.
var (
	forecastWindow   gtk.Window
	forecastClosedAt time.Time
	forecastDismiss  func()
)

// closeOpenPanel makes the tray icon a toggle: if the panel is up, the click
// that would have opened it closes it instead. It reports whether it consumed
// the click, and must run on the GTK thread.
func closeOpenPanel() bool {
	if forecastWindow != 0 {
		if forecastDismiss != nil {
			forecastDismiss()
		} else {
			forecastWindow.Destroy()
		}
		return true
	}
	if !forecastClosedAt.IsZero() && time.Since(forecastClosedAt) < toggleGrace {
		// Worth a line: to the user this click did nothing at all.
		log.Print("forecast: click within the toggle grace period, treated as the closing click")
		return true
	}
	return false
}

// showForecast opens the 7-day forecast panel. It returns immediately: the
// forecast is fetched on its own goroutine because the caller is the tray's
// single menu-dispatch loop, and a blocking 10s HTTP call there would freeze
// Settings, About and Quit along with it.
func showForecast(req gui.Forecast) {
	// The pointer is sampled now, while the user's click is still fresh, rather
	// than when the window is finally built: the fetch in between can take up
	// to ten seconds, by which time the pointer may be on another monitor
	// entirely. GDK is not thread-safe, so even this read goes through the GTK
	// loop - it costs a fraction of a millisecond.
	type start struct {
		at       gtk.Rect
		consumed bool
	}
	begin := make(chan start, 1)
	if err := gtk.Invoke(func() {
		if closeOpenPanel() {
			begin <- start{consumed: true}
			return
		}
		begin <- start{at: pointerAnchor()}
	}); err != nil {
		begin <- start{}
	}

	go func() {
		s := <-begin
		if s.consumed {
			return
		}
		data, err := weather.FetchDaily(req.Lat, req.Lon)
		schedErr := gtk.Invoke(func() {
			l := i18n.ParseLang(req.Lang)
			if err != nil {
				log.Printf("forecast: fetch failed: %v", err)
			} else if len(data) == 0 {
				log.Print("forecast: fetch returned no days")
			}
			if err != nil || len(data) == 0 {
				ensureAppIcon()
				gtk.ShowError(appName, forecastFailed(l), "", closeLabel(l))
				return
			}
			buildForecast(data, req, l, s.at)
		})
		if schedErr != nil {
			// Nothing will ever draw this. Say so rather than leaving the user
			// clicking a menu item that silently does nothing.
			log.Printf("forecast: cannot reach the GTK loop: %v", schedErr)
		}
	}()
}

// pointerAnchor captures where the panel should be anchored. A zero rect means
// the position could not be determined - on Wayland, for instance, where a
// client cannot see global pointer coordinates - and the panel is then left
// wherever the window manager puts it.
func pointerAnchor() gtk.Rect {
	x, y, ok := gtk.PointerPosition()
	if !ok {
		return gtk.Rect{}
	}
	area, ok := gtk.WorkAreaAt(x, y)
	if !ok {
		return gtk.Rect{}
	}
	return gtk.Rect{X: x, Y: y, W: area.W, H: area.H}
}

func buildForecast(data []weather.DailyForecast, req gui.Forecast, l i18n.Lang, at gtk.Rect) {
	if forecastWindow != 0 {
		forecastWindow.Present()
		return
	}

	ensureAppIcon()
	gtk.LoadCSS(forecastCSS)

	// The mid-move state, declared before the window because the destroy handler
	// below is one of the things that writes it. All of it is read and written
	// only on the GTK thread, like the package globals above.
	//
	// closed makes dismiss idempotent, which the deferral further down requires:
	// a suppressed dismissal is honoured by a callback that runs arbitrarily
	// later, and by then the × button, the tray toggle or the window manager may
	// already have destroyed this window. A second gtk_widget_destroy on it
	// would read a GdkWindow that is gone. It is set from the destroy handler as
	// well as from dismiss, so an exit that never went through dismiss - the
	// window manager's, or shutdown's - is recorded too.
	//
	// dragging says a press on the panel body was handed to the window manager
	// and the primary button has not been seen up since. It is only half of "a
	// move is in progress"; inMove is the other half.
	//
	// deferred is a focus loss that arrived during a move and has not been
	// settled. settling says a settler is already armed, so a burst of focus
	// events arms exactly one. settleBy is when that settler stops waiting for
	// the button and acts regardless.
	//
	// focused is whether the panel holds focus right now. settleDrag re-reads it
	// instead of trusting the decision it deferred, which is what the
	// GetForegroundWindow call in forecast_windows.go's settleDrag is for.
	//
	// byFocus says this panel is closing because it lost focus, and it is what
	// arms the tray toggle's grace period. Only that one cause may arm it: the
	// grace exists solely because a click on the tray icon can take the focus
	// away and close the panel before the click itself is delivered, so a click
	// arriving just after such a close is the closing click rather than a new
	// opening one. Arming it after a deliberate close - the x button, the toggle,
	// Escape - is what ate twelve consecutive clicks from a user who was tapping
	// the icon: every tap inside the window was answered by silence.
	var (
		closed   bool
		byFocus  bool
		dragging bool
		deferred bool
		settling bool
		// armSettle arms the poller below; it is declared here because the drag
		// handler is built before the poller it needs to start.
		armSettle func()
		settleBy  time.Time
		focused   bool
	)

	// The look is decided here, first, because everything it changes - the frame,
	// the visual, the palette classes - has to be settled before the window is
	// realised.
	//
	// Tested for "system" with everything else falling through to modern, never by
	// comparing against "modern": the value arrives from a configuration file the
	// user can edit by hand, and both an unrecognised string and the empty string a
	// caller that predates the option sends have to mean the look that existed
	// before the option did. gui.Forecast.Appearance asks every backend for exactly
	// this shape.
	system := req.Appearance == "system"

	// The one thing the two constructors disagree about is whether the window
	// manager frames the window; every hint either look needs is inside both.
	var win gtk.Window
	if system {
		win = gtk.NewFramedPanel(l.ForecastTitle(), forecastWidth, forecastHeight)
	} else {
		win = gtk.NewPanel(l.ForecastTitle(), forecastWidth, forecastHeight)
	}
	win.OnDestroy(func() {
		closed = true
		forecastWindow = 0
		forecastDismiss = nil
		if byFocus {
			forecastClosedAt = time.Now()
		}
	})
	gtk.SetName(uintptr(win), forecastWindowID)

	// fg is the ink the rasterised weather symbols are tinted with. It is decided
	// here, with the colours, because matching them is its whole job.
	var fg color.NRGBA
	if system {
		// No class at all: not .dark or .light, not .solid or .translucent. Every
		// colour and every corner radius in the stylesheet hangs off one of those
		// four, so with the name alone none of those rules match while the resets,
		// the page padding and the two font sizes still do - and the desktop theme
		// paints the window, its labels and its separators. That is what "colours
		// from the desktop theme" is implemented as; style_linux.go says which half
		// of the sheet each look uses and why the halves cannot be merged.
		//
		// SetTranslucent is deliberately not called. An RGBA visual exists to
		// composite the Modern sheet's alpha against the desktop, and this window is
		// a framed, opaque application window with no alpha to composite.
		//
		// The tint is the theme's own ink rather than a stylesheet literal, which is
		// the inverse of what the Modern look does - see paletteForeground, whose
		// reasoning inverts precisely here: with no palette forced on the window
		// there is no palette left for the desktop theme to disagree with.
		//
		// One class IS added, and it names no colour: .system exists so the header
		// rule can keep the weight it has in Modern. With no class at all both the
		// rule and the row hairlines fell through to the theme's single separator
		// colour, which collapsed two deliberate weights into one - measured at
		// 1.08:1 against the background under Adwaita:dark, which is invisible. The
		// sheet gives .system nothing but a min-height, so the colour is still the
		// theme's and only the thickness is ours. The Win32 system look keeps the
		// same hierarchy by giving the rule and the hairlines two alphas of
		// COLOR_3DSHADOW.
		gtk.AddClass(uintptr(win), "system")
		fg = win.Foreground()
	} else {
		// The visual has to be chosen before the window is realised, and the
		// stylesheet must follow what was actually granted rather than what was
		// asked for: with no compositor the alpha would composite against black and
		// every rounded corner would come out as a black notch.
		fill := "solid"
		if win.SetTranslucent() {
			fill = "translucent"
		}
		palette := paletteFor(win, req.Theme)
		gtk.AddClass(uintptr(win), palette, fill)
		fg = paletteForeground(palette)
	}

	// What licenses reporting a position, and nothing less: a press was handed to
	// the window manager's move loop at least once during THIS showing, AND the
	// window is no longer where it was at that first handoff. handedOff and origin
	// are the two halves of that test.
	//
	// Neither half is enough alone. Reporting every close wrote down a corner
	// nobody chose on the first open-and-close of a user's very first forecast; the
	// tray reads a stored position as "put it back there", so pointer anchoring
	// never ran again for that user - permanently, and by default, since
	// ForecastPinned defaults to on. But the handoff on its own is just as bad,
	// because a bare click on the panel body IS a handoff: the press goes to the
	// window manager whether or not the pointer then moves. So is a drag the user
	// cancels with Escape, which the window manager undoes by putting the window
	// back - and the position comparison is what makes that report nothing.
	//
	// Both positions are read the SAME way, through gtk.Window.Position, and never
	// against the placement placePanel asked for. Reading both through the same
	// call is what makes this correct on Wayland, where gtk_window_get_position
	// always answers 0,0: the two reads are then equal, so nothing is persisted,
	// which is the outcome we want there - a Wayland client cannot know its own
	// position, and 0,0 is not a position the user chose. forecast_windows.go
	// applies the identical rule through GetWindowRect.
	//
	// GTK thread only, like the package globals above: written from the
	// button-press handler, read from dismiss.
	handedOff := false
	origin := gui.Point{}

	// The system look needs a second way to establish that origin, because the
	// obvious gesture in it - dragging the window by its title bar - never reaches
	// the toolkit at all. The window manager reparents a decorated window and
	// handles the caption itself, so no button-press-event is emitted, handedOff
	// stays false, and a panel the user dragged across the screen by its title bar
	// reported nothing. Measured under Marco: a title-bar drag from 13,43 to
	// 163,153 wrote no position, while a body drag of the same window did.
	//
	// So in that look the window's settled placement is taken as the origin once,
	// shortly after it is mapped, and everything after that is a move. The delay is
	// what makes it a settled position rather than a transient one: the manager may
	// adjust a freshly mapped window, and a baseline read mid-adjustment would make
	// the adjustment itself look like something the user did. Modern needs none of
	// this - it has no frame for a manager to move it by, so the press is always
	// ours to see.
	//
	// The Win32 backend has the same problem and solves it in the same spirit, by
	// capturing the origin in WM_ENTERSIZEMOVE, which is the caption drag's own
	// starting message.
	if system {
		gtk.After(placeSettle, func() {
			if closed || handedOff {
				return
			}
			x, y := win.Position()
			handedOff, origin = true, gui.Point{X: x, Y: y}
		})
	}

	// A press on the panel body starts a window-manager move. The close button
	// keeps working because it has its own GdkWindow and consumes its own press,
	// so button-press-event on the toplevel never fires for it - see
	// gtk.Window.DragOnPress, which is where that is spelled out.
	//
	// Only the FIRST handoff records an origin. A later press happens wherever the
	// user has already dragged the panel to, so keeping the newest origin would
	// make a drag followed by one idle click report nothing at all. The callback
	// runs just before gtk_window_begin_move_drag, so this reads the position the
	// drag starts from.
	// dragging is set on EVERY press, unlike origin: it says a move is starting
	// now, not that one ever started, and the second and later presses of a
	// showing are moves just as much as the first.
	win.DragOnPress(func() {
		dragging = true
		// Watch for the button coming up. Nothing else can tell the latch the
		// move is over - see inMove.
		armSettle()
		if handedOff {
			return
		}
		x, y := win.Position()
		handedOff, origin = true, gui.Point{X: x, Y: y}
	})

	// dismiss is the panel's single exit: the × button, Escape, focus loss and the
	// tray toggle all close it through here, which is the only reason the position
	// gets reported at all.
	//
	// The order inside it is the whole point. The position has to be read while
	// the window still exists: gtk_widget_destroy takes the GdkWindow with it, and
	// gtk.Window.Position on what is left answers with garbage. That is why the
	// read cannot live in the destroy handler above, where forecastClosedAt is
	// set - by the time that runs there is nothing left to ask. A destroy that
	// arrives from outside this file, from the window manager or at shutdown,
	// therefore reports no move, which is correct: an unread position is better
	// than a wrong one.
	//
	// The coordinates reported are the ones read HERE, at close, not the origin
	// recorded at the handoff: the origin exists only to be compared against.
	//
	// The read stays here; only the delivery moves off the GTK thread. OnMove
	// writes the configuration file, and a disk write inside the GTK main loop
	// stalls every window this process owns - on the tray-toggle path it would
	// stall inside a gtk.Invoke as well, with the panel still visibly on screen
	// for as long as the disk takes. The Win32 backend hands OnMove to a
	// goroutine the same way, so the callback has one contract on both platforms:
	// arbitrary goroutine, coordinates already read, free to take locks and do
	// I/O.
	//
	// The guard on the front is not decoration. settleDrag can call this a moment
	// after the panel was closed by something else entirely, and a re-entrant
	// dismissal would read Position off a destroyed window - garbage - and then
	// report it as a position the user chose, or destroy the window twice.
	dismiss := func() {
		if closed {
			return
		}
		closed = true
		if req.OnMove != nil && handedOff {
			if x, y := win.Position(); x != origin.X || y != origin.Y {
				onMove := req.OnMove
				go onMove(x, y)
			}
		}
		win.Destroy()
	}

	// pinned is asked at the moment of each event rather than read once here.
	// The tray hands over a function precisely so that unticking the box in
	// settings frees the panel already on screen instead of only the next one.
	// A nil Pinned means not pinned - the behaviour before the option existed.
	pinned := func() bool { return req.Pinned != nil && req.Pinned() }

	// inMove reports whether the window manager is moving the panel right now.
	//
	// This is where this file stopped differing from the Win32 panel on purpose.
	// Both backends now promise the same thing - a move in progress suspends
	// dismissal - and they promise it for different reasons. On Win32 the move
	// loop runs ON the UI thread and the window would be destroyed from inside
	// itself. Here nothing is re-entrant: gtk_window_begin_move_drag sends the
	// manager one ClientMessage and returns, the main loop keeps dispatching, and
	// Marco ends its own grab on our DestroyNotify. What was left was purely the
	// user's experience - an unpinned panel vanishing out from under the pointer,
	// mid-drag, because something else took the focus - and the note that used to
	// stand here, calling that "the unpinned contract anyway", was the wrong call.
	// A window the user is holding is a window the user is using.
	//
	// HOW THE END OF THE MOVE IS DECIDED, which is the whole design problem
	// because GTK emits no signal for it: GDK is asked whether the primary button
	// is still physically down, at the moment each dismissal arrives. There is no
	// latched "the manager is moving me" flag to get stuck and no timer guessing
	// how long a drag lasts.
	//
	// Every alternative is worse. button-release-event never arrives here at all:
	// the release goes to whoever holds the pointer grab for the move, which is
	// the window manager on the EWMH path and GDK's own hidden helper window on
	// the emulated one, never this toplevel. focus-in as the end-of-move signal
	// only fires when the focus comes BACK, which is precisely the case where
	// nothing needed deferring. Watching configure-event stop arriving, or arming
	// a fixed timer, is a guess that either expires while the user is still
	// holding the panel or leaves it immune long after they let go.
	//
	// AN ABNORMALLY ENDED DRAG CANNOT LEAVE THE PANEL IMMUNE TO DISMISSAL, which
	// would be a worse bug than the one being fixed. Immunity is not state that
	// something has to remember to clear; it is recomputed from the pointer at
	// every dismissal and lasts exactly as long as the button is down. A manager
	// that dies mid-move, a drag the manager cancels, a window that never gets a
	// configure - none of them can hold a button down. If the question cannot be
	// answered at all, because the symbol is missing or GDK will not say, it
	// answers "not held" and nothing is ever suppressed, so that failure mode is
	// the old behaviour rather than a panel that cannot be closed. The one
	// remaining way for the pointer itself to lie - a physically stuck button - is
	// what settleDrag's deadline is for.
	//
	// Both halves are needed. The latch alone would suppress dismissals for the
	// rest of the panel's life, since nothing announces the end of the move. The
	// button alone would suppress them whenever the user happens to be holding a
	// button somewhere else entirely - drag-selecting text in the window that just
	// took the focus, say - deferring a close that has nothing to do with a move.
	//
	// For that second guarantee to be real the latch must be cleared when the
	// move ENDS, not lazily whenever the next dismissal happens to ask. Clearing
	// it here only was a bug with teeth: after one drag the latch stayed set for
	// the panel's whole life, so any button held anywhere on the desktop suppressed
	// a focus-loss close - and Escape, which is dropped rather than deferred, was
	// swallowed outright, leaving the panel with only its × button. The drag
	// handler therefore arms the poller below, which clears the latch as soon as
	// the button comes up whether or not anything is waiting on it.
	inMove := func() bool {
		if !dragging {
			return false
		}
		held, ok := gtk.PrimaryButtonHeld()
		if !ok || !held {
			// The move is over, or GDK cannot say. Either way the latch has done
			// its job and must not outlive it.
			dragging = false
			return false
		}
		return true
	}

	// settleDrag disposes of a focus loss that was suppressed during a move, and
	// is the only thing that ever does. It re-arms itself until the button comes
	// up, so it is also the only thing here that polls.
	//
	// THE CHOICE, stated once: a suppressed focus loss is HONOURED when the move
	// ends, unless the panel has the focus back by then or has been pinned in the
	// meantime. That is forecast_windows.go's settleDrag rule, reached the same
	// way - re-read the state now rather than trust the decision that was
	// deferred.
	//
	// Dropping it outright would be wrong for exactly the reason it is wrong
	// there: focus-out is delivered on the TRANSITION, so a panel that is already
	// unfocused is never told again unless it is focused first. An unpinned panel
	// would then sit above whatever the user switched to with only its × button
	// and the tray icon left to close it, which is the opposite of what "closes
	// when you look away" promised. Honouring it unconditionally would be equally
	// wrong: the ordinary end of a drag hands the focus straight back, and closing
	// then would make the panel vanish the instant the user let go of it.
	var settleDrag func()
	settleDrag = func() {
		if closed {
			// The × button, the tray toggle or the window manager got there
			// first. No window left to close and no position left to read.
			settling, deferred = false, false
			return
		}
		if inMove() {
			if time.Now().Before(settleBy) {
				gtk.After(dragPoll, settleDrag)
				return
			}
			// The only way to be here is a button GDK keeps reporting as down
			// long past any real drag. Acting anyway is the lesser evil: the
			// panel is otherwise immune to focus loss for as long as the lie
			// lasts. Destroying a window mid-move is safe - the manager ends its
			// grab on DestroyNotify - which is what makes this the safe direction
			// to fail in.
			log.Printf("forecast: the primary button has read as held for %s since a close was deferred; settling anyway", dragSettleMax)
			dragging = false
		}
		settling = false
		if !deferred {
			return
		}
		deferred = false
		if pinned() {
			return
		}
		if focused {
			log.Print("forecast: the focus came back before the move ended; staying open")
			return
		}
		log.Print("forecast: closing, the focus was taken during a window-manager move")
		byFocus = true
		dismiss()
	}

	// Escape always works; losing focus is the convenience path. Both are what
	// pinning switches off, and both are what a move in progress suspends, which
	// leaves the tray icon and the window's own close button - the × button in the
	// Modern look, the title bar's in the system one - as the ways out. So none of
	// those three may ever consult pinned or inMove. They destroy the panel mid-move
	// if that is what the user asked for, and that is safe.
	//
	// Focus loss is armed only after the panel has actually held focus once. An
	// undecorated window is not guaranteed to be given focus when it is mapped,
	// and without this the first focus-out - which can arrive before the user
	// has seen anything - closes the panel again immediately. The symptom is a
	// panel that flickers and vanishes, intermittently, depending on what held
	// focus at the moment it opened. The Win32 backend has always armed this.
	armed := false
	win.OnEscape(func() {
		if pinned() {
			return
		}
		if inMove() {
			// Dropped, not deferred, which is the opposite of what happens to a
			// focus loss and is deliberate. During a move Escape is the window
			// manager's own cancel-the-drag key: measured under Marco it never
			// reaches this handler at all, because the manager takes the key,
			// cancels the move and puts the window back. One that DOES arrive
			// therefore comes from a manager that does not bind it or from GDK's
			// emulated move path, and either way the keystroke was aimed at the
			// drag rather than at the panel. Nothing is lost by dropping it: unlike
			// focus-out, Escape is not delivered on a transition, so a user who did
			// mean "close" presses it again once the panel has stopped moving.
			log.Print("forecast: Escape during a window-manager move, ignored; it belongs to the drag")
			return
		}
		dismiss()
	})
	armSettle = func() {
		if settling {
			return
		}
		settling = true
		settleBy = time.Now().Add(dragSettleMax)
		gtk.After(dragPoll, settleDrag)
	}

	win.OnFocusIn(func() { armed, focused = true, true })
	win.OnFocusOut(func() {
		focused = false
		if !armed || pinned() {
			return
		}
		if inMove() {
			if !deferred {
				// Logged for the same reason the suppression exists at all: from
				// the outside, and from a log, a suppressed close is
				// indistinguishable from no event having happened. It is the only
				// evidence that this path ran. Guarded on deferred rather than
				// printed per event, so a focus storm during one drag costs one
				// line and cannot pad the log file.
				log.Print("forecast: the focus was taken during a window-manager move; deferring the close until the button is released")
			}
			deferred = true
			armSettle()
			return
		}
		// Any window taking focus lands here, not just one the user clicked -
		// a notification or a background window will close the panel too. The
		// line is here because an unexplained disappearance is otherwise
		// indistinguishable from a crash.
		log.Print("forecast: closing, focus lost")
		byFocus = true
		dismiss()
	})

	// In the system look the title bar is the way out of the window, so its close
	// button has to run the panel's own dismissal rather than GTK's default destroy:
	// the default reports nothing, and with no × button inside the window that would
	// mean a panel the user dragged somewhere almost never remembers where it was
	// dropped. Handed dismiss, exactly like the × button, so it closes
	// unconditionally - pinned or not, mid-move or not.
	//
	// Only in the system look. Modern has no frame, so nothing in it emits
	// delete-event to begin with; wiring it there anyway would still change what a
	// close arriving from outside the window - a session shutdown, a wmctrl -c -
	// does to the look the user is already running, and today it correctly reports
	// no position, because an unread position is better than a wrong one.
	if system {
		win.OnDeleteEvent(dismiss)
	}

	scale := win.ScaleFactor()

	page := gtk.NewVBox(pageGapY)
	gtk.AddClass(page, "page")
	win.Add(page)

	// The close row is the Modern look's stand-in for a title bar and belongs to it
	// alone. In the system look there is a real title bar with a real close button,
	// wired above to the same dismiss.
	if !system {
		gtk.PackStart(page, closeRow(dismiss), false, false, 0)
	}
	gtk.PackStart(page, forecastTable(data, req.Units, req.WindUnit, l, scale, fg), false, false, 0)

	placePanel(win, page, at, req.At)
	forecastWindow = win
	forecastDismiss = dismiss
}

// closeRow is the Modern look's title-bar substitute: nothing but the × button,
// pushed to the trailing edge of the sheet. It is not packed at all in the system
// look, which has the real thing.
//
// The button is packed expanding and filling with its own halign set, rather
// than packed non-expanding after a spacer label. A spacer would be one more
// label for the desktop theme to state a colour and a padding on, and there is
// no summary text left for it to hide behind.
//
// It is handed the panel's dismiss closure rather than the window, because it
// closes unconditionally - pinned or not, it is the only exit left inside the
// window - and because closing has to report the final position, which a bare
// Destroy would skip.
func closeRow(dismiss func()) uintptr {
	row := gtk.NewHBox(0)
	btn := gtk.NewButton(closeGlyph, dismiss)
	gtk.AddClass(btn, "close")
	gtk.SetHAlign(btn, gtk.AlignEnd)
	gtk.PackStart(row, btn, true, true, 0)
	return row
}

// forecastTable builds the table: captions, a rule, then one row per day with a
// hairline between neighbours.
//
// The grid is deliberately NOT column-homogeneous - forcing the columns equal
// makes every one of them as wide as the widest caption ("Температура") and puts
// a large floor under the panel width. Every cell is a gtk.NewCell, which does
// not wrap: a wrapping label reports a near-zero minimum width, and its column
// then collapses to nothing the moment the panel is narrower than its content.
func forecastTable(data []weather.DailyForecast, units, windUnit string, l i18n.Lang, scale int, fg color.NRGBA) uintptr {
	grid := gtk.NewGrid(rowGapY, colGapX)
	cols := len(colAlign)

	for i, caption := range l.ForecastHeaders() {
		if i >= cols {
			break
		}
		cell := gtk.NewCell(caption, colAlign[i])
		gtk.AddClass(cell, "thead")
		grid.Attach(cell, i, 0, 1, 1)
	}

	rule := gtk.NewHSeparator()
	gtk.AddClass(rule, "rule")
	grid.Attach(rule, 0, 1, cols, 1)

	row := 2
	for i, d := range data {
		if i > 0 {
			sep := gtk.NewHSeparator()
			grid.Attach(sep, 0, row, cols, 1)
			row++
		}

		// The ISO date exactly as Open-Meteo returned it. No weekday name and no
		// localised month: the column is a date, and a sortable one reads the
		// same in both languages.
		grid.Attach(dataCell(d.Date, 0), 0, row, 1, 1)

		// A missing glyph leaves the slot empty rather than substituting a
		// symbol that means something else.
		if sym := symbolWidget(d.WeatherCode, symbolPt, scale, fg); sym != 0 {
			gtk.SetHAlign(sym, gtk.AlignCenter)
			gtk.SetVAlign(sym, gtk.AlignCenter)
			grid.Attach(sym, 1, row, 1, 1)
		}

		grid.Attach(dataCell(tempRange(d, units, l), 2), 2, row, 1, 1)
		grid.Attach(dataCell(windSpeed(d, windUnit, l), 3), 3, row, 1, 1)
		grid.Attach(dataCell(precip(d, l), 4), 4, row, 1, 1)

		row++
	}
	return uintptr(grid)
}

func dataCell(text string, col int) uintptr {
	cell := gtk.NewCell(text, colAlign[col])
	gtk.AddClass(cell, "cell")
	return cell
}

// symbolWidget rasterises the Weather Icons glyph for a condition code into a
// GtkImage.
//
// The symbol is monochrome and tinted with the palette's foreground on purpose:
// it was picked over the colour artwork, and a glyph drawn in the same ink as
// the numbers beside it belongs to the table rather than sitting on top of it.
// The glyph is rasterised at scale times the layout size and the GtkImage is
// told the scale, so a HiDPI screen draws real pixels instead of an upscale.
func symbolWidget(code, pt, scale int, col color.NRGBA) uintptr {
	if scale < 1 {
		scale = 1
	}
	img := fonts.Glyph(fonts.RuneForCode(code), pt*scale, col)
	if img == nil {
		return 0
	}
	b := img.Bounds()
	return gtk.NewImageRGBAScaled(img.Pix, b.Dx(), b.Dy(), img.Stride, scale)
}

// placePanel shows the panel where the user last dragged it, or at the work-area
// corner nearest the click.
//
// The order matters and is not the obvious one. A content-hugging layout has no
// size until its children are visible, and neither the preferred size of an
// unrealised window nor the size reported after a bare realise tells the truth -
// both under-report, and the window manager then quietly clamps the window to
// the screen edge, which hides the bad arithmetic behind a result that looks
// almost right. Showing the CONTENT while the toplevel is still unmapped gives
// GTK everything it needs to measure and leaves the window invisible and free
// to move. Both placements need that real size, so both come after ShowContent.
func placePanel(win gtk.Window, content uintptr, at gtk.Rect, want *gui.Point) {
	win.ShowContent(content)
	if x, y, ok := panelOrigin(win, at, want); ok {
		win.Move(x, y)
	}
	win.Show()
}

// panelOrigin decides the top-left corner: the remembered position when it is
// still on a monitor, otherwise the corner nearest the click. A false return
// means leave the window wherever the window manager put it, which is all that
// can be done when neither position is usable - on Wayland, for instance, where
// a client sees no global coordinates at all.
//
// It reads the window size, so it must be called after ShowContent.
func panelOrigin(win gtk.Window, at gtk.Rect, want *gui.Point) (int, int, bool) {
	w, h := win.Size()

	if want != nil {
		if area, ok := rememberedArea(want.X, want.Y); ok {
			x, y := clampToArea(want.X, want.Y, w, h, area)
			return x, y, true
		}
		// Worth a line: the panel is about to ignore a position the user chose
		// by dragging it, and the reason is invisible from the outside.
		log.Printf("forecast: remembered position %d,%d is on no current monitor, using the corner nearest the pointer", want.X, want.Y)
	}

	if at.W <= 0 || at.H <= 0 {
		return 0, 0, false
	}
	area, ok := gtk.WorkAreaAt(at.X, at.Y)
	if !ok {
		return 0, 0, false
	}
	x, y := corner(at.X, at.Y, w, h, area)
	return x, y, true
}

// rememberedArea answers the two questions a remembered position raises, in the
// order they have to be asked: is that point still on a monitor at all, and if it
// is, which usable rectangle should the panel be clamped into. False means no
// monitor contains it - the display it was saved on is gone, or the screens were
// rearranged - and the caller falls back to the pointer corner.
//
// Containment is tested against monitor GEOMETRY and clamping done against the
// WORK AREA, and the two being different rectangles is the point. Testing
// containment against the work area is what made the backends disagree: Win32
// asks MonitorFromRect, which is geometry with the taskbar included, so a panel
// dropped with its top-left corner over a dock or a top panel was clamped back
// into view on Windows and silently forgotten here.
//
// Must be called on the GTK thread - both calls reach into GDK.
func rememberedArea(x, y int) (gtk.Rect, bool) {
	mon, ok := gtk.GeometryAt(x, y)
	if !ok || !inside(mon, x, y) {
		return gtk.Rect{}, false
	}
	return gtk.WorkAreaAt(x, y)
}

// inside reports whether a point lies in a rectangle.
//
// It exists because gtk.GeometryAt cannot answer "no monitor here":
// gdk_display_get_monitor_at_point falls back to the NEAREST monitor when the
// point is off every screen, so a position remembered on a monitor that has
// since been unplugged comes back as a perfectly valid rectangle belonging to
// some other monitor. Testing the point against the rectangle we were given is
// what turns that into the "no monitor contains it" answer the caller needs.
func inside(r gtk.Rect, x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// clampToArea slides a remembered origin until the whole panel is inside the
// work area. The area is not necessarily the one the position was saved on: a
// resolution change, a dock appearing or a monitor being rearranged can all
// leave a position that was legitimate when it was written with most of the
// panel hanging off an edge, and a panel whose × button is off-screen while it
// is pinned cannot be closed from the window at all.
//
// The lower bounds are applied last on purpose. When the panel is larger than
// the work area - a small screen with a large font scale - the upper bound comes
// out below the lower one, and the top-left corner is then the only answer worth
// having: the header row and the first days stay readable, whereas honouring the
// bottom edge would push exactly those off the top.
func clampToArea(x, y, w, h int, area gtk.Rect) (int, int) {
	if x+w > area.X+area.W {
		x = area.X + area.W - w
	}
	if y+h > area.Y+area.H {
		y = area.Y + area.H - h
	}
	if x < area.X {
		x = area.X
	}
	if y < area.Y {
		y = area.Y
	}
	return x, y
}

// corner picks the work-area corner on the same side as the pointer, so the
// panel opens towards the middle of the screen rather than off its edge. The
// work area is deliberately not the monitor geometry: the difference is exactly
// the desktop panels, and ignoring it puts the forecast underneath them.
func corner(px, py, w, h int, area gtk.Rect) (int, int) {
	x := area.X + panelMargin
	if px > area.X+area.W/2 {
		x = area.X + area.W - w - panelMargin
	}
	y := area.Y + panelMargin
	if py > area.Y+area.H/2 {
		y = area.Y + area.H - h - panelMargin
	}
	// Clamped to the work area's own origin, which is what forecast_windows.go's
	// panelCorner does and this did not. A panel wider or taller than the work area
	// - a small screen, or a text-scaling factor that grows the table - otherwise
	// gets a negative origin from the arithmetic above and opens with its header row
	// and first days off the top or left edge, where nothing clamps it afterwards.
	// Showing the top-left corner of an oversized panel is the lesser evil.
	if x < area.X {
		x = area.X
	}
	if y < area.Y {
		y = area.Y
	}
	return x, y
}

// paletteFor picks the panel palette. For an explicit setting it honours the
// user; for "auto" it asks the desktop theme which way round it draws text - a
// light foreground means a dark theme. That is more reliable than the GTK
// prefer-dark flag, which stays false under themes that are simply dark, such
// as BlackMATE.
//
// Asked in the Modern look only. The system look states no palette, so the theme
// setting does not reach the panel there at all - it still drives the tray icon
// and it still drives this, which is why it is neither removed nor renamed.
func paletteFor(win gtk.Window, theme string) string {
	switch theme {
	case "dark":
		return "dark"
	case "light":
		return "light"
	}
	if luminance(win.Foreground()) > 0.5 {
		return "dark"
	}
	return "light"
}

// paletteForeground is the ink the palette draws cell text in, and therefore the
// colour a rasterised symbol has to be tinted with to look like part of the
// table.
//
// The value is taken from the stylesheet rather than from win.Foreground(),
// which is what the DESKTOP theme draws text in. Those two disagree whenever the
// user forces a palette the desktop does not share - a dark panel on a light
// theme would otherwise get a near-black symbol on a near-black row. Both
// literals below are the `label { color: ... }` declarations in style_linux.go
// and the text members of the Win32 palettes.
//
// WHICH IS THE MODERN LOOK'S ANSWER, and the system look's is the opposite one:
// win.Foreground(), taken straight from the theme. That is not an inconsistency,
// it is this function's own reasoning applied to a case that inverts it. The
// mismatch guarded against above needs a palette that was forced on the window,
// and the system look forces none - it adds no palette class, the theme paints the
// labels, and so the theme's ink is what the symbol beside them has to match. Ask
// this only where a palette class was actually applied.
func paletteForeground(palette string) color.NRGBA {
	if palette == "dark" {
		return color.NRGBA{R: 0xf2, G: 0xf4, B: 0xf7, A: 0xff} // .dark label
	}
	return color.NRGBA{R: 0x14, G: 0x16, B: 0x1a, A: 0xff} // .light label
}

func luminance(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

func forecastFailed(l i18n.Lang) string {
	if l == i18n.UK {
		return "Не вдалося завантажити прогноз."
	}
	return "Failed to load forecast."
}

func closeLabel(l i18n.Lang) string {
	if l == i18n.UK {
		return "Закрити"
	}
	return "Close"
}
