package tray

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/icons"
	"github.com/JastRedPanda/Nimbus/internal/ui"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/getlantern/systray"
)

type app struct {
	cfg      *config.Config
	lang     i18n.Lang
	lastData *weather.WeatherData

	mForecast *systray.MenuItem
	mSettings *systray.MenuItem
	mAbout    *systray.MenuItem
	mQuit     *systray.MenuItem
}

func Run(cfg *config.Config) {
	systray.Run(func() { (&app{cfg: cfg, lang: i18n.ParseLang(cfg.Language)}).ready() }, func() {})
}

func (a *app) ready() {
	icon := icons.GenerateScale(20, 0, "auto", a.cfg.FontScale)
	if icon != nil {
		systray.SetIcon(icon)
	}
	systray.SetTooltip("Nimbus — loading...")

	a.mForecast = systray.AddMenuItem("7-day Forecast", "Open 7-day forecast")
	systray.AddSeparator()
	a.mSettings = systray.AddMenuItem("Settings...", "Configure Nimbus")
	systray.AddSeparator()
	a.mAbout = systray.AddMenuItem("About", "About Nimbus")
	a.mQuit = systray.AddMenuItem("Quit", "Quit Nimbus")

	a.fetchAndUpdate()
	go a.handleMenu()
}

func (a *app) handleMenu() {
	for {
		select {
		case <-a.mForecast.ClickedCh:
			a.showForecast()
		case <-a.mSettings.ClickedCh:
			a.openSettings()
		case <-a.mAbout.ClickedCh:
			a.openURL("https://github.com/JastRedPanda/Nimbus")
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
	a.lastData = data
	a.updateIcon(a.cfg.FontScale)
}

func (a *app) updateIcon(fontScale int) {
	if a.lastData == nil {
		return
	}
	data := a.lastData
	temp := data.Temperature
	apparent := data.ApparentTemp
	if a.cfg.Units == "fahrenheit" {
		temp = temp*9/5 + 32
		apparent = apparent*9/5 + 32
	}

	icon := icons.GenerateScale(temp, data.WeatherCode, a.cfg.IconTheme, fontScale)
	if icon != nil {
		systray.SetIcon(icon)
	}

	detail := a.lang.Tooltip("", data.WeatherCode, temp, apparent,
		int(data.Humidity), data.WindSpeed, data.SurfacePressure,
		a.cfg.Units, a.cfg.PressureUnit, a.cfg.WindUnit)
	systray.SetTooltip(detail)
}

func (a *app) showForecast() {
	ui.ShowForecast(a.cfg.Latitude, a.cfg.Longitude, a.cfg.Units, a.cfg.Language, a.cfg.IconTheme, a.cfg.WindUnit)
}

func (a *app) openSettings() {
	go func() {
		nc := ui.ShowSettings(a.cfg, func(fs int) { a.updateIcon(fs) })
		if nc == nil {
			return
		}
		a.cfg = nc
		if a.cfg.Language != string(a.lang) {
			a.lang = i18n.ParseLang(a.cfg.Language)
		}
		a.fetchAndUpdate()
	}()
}

func (a *app) openURL(url string) {
	switch runtime.GOOS {
	case "windows":
		exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}
