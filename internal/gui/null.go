package gui

import (
	"log"

	"github.com/JastRedPanda/Nimbus/internal/config"
)

// nullBackend is the guaranteed last resort. It draws nothing and logs what it
// was asked for.
//
// It exists so Current never returns nil - a caller that has to nil-check the
// GUI will eventually forget - and so the app can be started under CI with no
// display at all, which is how a headless build gets smoke-tested.
type nullBackend struct{}

func (nullBackend) Name() string { return "null" }

func (nullBackend) Settings(*config.Config, func(int)) *config.Config {
	log.Print("gui: no usable backend, settings unavailable")
	return nil
}

func (nullBackend) Forecast(Forecast) { log.Print("gui: no usable backend, forecast unavailable") }
func (nullBackend) About(string)      { log.Print("gui: no usable backend, about unavailable") }

func (nullBackend) Error(title, message string) {
	log.Printf("gui: %s: %s", title, message)
}

func init() {
	Register(Factory{
		Name: "null",
		// Below every real backend and below the browser fallback, so it is
		// reached only when nothing else can draw at all.
		Rank:  -100,
		Probe: func() bool { return true },
		Open:  func() Backend { return nullBackend{} },
	})
}
