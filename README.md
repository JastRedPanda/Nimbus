# Nimbus v1.0.0

**Інформер погоди в системному треї** | **Weather tray app**

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).

Languages: English, Українська

## Download

Pre-built binaries: [Releases](https://github.com/JastRedPanda/Nimbus/releases)

## Build from source

### Requirements
- Go 1.21+

### Windows
```bash
go build -o nimbus.exe .
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

### Menu / Меню
| English | Українська |
|---|---|
| Refresh now | Оновити зараз |
| Units: °C/°F | Одиниці: °C/°F |
| Language: English / Ukrainian | Мова: English / Українська |
| About | Про програму |
| Quit | Вийти |

### Configuration

Auto-created at first run:
- **Windows**: `%APPDATA%\Nimbus\config.json`
- **Linux**: `~/.config/nimbus/config.json`

```json
{
  "latitude": 55.7558,
  "longitude": 37.6173,
  "update_interval": 10,
  "units": "celsius",
  "language": "en"
}
```

Change coordinates to your city. Interval in minutes.  
`language`: `"en"` or `"uk"` — UI language (switchable via tray menu).

## Weather API

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.

## License

GNU General Public License v3.0
