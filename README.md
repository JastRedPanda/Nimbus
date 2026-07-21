# Nimbus v1.0.0

**Інформер погоди в системному треї** | **Weather tray app**

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).

Languages: English, Українська

## Features / Можливості

- Weather in system tray with temperature + weather icon / Погода в треї з температурою та іконкою
- **7-day Forecast** — opens in browser / Прогноз на 7 днів у браузері
- **Settings window** (Windows) — city, units, theme, language / Вікно налаштувань
- Temperature unit: °C / °F
- Pressure unit: hPa / mmHg / inHg
- Icon theme: Auto (temperature color) / Dark / Light
- Language: English / Українська
- No console window (Windows)

## Download

Pre-built binaries: [Releases](https://github.com/JastRedPanda/Nimbus/releases)

## Build from source

### Requirements
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
| **Tray icon** | Shows weather symbol colored by temperature |
| **Tray text** | Temperature with +/- sign (e.g. `+15°C`) |
| **Hover** | Detailed tooltip (condition, feels like, humidity, wind, pressure) |
| **Click** → **7-day Forecast** | Opens Open-Meteo forecast in browser |
| **Right-click** → **Settings...** | Opens settings window (Windows: PowerShell GUI / other: config file) |
| **Right-click** → **About** | Opens GitHub page |
| **Right-click** → **Quit** | Exits app |

### Configuration

Auto-created at first run:
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
  "icon_theme": "auto",
  "language": "en"
}
```

| Field | Values | Description |
|---|---|---|
| `latitude`, `longitude` | float | Coordinates |
| `city_name` | string | Display name |
| `update_interval` | int (minutes) | Refresh interval (min 1) |
| `units` | `celsius` / `fahrenheit` | Temperature unit |
| `pressure_unit` | `hpa` / `mmhg` / `inhg` | Pressure unit |
| `icon_theme` | `auto` / `dark` / `light` | Icon theme |
| `language` | `en` / `uk` | UI language |

## Weather API

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.
IP geolocation via [ip-api.com](http://ip-api.com/).

## License

GNU General Public License v3.0
