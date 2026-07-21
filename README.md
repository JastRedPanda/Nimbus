# Nimbus

**Weather tray app** | **Інформер погоди в системному треї**

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).  
Languages: English, Українська

## Features / Можливості

- Temperature text in system tray / Текст температури в системному треї
- **7-day Forecast** popup window (Windows) / Спливаюче вікно прогнозу на 7 днів (Windows)
- **Settings GUI** (Windows) — city, units, theme, language, font scale, update interval / Вікно налаштувань з графічним інтерфейсом
- Temperature unit: °C / °F
- Pressure unit: hPa / mmHg / inHg
- Wind unit: m/s / km/h
- Font scale: slider 1–100% for tray text
- Update interval: 5 min – 24 hours
- Icon theme: Dark / Light (for settings & forecast windows)
- Language: English / Українська
- No console window (Windows)

## Download / Завантажити

Pre-built binaries: [Releases](https://github.com/JastRedPanda/Nimbus/releases)

## Build from source / Збірка з вихідного коду

### Requirements / Вимоги
- Go 1.21+

### Windows
```bash
go build -ldflags="-s -w -H windowsgui" -o nimbus.exe .
```

### Linux
```bash
GOOS=linux GOARCH=amd64 go build -o nimbus .
```

### Static build (Linux)
```bash
go build -ldflags="-s -w" -o nimbus .
```

## Usage / Використання

Just run the binary — it appears in the system tray.  
Просто запустіть — з'явиться в системному треї.

| Action | Result |
|---|---|
| **Tray text** | Temperature with +/- sign (e.g. `+15°C`) |
| **Hover** | Detailed tooltip (condition, feels like, humidity, wind, pressure) |
| **Left-click** | Opens **7-day Forecast** popup (Windows) |
| **Right-click → Forecast** | Opens forecast in browser (Linux) |
| **Right-click → Settings...** | Opens **Settings GUI** (Windows: native window / Linux: opens config file) |
| **Right-click → Refresh now** | Forces weather update |
| **Right-click → About** | Opens GitHub page |
| **Right-click → Quit** | Exits app |

### Settings window / Вікно налаштувань

- City search with autocomplete / Пошук міста з автодоповненням
- Manual lat/lon entry / Ручне введення координат
- Temperature, pressure, wind unit selection / Вибір одиниць вимірювання
- Window theme (Auto/Dark/Light) / Тема вікон
- Language switch / Перемикання мови
- Font scale slider (1–100%) / Слайдер масштабу шрифту
- Update interval dropdown (5 min – 24 h) / Вибір інтервалу оновлення
- Dark mode support (Windows) / Підтримка темної теми

### Configuration / Конфігурація

Auto-created at first run:  
Автоматично створюється при першому запуску:

- **Windows**: `%APPDATA%\Nimbus\config.json`
- **Linux**: `~/.config/nimbus/config.json`

```json
{
  "latitude": 50.4501,
  "longitude": 30.5234,
  "city_name": "Kyiv",
  "update_interval": 10,
  "units": "celsius",
  "pressure_unit": "hpa",
  "wind_unit": "ms",
  "icon_theme": "auto",
  "language": "en",
  "font_scale": 100
}
```

| Field | Values | Description |
|---|---|---|
| `latitude`, `longitude` | float | Coordinates / Координати |
| `city_name` | string | Display name / Назва міста |
| `update_interval` | int (minutes) | Refresh interval / Інтервал оновлення (хв) |
| `units` | `celsius` / `fahrenheit` | Temperature unit / Одиниця температури |
| `pressure_unit` | `hpa` / `mmhg` / `inhg` | Pressure unit / Одиниця тиску |
| `wind_unit` | `ms` / `kmh` | Wind unit / Одиниця вітру |
| `icon_theme` | `auto` / `dark` / `light` | Window theme / Тема вікон |
| `language` | `en` / `uk` | UI language / Мова інтерфейсу |
| `font_scale` | int 1–100 | Tray font scale % / Масштаб шрифту в треї (%) |

## Weather API

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.  
IP geolocation via [ip-api.com](http://ip-api.com/).

## License

GNU General Public License v3.0
