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

func (l Lang) UnitsLabel(unit string) string {
	sym := unitSymbol(unit)
	if l == UK {
		return "Одиниці: " + sym
	}
	return "Units: " + sym
}

func (l Lang) UnitsTooltip() string {
	if l == UK {
		return "Перемкнути °C/°F"
	}
	return "Toggle °C/°F"
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

func (l Lang) Tooltip(emoji string, weatherCode int, temp, apparent float64, humidity int, windSpeed float64, unit string) string {
	sym := unitSymbol(unit)
	if l == UK {
		return fmt.Sprintf("%s %.0f%s | %s | Відчувається %.0f%s | 💧%d%% | 💨%s",
			emoji, temp, sym, conditionUK(weatherCode), apparent, sym, humidity, formatWindUK(windSpeed))
	}
	return fmt.Sprintf("%s %.0f%s | %s | Feels %.0f%s | 💧%d%% | 💨%s",
		emoji, temp, sym, conditionEN(weatherCode), apparent, sym, humidity, formatWindEN(windSpeed))
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
