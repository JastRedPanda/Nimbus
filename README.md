# Nimbus v1.0.0

**Інформер погоди в системному треї** | **Weather tray app**

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).

Languages: English, Українська

## Features / Можливості

- Current weather in system tray / Погода в системному треї
- **Settings / Налаштування**:
  - City: auto-detect by IP or choose from presets / Місто: автовизначення за IP або вибір зі списку
  - Temperature: °C / °F / Температура: °C / °F
  - Pressure: hPa / mmHg / inHg / Тиск: гПа / мм рт. ст. / inHg
  - Icon theme: Auto / Light / Dark / Тема іконок: Авто / Світла / Темна
  - Language: English / Українська
- No console window (Windows) / Без консольного вікна (Windows)

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

### Right-click menu / Контекстне меню

| English | Українська |
|---|---|
| Refresh now | Оновити зараз |
| City: ... | Місто: ... |
| ├ Auto-detect | ├ Автовизначення |
| ├ Moscow, London... | ├ Москва, Лондон... |
| └ Edit config... | └ Редагувати конфіг... |
| Temperature: °C/°F | Температура: °C/°F |
| Pressure: hPa/mmHg/inHg | Тиск: гПа/мм рт. ст./inHg |
| Icon theme: Auto/Light/Dark | Тема іконок: Авто/Світла/Темна |
| Language: English/Українська | Мова: English/Українська |
| About | Про програму |
| Quit | Вийти |

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
