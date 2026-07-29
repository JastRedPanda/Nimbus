//go:build linux && qt

package qt

import (
	"log"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/fonts"
	"github.com/JastRedPanda/Nimbus/internal/gui"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

// The 7-day forecast panel, Qt edition.
//
// What is in THIS file is the part that is the same on every backend: fetch off
// the loop, format the columns, decide whether the click was an opening one or a
// closing one, and remember where the panel was left. The window itself - the
// table, the drag, the dismissal policy, the placement - is in qtshim/shim.cpp,
// because that is the half that has to be written in C++.
//
// The division is not arbitrary. Everything that could be decided in Go IS
// decided in Go: the strings come from internal/i18n, the numbers from
// weather.TempRange and its two siblings, the symbol from internal/fonts, the
// remembered position from the configuration. The shim is handed finished text.

const (
	appName = "Nimbus"

	// toggleGrace is how long after the panel closed itself a tray click still
	// counts as the click that closed it, rather than a fresh request to open.
	//
	// Clicking the tray icon can take focus away from the panel, and the panel
	// closes on focus loss, so the close can land BEFORE the host delivers the
	// click. Without this the second click would reopen the panel instead of
	// dismissing it.
	toggleGrace = 400 * time.Millisecond
)

// colAlign is the horizontal alignment of each table column, applied to both the
// header caption and the cells under it so a column reads as one thing. Day
// reads as a label and sits left; the symbol is centred; the three numeric
// columns are right aligned so their digits line up, which is the whole reason a
// table beats a row of cards. The same order as internal/ui's colAlign, because
// it is the same table.
var colAlign = [...]int32{alignStart, alignCenter, alignEnd, alignEnd, alignEnd}

// Read and written only on the Qt thread. panelClosedAt is when the panel last
// went away by losing focus, which the tray toggle needs to tell a closing click
// from an opening one; panelUp says one is on screen right now.
var (
	panelUp       bool
	panelClosedAt time.Time
)

// Forecast opens the panel, or closes the one that is up. It returns
// immediately: the forecast is fetched on its own goroutine because the caller
// is the tray's single menu-dispatch loop, and a blocking ten-second HTTP call
// there would freeze Settings, About and Quit along with it.
func (backend) Forecast(req gui.Forecast) {
	// Whether the click was consumed is decided on the Qt thread, NOW, while the
	// user's click is still fresh - not after the fetch, which can take ten
	// seconds. A toggle that closed the panel only once the network answered
	// would be no toggle at all.
	consumed := make(chan bool, 1)
	if !invoke(func() {
		if qtPanelClose() != 0 {
			consumed <- true
			return
		}
		if !panelClosedAt.IsZero() && time.Since(panelClosedAt) < toggleGrace {
			// Worth a line: to the user this click did nothing at all.
			log.Print("qt: forecast click within the toggle grace period, treated as the closing click")
			consumed <- true
			return
		}
		consumed <- false
	}) {
		return
	}

	go func() {
		if <-consumed {
			return
		}
		data, err := weather.FetchDaily(req.Lat, req.Lon)
		l := i18n.ParseLang(req.Lang)
		if err != nil {
			log.Printf("qt: forecast fetch failed: %v", err)
		} else if len(data) == 0 {
			log.Print("qt: forecast fetch returned no days")
		}
		if err != nil || len(data) == 0 {
			invoke(func() { qtError(appName, l.ForecastFailed()) })
			return
		}
		invoke(func() { buildPanel(data, req, l) })
	}()
}

// buildPanel hands the shim a finished table and the two callbacks the window
// answers through. Qt thread only.
func buildPanel(data []weather.DailyForecast, req gui.Forecast, l i18n.Lang) {
	if panelUp {
		// Two clicks inside one fetch: the first had not drawn anything yet when
		// the second decided nothing was open, so both went on to fetch and both
		// arrived here. The shim would raise the panel it already has and drop
		// everything built for the second one - including the callbacks registered
		// for it, which would then never be released. Answering here instead keeps
		// that bookkeeping in one place.
		log.Print("qt: a forecast panel is already open; leaving it as it is")
		return
	}
	ensureFont()

	var id uint64
	id = register(&window{
		// Asked at the moment of each event rather than read once here: the tray
		// hands over a function precisely so that unticking the box in settings
		// frees the panel already on screen instead of only the next one. A nil
		// Pinned means not pinned, the behaviour before the option existed.
		pinned: func() bool { return req.Pinned != nil && req.Pinned() },
		event: func(code, a, b int64) {
			switch code {
			case evMoved:
				if req.OnMove == nil {
					return
				}
				// On a goroutine because OnMove writes the configuration file, and
				// a disk write on the Qt thread stalls every window this process
				// owns. The coordinates are already read, so the callback is free
				// to take locks and do I/O - which is the contract all three
				// backends give it.
				onMove := req.OnMove
				go onMove(int(a), int(b))
			case evClosed:
				panelUp = false
				if a == 1 {
					panelClosedAt = time.Now()
				}
				drop(id)
			}
		},
	})

	qtPanelBegin(l.ForecastTitle())

	for i, caption := range l.ForecastHeaders() {
		if i >= len(colAlign) {
			break
		}
		qtPanelHeader(caption, colAlign[i])
	}
	for _, d := range data {
		// The ISO date exactly as Open-Meteo returned it. No weekday name and no
		// localised month: the column is a date, and a sortable one reads the same
		// in both languages.
		qtPanelRow(d.Date,
			fonts.IconForCode(d.WeatherCode),
			weather.TempRange(d, req.Units, l),
			weather.WindSpeed(d, req.WindUnit, l),
			weather.Precip(d, l))
	}

	var have, x, y int32
	if req.At != nil {
		have, x, y = 1, int32(req.At.X), int32(req.At.Y)
	}
	qtPanelShow(id, have, x, y, pinnedTramp, eventTramp)
	panelUp = true
}
