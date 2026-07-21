package forecast

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/JastRedPanda/Nimbus/internal/i18n"
	"github.com/JastRedPanda/Nimbus/internal/weather"
)

func Show(lat, lon float64, units, lang string) {
	data, err := weather.FetchDaily(lat, lon)
	if err != nil {
		return
	}
	html := renderHTML(data, units, lang)
	tmp, _ := os.CreateTemp("", "nimbus-forecast-*.html")
	if tmp == nil {
		return
	}
	tmp.WriteString(html)
	tmp.Close()

	switch runtime.GOOS {
	case "windows":
		exec.Command("cmd", "/c", "start", tmp.Name()).Start()
	case "darwin":
		exec.Command("open", tmp.Name()).Start()
	default:
		exec.Command("xdg-open", tmp.Name()).Start()
	}
}

func renderHTML(data []weather.DailyForecast, units, lang string) string {
	sym := "°C"
	if units == "fahrenheit" {
		sym = "°F"
	}
	l := i18n.ParseLang(lang)

	rows := ""
	for _, d := range data {
		tmax, tmin := d.TempMax, d.TempMin
		if units == "fahrenheit" {
			tmax = tmax*9/5 + 32
			tmin = tmin*9/5 + 32
		}
		rows += fmt.Sprintf(`<div class="day">
<div class="date">%s</div>
<div class="icon">%s</div>
<div class="temps"><span class="max">%s%.0f%s</span> <span class="min">%s%.0f%s</span></div>
<div class="extra">💧 %.1f mm | 💨 %.0f km/h</div>
</div>`, d.Date, emoji(d.WeatherCode), sign(tmax), tmax, sym, sign(tmin), tmin, sym, d.PrecipSum, d.WindMax)
	}

	title := "7-Day Forecast"
	if l == i18n.UK {
		title = "Прогноз на 7 днів"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Nimbus — %s</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#111;color:#eee;padding:20px;max-width:600px;margin:0 auto}
h1{text-align:center;font-size:20px;margin-bottom:16px;color:#888}
.days{display:flex;flex-direction:column;gap:8px}
.day{display:flex;align-items:center;gap:12px;background:#1c1c1e;border-radius:12px;padding:12px 16px}
.date{width:50px;font-size:13px;color:#888;flex-shrink:0}
.icon{font-size:24px;width:32px;text-align:center;flex-shrink:0}
.temps{flex:1;text-align:right;font-size:16px}
.max{color:#fff;font-weight:600}
.min{color:#888;margin-left:8px}
.extra{font-size:11px;color:#555;flex-shrink:0;text-align:right;width:120px}
</style>
</head><body>
<h1>%s</h1>
<div class="days">%s</div>
</body></html>`, l, title, title, rows)
}

func emoji(code int) string {
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

func sign(v float64) string {
	if v > 0 {
		return "+"
	}
	return ""
}


