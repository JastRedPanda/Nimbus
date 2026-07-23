package webui

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/JastRedPanda/Nimbus/internal/config"
	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

//go:embed settings.html
var settingsContent string

//go:embed forecast.html
var forecastContent string

//go:embed about.html
var aboutContent string

//go:embed favicon.png
var faviconBytes []byte

//go:embed about_logo.png
var aboutLogoBytes []byte

var (
	settingsTmpl = template.Must(template.New("settings").Parse(settingsContent))
	forecastTmpl = template.Must(template.New("forecast").Parse(forecastContent))
	aboutTmpl    = template.Must(template.New("about").Parse(aboutContent))
)

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Write(faviconBytes)
}

func openURL(url string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

func listen() net.Listener {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil
	}
	return l
}

func ShowSettings(cfg *config.Config) *config.Config {
	l := listen()
	if l == nil {
		return nil
	}

	addr := l.Addr().String()
	openURL("http://" + addr + "/settings")

	res := make(chan *config.Config, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/icon", faviconHandler)
	mux.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		renderSettings(w, cfg)
	})
	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST only", 405)
			return
		}
		nc := parseForm(r, cfg)
		nc.Save()
		res <- nc
		http.Redirect(w, r, "/done", 302)
	})
	mux.HandleFunc("/done", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html><body><p>Saved. You may close this tab.</p></body></html>"))
	})

	go http.Serve(l, mux)

	select {
	case nc := <-res:
		l.Close()
		return nc
	}
}

func ShowAbout() {
	l := listen()
	if l == nil {
		return
	}
	addr := l.Addr().String()
	openURL("http://" + addr + "/about")

	mux := http.NewServeMux()
	mux.HandleFunc("/icon", faviconHandler)
	mux.HandleFunc("/logo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(aboutLogoBytes)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		aboutTmpl.Execute(w, nil)
	})
	http.Serve(l, mux)
}

func ShowForecast(lat, lon float64, units, lang, windUnit string) {
	l := listen()
	if l == nil {
		return
	}

	addr := l.Addr().String()
	openURL("http://" + addr + "/forecast")

	mux := http.NewServeMux()
	mux.HandleFunc("/icon", faviconHandler)
	mux.HandleFunc("/forecast", func(w http.ResponseWriter, r *http.Request) {
		renderForecast(w, lat, lon, units, lang, windUnit)
	})
	go http.Serve(l, mux)
}

func renderSettings(w io.Writer, cfg *config.Config) {
	t := i18n.ParseLang(cfg.Language)

	units := []string{"celsius", "fahrenheit"}
	presUnits := []string{"hpa", "mmhg", "inhg"}
	windUnits := []string{"ms", "kmh"}
	themes := []string{"auto", "dark", "light"}
	langs := []struct{ Value, Label string }{
		{"en", "English"},
		{"uk", "Українська"},
	}
	intervals := []struct{ Minutes int; Label string }{
		{5, "5 min"},
		{30, "30 min"},
		{60, "1 hour"},
		{720, "12 hours"},
		{1440, "24 hours"},
	}

	sel := func(list []string, v string) string {
		for _, x := range list {
			if x == v {
				return "selected"
			}
		}
		return ""
	}
	chk := func(v, target string) string {
		if v == target {
			return "checked"
		}
		return ""
	}

	data := map[string]interface{}{
		"cfg":         cfg,
		"t":           t,
		"units":       units,
		"presUnits":   presUnits,
		"windUnits":   windUnits,
		"themes":      themes,
		"langs":       langs,
		"intervals":   intervals,
		"sel":         sel,
		"chk":         chk,
		"fontScale":   cfg.FontScale,
		"updateInt":   cfg.UpdateInterval,
		"tempUnit":    cfg.Units,
		"presUnit":    cfg.PressureUnit,
		"windUnit":    cfg.WindUnit,
		"iconTheme":   cfg.IconTheme,
		"language":    cfg.Language,
		"cityName":    cfg.CityName,
		"latitude":    fmt.Sprintf("%.4f", cfg.Latitude),
		"longitude":   fmt.Sprintf("%.4f", cfg.Longitude),
	}
	settingsTmpl.Execute(w, data)
}

func renderForecast(w io.Writer, lat, lon float64, units, lang, windUnit string) {
	data, err := weather.FetchDaily(lat, lon)
	if err != nil || len(data) == 0 {
		io.WriteString(w, "<html><body><p>Failed to load forecast.</p></body></html>")
		return
	}

	sym := "°C"
	if units == "fahrenheit" {
		sym = "°F"
	}
	l := i18n.ParseLang(lang)
	title := "7-Day Forecast"
	if l == i18n.UK {
		title = "Прогноз на 7 днів"
	}
	precipUnit := l.PrecipUnit()
	windLabel := l.WindUnitCfg(windUnit)

	type dayRow struct {
		Date       string
		Icon       string
		TempMax    string
		TempMin    string
		Precip     string
		Wind       string
	}
	var rows []dayRow
	for _, d := range data {
		tmax, tmin := d.TempMax, d.TempMin
		if units == "fahrenheit" {
			tmax = tmax*9/5 + 32
			tmin = tmin*9/5 + 32
		}
		ws := d.WindMax
		if windUnit == "ms" {
			ws = ws / 3.6
		}
		rows = append(rows, dayRow{
			Date:    d.Date,
			Icon:    iconForCode(d.WeatherCode),
			TempMax: fmt.Sprintf("%+.0f%s", tmax, sym),
			TempMin: fmt.Sprintf("%+.0f%s", tmin, sym),
			Precip:  fmt.Sprintf("%.1f %s", d.PrecipSum, precipUnit),
			Wind:    fmt.Sprintf("%.1f %s", ws, windLabel),
		})
	}
	forecastTmpl.Execute(w, map[string]interface{}{
		"title": title,
		"rows":  rows,
	})
}

func iconForCode(code int) string {
	switch {
	case code == 0:
		return "☀️"
	case code <= 2:
		return "⛅"
	case code == 3:
		return "☁️"
	case code == 45 || code == 48:
		return "🌫️"
	case code >= 51 && code <= 55:
		return "🌦️"
	case code >= 61 && code <= 65:
		return "🌧️"
	case code >= 71 && code <= 77:
		return "❄️"
	case code >= 80 && code <= 82:
		return "🌧️"
	case code >= 85 && code <= 86:
		return "🌨️"
	case code >= 95:
		return "⛈️"
	default:
		return "🌡️"
	}
}

func parseForm(r *http.Request, old *config.Config) *config.Config {
	r.ParseForm()
	nc := *old
	nc.CityName = r.FormValue("city_name")
	nc.Latitude, _ = strconv.ParseFloat(r.FormValue("latitude"), 64)
	nc.Longitude, _ = strconv.ParseFloat(r.FormValue("longitude"), 64)
	nc.Units = r.FormValue("units")
	nc.PressureUnit = r.FormValue("pressure_unit")
	nc.WindUnit = r.FormValue("wind_unit")
	nc.IconTheme = r.FormValue("icon_theme")
	nc.Language = r.FormValue("language")
	nc.FontScale, _ = strconv.Atoi(r.FormValue("font_scale"))
	if nc.FontScale < 1 {
		nc.FontScale = 100
	}
	if iv, err := strconv.Atoi(r.FormValue("update_interval")); err == nil && iv > 0 {
		nc.UpdateInterval = iv
	}
	return &nc
}
