package i18n

import "fmt"

type Lang string

const (
	EN Lang = "en"
	UK Lang = "uk"
)

func ParseLang(s string) Lang {
	switch s {
	case "uk":
		return UK
	default:
		return EN
	}
}

func (l Lang) String() string {
	return string(l)
}

func (l Lang) Condition(code int) string {
	switch l {
	case UK:
		return conditionUK(code)
	default:
		return conditionEN(code)
	}
}

func conditionEN(code int) string {
	switch {
	case code == 0:
		return "Clear"
	case code <= 3:
		return "Partly cloudy"
	case code == 45 || code == 48:
		return "Foggy"
	case code >= 51 && code <= 55:
		return "Drizzle"
	case code >= 61 && code <= 65:
		return "Rain"
	case code >= 71 && code <= 77:
		return "Snow"
	case code >= 80 && code <= 82:
		return "Rain showers"
	case code >= 85 && code <= 86:
		return "Snow showers"
	case code >= 95:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

func conditionUK(code int) string {
	switch {
	case code == 0:
		return "Ясно"
	case code <= 3:
		return "Мінлива хмарність"
	case code == 45 || code == 48:
		return "Туман"
	case code >= 51 && code <= 55:
		return "Мряка"
	case code >= 61 && code <= 65:
		return "Дощ"
	case code >= 71 && code <= 77:
		return "Сніг"
	case code >= 80 && code <= 82:
		return "Злива"
	case code >= 85 && code <= 86:
		return "Снігопад"
	case code >= 95:
		return "Гроза"
	default:
		return "Невідомо"
	}
}

func (l Lang) Settings() string {
	if l == UK {
		return "Налаштування"
	}
	return "Settings"
}

func (l Lang) Refresh() string {
	if l == UK {
		return "Оновити зараз"
	}
	return "Refresh now"
}

func (l Lang) RefreshTooltip() string {
	if l == UK {
		return "Отримати актуальну погоду"
	}
	return "Fetch latest weather"
}

func unitSymbol(unit string) string {
	if unit == "fahrenheit" {
		return "°F"
	}
	return "°C"
}

func (l Lang) UnitLabel(unit string) string {
	sym := unitSymbol(unit)
	if l == UK {
		return "Температура: " + sym
	}
	return "Temperature: " + sym
}

func (l Lang) Celsius() string { return "°C" }
func (l Lang) Fahrenheit() string { return "°F" }

func (l Lang) PressureUnitLabel(unit string) string {
	if l == UK {
		switch unit {
		case "mmhg":
			return "Тиск: мм рт. ст."
		case "inhg":
			return "Тиск: inHg"
		default:
			return "Тиск: гПа"
		}
	}
	switch unit {
	case "mmhg":
		return "Pressure: mmHg"
	case "inhg":
		return "Pressure: inHg"
	default:
		return "Pressure: hPa"
	}
}

func (l Lang) PressureUnitTooltip() string {
	if l == UK {
		return "Змінити одиницю тиску"
	}
	return "Change pressure unit"
}

func (l Lang) ThemeLabel(theme string) string {
	if l == UK {
		switch theme {
		case "dark":
			return "Тема: Темна"
		case "light":
			return "Тема: Світла"
		default:
			return "Тема: Авто"
		}
	}
	switch theme {
	case "dark":
		return "Theme: Dark"
	case "light":
		return "Theme: Light"
	default:
		return "Theme: Auto"
	}
}

func (l Lang) ThemeTooltip() string {
	if l == UK {
		return "Змінити тему іконок"
	}
	return "Change icon theme"
}

func (l Lang) City() string {
	if l == UK {
		return "Місто"
	}
	return "City"
}

func (l Lang) AutoDetect() string {
	if l == UK {
		return "Автовизначення"
	}
	return "Auto-detect"
}

func (l Lang) AutoDetectTooltip() string {
	if l == UK {
		return "Визначити місто за IP"
	}
	return "Detect city by IP"
}

func (l Lang) EditConfig() string {
	if l == UK {
		return "Редагувати конфіг..."
	}
	return "Edit config..."
}

func (l Lang) EditConfigTooltip() string {
	if l == UK {
		return "Відкрити файл конфігурації"
	}
	return "Open configuration file"
}

func (l Lang) LanguageLabel(cur Lang) string {
	if l == UK {
		if cur == UK {
			return "Мова: Українська"
		}
		return "Мова: English"
	}
	if cur == UK {
		return "Language: Ukrainian"
	}
	return "Language: English"
}

func (l Lang) LanguageTooltip() string {
	if l == UK {
		return "Змінити мову"
	}
	return "Switch language"
}

func (l Lang) About() string {
	if l == UK {
		return "Про програму"
	}
	return "About"
}

func (l Lang) AboutTooltip() string {
	if l == UK {
		return "Про Nimbus"
	}
	return "About Nimbus"
}

func (l Lang) Quit() string {
	if l == UK {
		return "Вийти"
	}
	return "Quit"
}

func (l Lang) QuitTooltip() string {
	if l == UK {
		return "Вийти з Nimbus"
	}
	return "Quit Nimbus"
}

func (l Lang) mmHg() string {
	if l == UK {
		return "мм рт. ст."
	}
	return "mmHg"
}

func (l Lang) hPa() string {
	return "hPa"
}

func (l Lang) inHg() string {
	return "inHg"
}

func formatPressure(hPa float64, unit string, lang Lang) string {
	switch unit {
	case "mmhg":
		return fmt.Sprintf("%.0f %s", hPa*0.750064, lang.mmHg())
	case "inhg":
		return fmt.Sprintf("%.2f %s", hPa*0.02953, lang.inHg())
	default:
		return fmt.Sprintf("%.0f %s", hPa, lang.hPa())
	}
}

func (l Lang) Tooltip(emoji string, weatherCode int, temp, apparent float64, humidity int, windSpeed, pressure float64, unit, pressureUnit string) string {
	sym := unitSymbol(unit)
	cond := conditionEN(weatherCode)
	windStr := formatWindEN(windSpeed)
	pressureStr := formatPressure(pressure, pressureUnit, EN)

	if l == UK {
		cond = conditionUK(weatherCode)
		windStr = formatWindUK(windSpeed)
		pressureStr = formatPressure(pressure, pressureUnit, UK)
		return fmt.Sprintf("%s %.0f%s | %s | Відчувається %.0f%s | 💧%d%% | 💨%s | %s",
			emoji, temp, sym, cond, apparent, sym, humidity, windStr, pressureStr)
	}
	return fmt.Sprintf("%s %.0f%s | %s | Feels %.0f%s | 💧%d%% | 💨%s | %s",
		emoji, temp, sym, cond, apparent, sym, humidity, windStr, pressureStr)
}

func (l Lang) WeatherLine(emoji string, temp float64, unit string) string {
	return fmt.Sprintf("%.0f%s %s", temp, unitSymbol(unit), emoji)
}

func formatWindEN(speed float64) string {
	return fmt.Sprintf("%.0f km/h", speed)
}

func formatWindUK(speed float64) string {
	return fmt.Sprintf("%.0f км/год", speed)
}
