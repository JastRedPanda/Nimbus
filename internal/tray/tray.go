package tray

import (
	"fmt"
	"log"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/icons"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/getlantern/systray"
)

type app struct {
	cfg      *config.Config
	lang     i18n.Lang
	mRefresh *systray.MenuItem
	mUnits   *systray.MenuItem
	mLang    *systray.MenuItem
	mAbout   *systray.MenuItem
	mQuit    *systray.MenuItem
}

func Run(cfg *config.Config) {
	systray.Run(func() { (&app{cfg: cfg, lang: i18n.ParseLang(cfg.Language)}).ready() }, func() {})
}

func (a *app) ready() {
	systray.SetIcon(icons.Generate(15, 0))
	systray.SetTooltip("Nimbus — loading...")
	systray.SetTitle("Nimbus")

	a.mRefresh = systray.AddMenuItem(a.lang.Refresh(), a.lang.RefreshTooltip())
	systray.AddSeparator()
	a.mUnits = systray.AddMenuItem(a.lang.UnitsLabel(a.cfg.Units), a.lang.UnitsTooltip())
	a.mLang = systray.AddMenuItem(a.lang.LanguageLabel(a.lang), a.lang.LanguageTooltip())
	a.mAbout = systray.AddMenuItem(a.lang.About(), a.lang.AboutTooltip())
	systray.AddSeparator()
	a.mQuit = systray.AddMenuItem(a.lang.Quit(), a.lang.QuitTooltip())

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
			a.mUnits.SetTitle(a.lang.UnitsLabel(a.cfg.Units))
			_ = a.cfg.Save()
			a.fetchAndUpdate()
		case <-a.mLang.ClickedCh:
			if a.lang == i18n.EN {
				a.lang = i18n.UK
				a.cfg.Language = "uk"
			} else {
				a.lang = i18n.EN
				a.cfg.Language = "en"
			}
			a.updateMenuLabels()
			_ = a.cfg.Save()
		case <-a.mAbout.ClickedCh:
			systray.SetTooltip("Nimbus — Weather tray app | github.com/JastRedPanda/Nimbus")
		case <-a.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *app) updateMenuLabels() {
	a.mRefresh.SetTitle(a.lang.Refresh())
	a.mRefresh.SetTooltip(a.lang.RefreshTooltip())
	a.mUnits.SetTitle(a.lang.UnitsLabel(a.cfg.Units))
	a.mUnits.SetTooltip(a.lang.UnitsTooltip())
	a.mLang.SetTitle(a.lang.LanguageLabel(a.lang))
	a.mLang.SetTooltip(a.lang.LanguageTooltip())
	a.mAbout.SetTitle(a.lang.About())
	a.mAbout.SetTooltip(a.lang.AboutTooltip())
	a.mQuit.SetTitle(a.lang.Quit())
	a.mQuit.SetTooltip(a.lang.QuitTooltip())
	a.fetchAndUpdate()
}

func (a *app) fetchAndUpdate() {
	data, err := weather.Fetch(a.cfg.Latitude, a.cfg.Longitude)
	if err != nil {
		log.Printf("Weather fetch error: %v", err)
		systray.SetTooltip(fmt.Sprintf("Nimbus — error: %v", err))
		return
	}

	temp := data.Temperature
	apparent := data.ApparentTemp
	if a.cfg.Units == "fahrenheit" {
		temp = temp*9/5 + 32
		apparent = apparent*9/5 + 32
	}

	systray.SetIcon(icons.Generate(temp, data.WeatherCode))
	systray.SetTooltip(a.lang.Tooltip(data.Emoji(), data.WeatherCode, temp, apparent, int(data.Humidity), data.WindSpeed, a.cfg.Units))
}
