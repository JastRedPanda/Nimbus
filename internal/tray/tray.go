package tray

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os/exec"
	"runtime"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/forecast"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/icons"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/getlantern/systray"
)

type app struct {
	cfg  *config.Config
	lang i18n.Lang

	mForecast *systray.MenuItem
	mSettings *systray.MenuItem
	mAbout    *systray.MenuItem
	mQuit     *systray.MenuItem
}

func Run(cfg *config.Config) {
	systray.Run(func() { (&app{cfg: cfg, lang: i18n.ParseLang(cfg.Language)}).ready() }, func() {})
}

func (a *app) ready() {
	icon := icons.Generate(20, 0, "auto")
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
			forecast.Show(a.cfg.Latitude, a.cfg.Longitude, a.cfg.Units, a.cfg.Language)
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

	temp := data.Temperature
	apparent := data.ApparentTemp
	if a.cfg.Units == "fahrenheit" {
		temp = temp*9/5 + 32
		apparent = apparent*9/5 + 32
	}

	icon := icons.Generate(temp, data.WeatherCode, a.cfg.IconTheme)
	if icon != nil {
		systray.SetIcon(icon)
	}

	ts := tooltipLine(temp, a.cfg.Units, data.WeatherCode)
	detail := a.lang.Tooltip("", data.WeatherCode, temp, apparent,
		int(data.Humidity), data.WindSpeed, data.SurfacePressure,
		a.cfg.Units, a.cfg.PressureUnit)
	systray.SetTooltip(ts + "\n" + detail)
}

func tooltipLine(temp float64, unitCfg string, code int) string {
	sym := "°C"
	if unitCfg == "fahrenheit" {
		sym = "°F"
	}
	t := int(math.Round(temp))
	sign := ""
	if t > 0 {
		sign = "+"
	}
	return fmt.Sprintf("Nimbus — %s%d%s", sign, t, sym)
}

func (a *app) openSettings() {
	path, err := config.ConfigPath()
	if err != nil {
		log.Printf("Config path error: %v", err)
		return
	}

	if runtime.GOOS == "windows" {
		shim := &cfgShim{
			CityName:     a.cfg.CityName,
			Latitude:     a.cfg.Latitude,
			Longitude:    a.cfg.Longitude,
			Units:        a.cfg.Units,
			PressureUnit: a.cfg.PressureUnit,
			IconTheme:    a.cfg.IconTheme,
			Language:     a.cfg.Language,
		}
		ps := settingsScript(shim, path)
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
		configureCmd(cmd)
		out, err := cmd.Output()
		if err == nil {
			var nc config.Config
			if e := json.Unmarshal(out, &nc); e == nil {
				a.cfg.Latitude = nc.Latitude
				a.cfg.Longitude = nc.Longitude
				a.cfg.CityName = nc.CityName
				if nc.Units != "" {
					a.cfg.Units = nc.Units
				}
				if nc.PressureUnit != "" {
					a.cfg.PressureUnit = nc.PressureUnit
				}
				if nc.IconTheme != "" {
					a.cfg.IconTheme = nc.IconTheme
				}
				if nc.Language != "" {
					a.cfg.Language = nc.Language
				}
				if a.cfg.Language != string(a.lang) {
					a.lang = i18n.ParseLang(a.cfg.Language)
				}
				_ = a.cfg.Save()
				a.fetchAndUpdate()
				return
			}
		}
	}

	switch runtime.GOOS {
	case "windows":
		exec.Command("notepad", path).Start()
	case "darwin":
		exec.Command("open", "-t", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
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
