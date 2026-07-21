# Nimbus

Weather tray app — shows current weather in system tray.

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).

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

## Usage

Just run the binary — it appears in the system tray.

- **Left click / hover**: see current weather
- **Refresh now**: force update
- **Units**: toggle Celsius/Fahrenheit
- **Quit**: exit

### Configuration

Auto-created at first run:
- **Windows**: `%APPDATA%\Nimbus\config.json`
- **Linux**: `~/.config/nimbus/config.json`

```json
{
  "latitude": 55.7558,
  "longitude": 37.6173,
  "update_interval": 10,
  "units": "celsius"
}
```

Change coordinates to your city. Interval in minutes.

## Weather API

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.

## License

MIT
