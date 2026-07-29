//go:build linux && qt

package qt

import (
	"log"

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
// table, the drag, the placement - is in qtshim/shim.cpp, because that is the
// half that has to be written in C++.
//
// The division is not arbitrary. Everything that could be decided in Go IS
// decided in Go: the strings come from internal/i18n, the numbers from
// weather.TempRange and its two siblings, the symbol from internal/fonts, the
// remembered position from the configuration. The shim is handed finished text.

const appName = "Nimbus"

// colAlign is the horizontal alignment of each table column, applied to both the
// header caption and the cells under it so a column reads as one thing. Day
// reads as a label and sits left; the symbol is centred; the three numeric
// columns are right aligned so their digits line up, which is the whole reason a
// table beats a row of cards. The same order as internal/ui's colAlign, because
// it is the same table.
var colAlign = [...]int32{alignStart, alignCenter, alignEnd, alignEnd, alignEnd}

// panelUp says a panel is on screen right now. Read and written only on the Qt
// thread.
var panelUp bool

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
	if !invoke(func() { consumed <- qtPanelClose() != 0 }) {
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

// buildPanel hands the shim a finished table and the callback the window answers
// through. Qt thread only.
// applyTheme hands the theme option to Qt before a window is built. A build
// against Qt older than 6.8 cannot honour it and says so through canTheme; the
// settings window then offers no switch at all, so the only value that reaches
// here is the one already stored.
func applyTheme(theme string) {
	switch theme {
	case "dark":
		qtTheme(1)
	case "light":
		qtTheme(0)
	default:
		qtTheme(-1)
	}
}

// canTheme reports whether this shim can act on the theme option.
func canTheme() bool { return qtCanTheme() != 0 }

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
	applyTheme(req.Theme)

	var id uint64
	id = register(&window{
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
	qtPanelShow(id, have, x, y, eventTramp)
	panelUp = true
}
