package tray

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/icons"
	"github.com/JastRedPanda/Nimbus/internal/weather"
	"github.com/getlantern/systray"
)

var buildDate = "07.2026"

type presetCity struct {
	name string
	lat  float64
	lon  float64
}

var presetCities = []presetCity{
	{"Moscow", 55.7558, 37.6173},
	{"London", 51.5074, -0.1278},
	{"Paris", 48.8566, 2.3522},
	{"Berlin", 52.5200, 13.4050},
	{"Madrid", 40.4168, -3.7038},
	{"Rome", 41.9028, 12.4964},
	{"Kyiv", 50.4501, 30.5234},
	{"Warsaw", 52.2297, 21.0122},
	{"New York", 40.7128, -74.0060},
	{"Los Angeles", 34.0522, -118.2437},
	{"Tokyo", 35.6762, 139.6503},
	{"Beijing", 39.9042, 116.4074},
	{"Sydney", -33.8688, 151.2093},
	{"Cairo", 30.0444, 31.2357},
	{"Cape Town", -33.9249, 18.4241},
}

type app struct {
	cfg  *config.Config
	lang i18n.Lang

	mCity      *systray.MenuItem
	mRefresh   *systray.MenuItem
	mAutoDetect *systray.MenuItem
	mEditCfg   *systray.MenuItem
	mResetCfg  *systray.MenuItem

	mTempUnit *systray.MenuItem
	mPresUnit *systray.MenuItem
	mTheme    *systray.MenuItem
	mLang     *systray.MenuItem

	mCitySub []*systray.MenuItem

	mAbout *systray.MenuItem
	mQuit  *systray.MenuItem
}

func Run(cfg *config.Config) {
	systray.Run(func() { (&app{cfg: cfg, lang: i18n.ParseLang(cfg.Language)}).ready() }, func() {})
}

func (a *app) ready() {
	a.setLoadingIcon()
	systray.SetTooltip("Nimbus — loading...")

	a.mRefresh = systray.AddMenuItem(a.lang.Refresh(), a.lang.RefreshTooltip())
	systray.AddSeparator()

	a.mCity = systray.AddMenuItem(a.lang.City()+": "+a.cfg.CityName, "Change city")
	a.mAutoDetect = a.mCity.AddSubMenuItem(a.lang.AutoDetect(), a.lang.AutoDetectTooltip())
	a.mCity.AddSubMenuItem("", "")
	for _, c := range presetCities {
		item := a.mCity.AddSubMenuItem(c.name, fmt.Sprintf("%.4f, %.4f", c.lat, c.lon))
		a.mCitySub = append(a.mCitySub, item)
	}
	a.mCity.AddSubMenuItem("", "")
	a.mEditCfg = a.mCity.AddSubMenuItem(a.lang.EditConfig(), a.lang.EditConfigTooltip())
	a.mResetCfg = a.mCity.AddSubMenuItem(a.lang.ResetConfig(), a.lang.ResetConfigTooltip())

	systray.AddSeparator()

	a.mTempUnit = systray.AddMenuItem(a.lang.UnitLabel(a.cfg.Units), "Toggle °C/°F")
	a.mPresUnit = systray.AddMenuItem(a.lang.PressureUnitLabel(a.cfg.PressureUnit), a.lang.PressureUnitTooltip())
	a.mTheme = systray.AddMenuItem(a.lang.ThemeLabel(a.cfg.IconTheme), a.lang.ThemeTooltip())
	a.mLang = systray.AddMenuItem(a.lang.LanguageLabel(a.lang), a.lang.LanguageTooltip())

	systray.AddSeparator()
	a.mAbout = systray.AddMenuItem(a.lang.About(), a.lang.AboutTooltip())
	a.mQuit = systray.AddMenuItem(a.lang.Quit(), a.lang.QuitTooltip())

	a.fetchAndUpdate()

	go a.updateLoop()
	go a.handleMenu()
	go a.handleCityClicks()
}

func (a *app) handleMenu() {
	for {
		select {
		case <-a.mRefresh.ClickedCh:
			a.fetchAndUpdate()

		case <-a.mAutoDetect.ClickedCh:
			go a.autoDetectCity()

		case <-a.mEditCfg.ClickedCh:
			a.openConfig()

		case <-a.mResetCfg.ClickedCh:
			go a.resetConfig()

		case <-a.mTempUnit.ClickedCh:
			if a.cfg.Units == "celsius" {
				a.cfg.Units = "fahrenheit"
			} else {
				a.cfg.Units = "celsius"
			}
			a.mTempUnit.SetTitle(a.lang.UnitLabel(a.cfg.Units))
			_ = a.cfg.Save()
			a.fetchAndUpdate()

		case <-a.mPresUnit.ClickedCh:
			switch a.cfg.PressureUnit {
			case "hpa":
				a.cfg.PressureUnit = "mmhg"
			case "mmhg":
				a.cfg.PressureUnit = "inhg"
			default:
				a.cfg.PressureUnit = "hpa"
			}
			a.mPresUnit.SetTitle(a.lang.PressureUnitLabel(a.cfg.PressureUnit))
			_ = a.cfg.Save()
			a.fetchAndUpdate()

		case <-a.mTheme.ClickedCh:
			switch a.cfg.IconTheme {
			case "auto":
				a.cfg.IconTheme = "dark"
			case "dark":
				a.cfg.IconTheme = "light"
			default:
				a.cfg.IconTheme = "auto"
			}
			a.mTheme.SetTitle(a.lang.ThemeLabel(a.cfg.IconTheme))
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
			_ = a.cfg.Save()
			a.updateMenuLabels()
			a.fetchAndUpdate()

		case <-a.mAbout.ClickedCh:
			a.openURL("https://github.com/JastRedPanda/Nimbus")

		case <-a.mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *app) handleCityClicks() {
	for i, c := range presetCities {
		item := a.mCitySub[i]
		go func(c presetCity) {
			for range item.ClickedCh {
				a.setCity(c.name, c.lat, c.lon)
			}
		}(c)
	}
}

func (a *app) setCity(name string, lat, lon float64) {
	a.cfg.CityName = name
	a.cfg.Latitude = lat
	a.cfg.Longitude = lon
	_ = a.cfg.Save()
	a.updateMenuLabels()
	a.fetchAndUpdate()
}

func (a *app) updateLoop() {
	ticker := time.NewTicker(a.cfg.Interval())
	defer ticker.Stop()
	for range ticker.C {
		a.fetchAndUpdate()
	}
}

func (a *app) updateMenuLabels() {
	a.mRefresh.SetTitle(a.lang.Refresh())
	a.mRefresh.SetTooltip(a.lang.RefreshTooltip())
	a.mCity.SetTitle(a.lang.City() + ": " + a.cfg.CityName)
	a.mAutoDetect.SetTitle(a.lang.AutoDetect())
	a.mAutoDetect.SetTooltip(a.lang.AutoDetectTooltip())
	a.mEditCfg.SetTitle(a.lang.EditConfig())
	a.mEditCfg.SetTooltip(a.lang.EditConfigTooltip())
	a.mResetCfg.SetTitle(a.lang.ResetConfig())
	a.mResetCfg.SetTooltip(a.lang.ResetConfigTooltip())
	a.mTempUnit.SetTitle(a.lang.UnitLabel(a.cfg.Units))
	a.mPresUnit.SetTitle(a.lang.PressureUnitLabel(a.cfg.PressureUnit))
	a.mPresUnit.SetTooltip(a.lang.PressureUnitTooltip())
	a.mTheme.SetTitle(a.lang.ThemeLabel(a.cfg.IconTheme))
	a.mTheme.SetTooltip(a.lang.ThemeTooltip())
	a.mLang.SetTitle(a.lang.LanguageLabel(a.lang))
	a.mLang.SetTooltip(a.lang.LanguageTooltip())
	a.mAbout.SetTitle(a.lang.About())
	a.mAbout.SetTooltip(a.lang.AboutTooltip())
	a.mQuit.SetTitle(a.lang.Quit())
	a.mQuit.SetTooltip(a.lang.QuitTooltip())
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

	a.setIcon(temp, data.WeatherCode)
	systray.SetTooltip(a.lang.Tooltip(data.Emoji(), data.WeatherCode,
		temp, apparent, int(data.Humidity), data.WindSpeed,
		data.SurfacePressure, a.cfg.Units, a.cfg.PressureUnit))

	systray.SetTitle(tempStr(temp, a.cfg.Units))
}

func (a *app) setLoadingIcon() {
	icon := icons.Generate(15, 0, "auto")
	if icon != nil {
		systray.SetIcon(icon)
	}
}

func (a *app) setIcon(temp float64, code int) {
	icon := icons.Generate(temp, code, a.cfg.IconTheme)
	if icon != nil {
		systray.SetIcon(icon)
	}
}

func (a *app) autoDetectCity() {
	systray.SetTooltip("Nimbus — detecting location...")
	city, lat, lon, err := weather.DetectLocation()
	if err != nil {
		log.Printf("Auto-detect failed: %v", err)
		systray.SetTooltip(fmt.Sprintf("Nimbus — detection failed: %v", err))
		return
	}
	a.setCity(city, lat, lon)
}

func (a *app) openConfig() {
	path, err := config.ConfigPath()
	if err != nil {
		log.Printf("Config path error: %v", err)
		return
	}
	switch runtime.GOOS {
	case "windows":
		exec.Command("cmd", "/c", "start", "", path).Start()
	case "darwin":
		exec.Command("open", path).Start()
	default:
		exec.Command("xdg-open", path).Start()
	}
}

func (a *app) resetConfig() {
	path, err := config.ConfigPath()
	if err == nil {
		os.Remove(path)
	}
	exec.Command(os.Args[0]).Start()
	systray.Quit()
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

func tempStr(temp float64, unitCfg string) string {
	sym := "°C"
	if unitCfg == "fahrenheit" {
		sym = "°F"
	}
	t := int(temp)
	if t > 0 {
		return "+" + fmt.Sprintf("%d%s", t, sym)
	}
	return fmt.Sprintf("%d%s", t, sym)
}
