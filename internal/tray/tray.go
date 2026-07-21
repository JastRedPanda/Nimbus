package tray

import (
	"fmt"
	"log"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/icons"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/getlantern/systray"
)

type app struct {
	cfg      *config.Config
	mRefresh *systray.MenuItem
	mUnits   *systray.MenuItem
	mAbout   *systray.MenuItem
	mQuit    *systray.MenuItem
}

func Run(cfg *config.Config) {
	systray.Run(func() { (&app{cfg: cfg}).ready() }, func() {})
}

func (a *app) ready() {
	systray.SetIcon(icons.Generate(15, 0))
	systray.SetTooltip("Nimbus — loading...")
	systray.SetTitle("Nimbus")

	a.mRefresh = systray.AddMenuItem("Refresh now", "Fetch latest weather")
	systray.AddSeparator()
	a.mUnits = systray.AddMenuItem(fmt.Sprintf("Units: %s", a.cfg.Units), "Toggle °C/°F")
	a.mAbout = systray.AddMenuItem("About", "About Nimbus")
	systray.AddSeparator()
	a.mQuit = systray.AddMenuItem("Quit", "Quit Nimbus")

	a.fetchAndUpdate()

	go a.updateLoop()
	go a.handleMenu()
}

func (a *app) updateLoop() {
	ticker := time.NewTicker(a.cfg.Interval())
	defer ticker.Stop()

	for range ticker.C {
		a.fetchAndUpdate()
	}
}

func (a *app) handleMenu() {
	for {
		select {
		case <-a.mRefresh.ClickedCh:
			a.fetchAndUpdate()
		case <-a.mUnits.ClickedCh:
			if a.cfg.Units == "celsius" {
				a.cfg.Units = "fahrenheit"
			} else {
				a.cfg.Units = "celsius"
			}
			a.mUnits.SetTitle(fmt.Sprintf("Units: %s", a.cfg.Units))
			_ = a.cfg.Save()
			a.fetchAndUpdate()
		case <-a.mAbout.ClickedCh:
			systray.SetTooltip("Nimbus — Weather tray app | github.com/JastRedPanda/Nimbus")
		case <-a.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *app) fetchAndUpdate() {
	data, err := weather.Fetch(a.cfg.Latitude, a.cfg.Longitude)
	if err != nil {
		log.Printf("Weather fetch error: %v", err)
		systray.SetTooltip(fmt.Sprintf("Nimbus — error: %v", err))
		return
	}

	temp := data.Temperature
	if a.cfg.Units == "fahrenheit" {
		temp = temp*9/5 + 32
	}

	systray.SetIcon(icons.Generate(temp, data.WeatherCode))
	systray.SetTooltip(data.Tooltip())
}
