# Nimbus

![Nimbus](nimbus1.png)

**Weather tray app** | **Інформер погоди в системному треї**

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).  
Крос-платформа: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).  
Languages: English, Українська

## Features / Можливості

- Temperature icon in system tray / Піктограма температури в системному треї
- **7-day Forecast** — native window (Windows) or browser tab (Linux) / Прогноз на 7 днів
- **Settings GUI** — native window (Windows) or browser tab (Linux) / Вікно налаштувань
- Temperature unit: °C / °F
- Pressure unit: hPa / mmHg / inHg
- Wind unit: m/s / km/h
- Font scale: slider 1–100% for tray text / Масштаб шрифту в треї
- Update interval: 5 min – 24 hours
- Window theme: Auto / Dark / Light
- Language: English / Українська
- No console window (Windows) / Без консольного вікна (Windows)

## Download / Завантажити

### Windows
Pre-built binaries: [Releases](https://github.com/JastRedPanda/Nimbus/releases)  
Готові бінарники: [Releases](https://github.com/JastRedPanda/Nimbus/releases)

### Linux — distribution packages / Пакети для дистрибутивів

#### Debian / Ubuntu
```bash
sudo apt install ./nimbus_1.0.0-1_amd64.deb
```

#### RHEL / Rocky / Fedora
```bash
sudo dnf install nimbus-1.0.0-1.x86_64.rpm
```

#### openSUSE
```bash
sudo zypper install nimbus-1.0.0-1.x86_64.rpm
```

## Build from source / Збірка з вихідного коду

### Requirements / Вимоги
- Go 1.21+
- Linux: `libgtk-3-dev` / `gtk3-devel`, `libayatana-appindicator3-dev` / `libappindicator-gtk3-devel`

### Windows
```bash
go build -ldflags="-s -w -H windowsgui" -o nimbus.exe .
```

### Linux
```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o nimbus .
```

For release builds, inject version and date via ldflags:  
Для релізних збірок версія та дата передаються через ldflags:
```bash
-X github.com/JastRedPanda/Nimbus/internal/build.Version=<version>
-X github.com/JastRedPanda/Nimbus/internal/build.Date=$(date +%m.%Y)
```

### Build packages / Збірка пакетів

#### Debian / Ubuntu
```bash
make deb
```

#### RHEL / Rocky / Fedora / openSUSE
```bash
make rpm
```

### Settings / Налаштування

Available via **Menu → Settings...**:  
Доступно через **Меню → Налаштування...**

- **Windows**: native GUI window with all controls / рідне вікно з усіма елементами
- **Linux**: web form opened in default browser with local HTTP server / веб-форма в браузері з локальним HTTP-сервером

Fields / Поля:
- City name, latitude, longitude / Назва міста та координати
- Temperature unit (°C / °F) / Одиниця температури
- Pressure unit (hPa / mmHg / inHg) / Одиниця тиску
- Wind unit (m/s / km/h) / Одиниця вітру
- Window theme (Auto / Dark / Light) / Тема вікон
- Language (English / Українська)
- Font scale 1–100% for tray text / Масштаб шрифту в треї
- Update interval (5 min – 24 h) / Інтервал оновлення

### Configuration / Конфігурація

Auto-created at first run:  
Автоматично створюється при першому запуску:

- **Windows**: `%APPDATA%\Nimbus\config.json`
- **Linux**: `~/.config/Nimbus/config.json`

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

## Weather API / API погоди

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.  
IP geolocation via [ip-api.com](http://ip-api.com/).

## License / Ліцензія

GNU General Public License v3.0
