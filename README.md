# Nimbus

![Nimbus](nimbus1.png)

**Weather tray app** | **Інформер погоди в системному треї**

Cross-platform: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).  
Крос-платформа: Windows, Linux (Debian/Ubuntu, RHEL/Rocky/Fedora, openSUSE).  
Languages: English, Українська

## Features / Можливості

- Temperature icon in system tray / Піктограма температури в системному треї
- **7-day Forecast** — native window (Windows, Linux) / Прогноз на 7 днів
- **Settings GUI** — native window (Windows, Linux) / Вікно налаштувань
- Temperature unit: °C / °F
- Pressure unit: hPa / mmHg / inHg
- Wind unit: m/s / km/h
- Font scale: slider 1–100% for tray text / Масштаб шрифту в треї
- Update interval: 5 min – 24 hours
- Window theme: Auto / Dark / Light
- **Forecast panel look**: Modern / System look — translucent frameless sheet, or an ordinary window with a title bar / Вигляд панелі прогнозу: Modern / Системний вигляд
- Language: English / Українська
- **Pinned forecast panel** - closes only from the tray icon or the close button, and remembers where it was dragged / Панель прогнозу закривається лише кліком по іконці в треї або кнопкою закриття та запамʼятовує місце, куди її перетягнули
- No console window (Windows) / Без консольного вікна (Windows)

## Download / Завантажити

### Windows
Pre-built binaries: [Releases](https://github.com/JastRedPanda/Nimbus/releases)  
Готові бінарники: [Releases](https://github.com/JastRedPanda/Nimbus/releases)

### Linux — two builds / Дві збірки

Pick the one that matches your desktop. **nimbus-gtk** draws its windows with
GTK 3 and contains no Qt; **nimbus-qt** draws them with Qt 6 and contains no GTK.
Everything else about them is identical, and they can be installed side by side -
different binaries, different menu entries, one shared configuration file.

Оберіть ту, що відповідає вашій стільниці. **nimbus-gtk** малює вікна через GTK 3
і не містить Qt; **nimbus-qt** малює їх через Qt 6 і не містить GTK. У всьому
іншому вони однакові, і їх можна встановити поруч - різні бінарники, різні пункти
меню, один спільний файл конфігурації.

#### Debian / Ubuntu
```bash
sudo apt install ./nimbus-gtk_1.0.0-1_amd64.deb    # GTK
sudo apt install ./nimbus-qt_1.0.0-1_amd64.deb     # Qt
```

#### RHEL / Rocky / Fedora
```bash
sudo dnf install nimbus-gtk-1.0.0-1.x86_64.rpm
sudo dnf install nimbus-qt-1.0.0-1.x86_64.rpm
```

#### openSUSE
```bash
sudo zypper install nimbus-gtk-1.0.0-1.x86_64.rpm
sudo zypper install nimbus-qt-1.0.0-1.x86_64.rpm
```

The GTK package replaces the older `nimbus` package on upgrade. The Qt package
does not: it is a new thing that installs alongside. / Пакет GTK при оновленні
заміщає старий пакет `nimbus`; пакет Qt - ні, він встановлюється поруч.

Released binaries are built for the oldest systems that can run them: glibc 2.34
and Qt 6.2, which covers Ubuntu 22.04, Debian 12, RHEL 9 and anything newer. The
release workflow measures that and fails if it ever slips. / Бінарники в релізах
збираються під найстаріші системи, на яких вони працюють: glibc 2.34 і Qt 6.2 -
це Ubuntu 22.04, Debian 12, RHEL 9 і новіші. Workflow перевіряє це на кожній
збірці.

_Commands above are examples — the actual package version may differ._  
_Команди вище — приклади; актуальна версія пакета може відрізнятися._

## Build from source / Збірка з вихідного коду

### Requirements / Вимоги
- Go, the version in `go.mod`
- Nothing else for `nimbus-gtk` or `nimbus.exe`: GTK is loaded at runtime with
  `dlopen`, so no headers and no pkg-config are involved
- `nimbus-qt` additionally needs the Qt 6 development package **at build time
  only**: `qt6-base-dev` (Debian/Ubuntu), `qt6-base` (Arch), `qt6-qtbase-devel`
  (Fedora/RHEL)

### Windows
```bash
go build -ldflags="-s -w -H windowsgui" -o nimbus.exe .
```

### Linux
```bash
make gtk        # -> build/nimbus-gtk
make qt         # -> build/nimbus-qt  (needs the Qt 6 dev package)
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
make deb-gtk
make deb-qt
```

#### RHEL / Rocky / Fedora / openSUSE
```bash
make rpm-gtk
make rpm-qt
```

### How the Qt build works / Як влаштована збірка Qt

Qt is C++ and exports no C ABI, so the Qt windows are written in C++ in
`qtshim/` and reached through a few dozen plain C functions. That directory is a
nested Go module, like `winres/`, and `make shim` compiles it with
`go build -buildmode=c-shared` - one `go build`, no CMake, no second build
system. The resulting shared object is **embedded in the binary** and loaded from
memory at startup, so `nimbus-qt` is still a single file with nothing to install
beside it, still has four dynamic dependencies, and still needs no versioned
glibc symbol.

Qt is located with `qmake6 -query` rather than pkg-config: Debian and Ubuntu ship
no `.pc` files for Qt 6 at all, so `pkg-config` works on some distributions and
not on others. `make shim` handles that; running `go build` inside `qtshim/` by
hand does not.

Which backend a binary uses is not a guess - it is what the binary contains.
`NIMBUS_GUI_BACKEND` still exists for bug reports and forces a named backend.

Qt - це C++ без C ABI, тому вікна Qt написані на C++ у `qtshim/` і доступні через
кілька десятків звичайних C-функцій. Це вкладений Go-модуль, як `winres/`, і
`make shim` збирає його через `go build -buildmode=c-shared` - однією командою
`go build`, без CMake і другої системи збірки. Отриманий `.so` **вбудовано в
бінарник** і завантажується з памʼяті, тому `nimbus-qt` лишається одним файлом.

### Settings / Налаштування

Available via **Menu → Settings...**:  
Доступно через **Меню → Налаштування...**

- **Windows**: native GUI window with all controls / рідне вікно з усіма елементами
- **Linux**: native GTK window (`nimbus-gtk`) or native Qt window (`nimbus-qt`); a web form in the browser only when the toolkit that build needs cannot be loaded / рідне вікно GTK (`nimbus-gtk`) або Qt (`nimbus-qt`); веб-форма в браузері - лише коли потрібний тулкіт не вдалося завантажити

Fields / Поля:
- City name, latitude, longitude / Назва міста та координати
- Temperature unit (°C / °F) / Одиниця температури
- Pressure unit (hPa / mmHg / inHg) / Одиниця тиску
- Wind unit (m/s / km/h) / Одиниця вітру
- Window theme (Auto / Dark / Light) / Тема вікон
- Appearance of the forecast panel (Modern / System look) / Вигляд панелі прогнозу (Modern / Системний вигляд)
- Language (English / Українська)
- Font scale 1–100% for tray text / Масштаб шрифту в треї
- Update interval (5 min – 24 h) / Інтервал оновлення
- Close the forecast panel only from the tray icon / Закривати панель прогнозу лише кліком по іконці

#### Forecast panel appearance / Вигляд панелі прогнозу

**System look is the default:** the panel is an ordinary application window -
opaque, square corners, the window manager's title bar with its own close button,
so there is no close button inside the panel. The colours come **from the desktop
theme**, which is why the theme option (Auto / Dark / Light) has no say over the
panel in this look; it still decides the tray icon and the other windows.

**Modern** is the other choice: a translucent sheet with rounded corners, no title
bar, and its own close button in the top corner, coloured from the app's palette,
which the theme option picks. It is what the panel looked like before the option
existed - so a configuration file written earlier, which has no appearance key at
all, now opens in the system look. That is a visible change after an upgrade, not
a fault.

Nothing else changes. In both looks the panel stays above other windows, stays off
the taskbar and the pager, is visible on every workspace, is dragged by any part of
its body, remembers where it was put, toggles from the tray icon, obeys the pinned
option below, and shows exactly the same table.

**Системний вигляд - типове значення:** панель є звичайним вікном застосунку -
непрозорим, з прямими кутами, із заголовком і кнопкою закриття від менеджера вікон,
тому власної кнопки закриття всередині панелі немає. Кольори беруться **з теми
стільниці**, тому опція теми (Авто / Темна / Світла) в цьому вигляді на панель не
впливає; вона й далі керує піктограмою в треї та іншими вікнами.

**Modern** - друга можливість: напівпрозоре полотно із заокругленими кутами, без
заголовка, з власною кнопкою закриття в куті та кольорами з палітри застосунку, яку
вибирає опція теми. Саме так панель виглядала до появи цієї опції - тому файл
конфігурації, написаний раніше і зовсім без ключа appearance, тепер відкривається в
системному вигляді. Це помітна зміна після оновлення, а не помилка.

Решта - як і було. В обох виглядах панель тримається поверх інших вікон, не
показується в панелі задач і в перемикачі робочих столів, видима на всіх робочих
столах, тягнеться за будь-яке місце, запамʼятовує, куди її поставили,
перемикається кліком по іконці в треї, підкоряється опції нижче та показує ту саму
таблицю.

#### Forecast panel behaviour / Поведінка панелі прогнозу

**Enabled by default.** The option governs one thing: what may dismiss the panel.
With it on, the panel ignores `Esc` and keeps standing when it loses focus, so it
closes only from the tray icon or its own close button. Turn it off and `Esc` or a
click elsewhere dismisses it again, as before.

The panel is draggable by any part of its body, and where you put it is remembered
either way - the option has no say in that. The position is stored in `forecast_x`
/ `forecast_y` and written only after an actual drag, so a panel you have never
moved keeps appearing at the corner nearest the pointer.

**Увімкнено за замовчуванням.** Опція керує лише одним: чим можна закрити панель.
Коли вона увімкнена, панель ігнорує `Esc` і не зникає, втративши фокус, - закрити
її можна лише кліком по іконці в треї або кнопкою закриття. Вимкніть її, і `Esc`
або клік поза панеллю знову її закриють, як раніше.

Панель можна тягнути за будь-яке місце, і те, куди ви її поставили,
запамʼятовується незалежно від опції. Позиція зберігається в `forecast_x` /
`forecast_y` і записується лише після реального перетягування, тому панель, яку
жодного разу не переміщали, і далі зʼявляється біля найближчого до курсора кута.

The browser fallback (used when the toolkit a build needs cannot be loaded) has no checkbox for
this option and no draggable panel, so there `forecast_pinned` and the remembered
position keep whatever the config file holds and can only be changed by editing
that file. `appearance` is meaningless there for the same reason - the forecast is
a browser tab, not a window Nimbus draws - and is likewise left as stored. / Резервна
веб-форма (використовується, коли потрібний тулкіт не вдалося завантажити) не має ні цієї галочки,
ні панелі, яку можна перетягувати, тому там `forecast_pinned` зберігає значення з
файлу конфігурації та змінюється лише редагуванням цього файлу. `appearance` там не
має сенсу з тієї ж причини - прогноз відкривається як вкладка браузера, а не як
вікно, яке малює Nimbus - і теж зберігає значення з файлу.

_After an upgrade the panel stops closing on `Esc` and on focus loss - this is the
new default, not a bug. / Після оновлення панель більше не закривається по `Esc`
та при втраті фокуса - це нове типове значення, а не помилка._

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
  "font_scale": 100,
  "appearance": "system",
  "forecast_pinned": true
}
```

`forecast_x` and `forecast_y` are absent until the panel is dragged for the first
time; while they are absent the panel opens next to the pointer.  
`forecast_x` та `forecast_y` відсутні, доки панель не перетягнули вперше; поки їх
немає, панель відкривається біля курсора.

| Field | Values | Description |
|---|---|---|
| `latitude`, `longitude` | float | Coordinates / Координати |
| `city_name` | string | Display name / Назва міста |
| `update_interval` | int (minutes) | Refresh interval / Інтервал оновлення (хв) |
| `units` | `celsius` / `fahrenheit` | Temperature unit / Одиниця температури |
| `pressure_unit` | `hpa` / `mmhg` / `inhg` | Pressure unit / Одиниця тиску |
| `wind_unit` | `ms` / `kmh` | Wind unit / Одиниця вітру |
| `icon_theme` | `auto` / `dark` / `light` | Window theme; does not apply to the forecast panel in the system look / Тема вікон; у системному вигляді на панель прогнозу не впливає |
| `appearance` | `modern` / `system` (default `system`) | Forecast panel look: an ordinary window coloured by the desktop theme, or a translucent frameless sheet. Anything else is replaced by the default when the file is read / Вигляд панелі прогнозу: звичайне вікно з кольорами теми стільниці або напівпрозоре полотно без рамки. Будь-яке інше значення замінюється типовим під час читання файлу |
| `language` | `en` / `uk` | UI language / Мова інтерфейсу |
| `font_scale` | int 1–100 | Tray font scale % / Масштаб шрифту в треї (%) |
| `forecast_pinned` | bool (default `true`) | Forecast panel closes only from the tray icon or the close button / Панель прогнозу закривається лише кліком по іконці в треї або кнопкою закриття |
| `forecast_x`, `forecast_y` | int, optional | Remembered panel position; absent until it is dragged / Запамʼятована позиція панелі; відсутня, доки її не перетягнули |

## Weather API / API погоди

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.  
IP geolocation via [ip-api.com](http://ip-api.com/).

## License / Ліцензія

GNU General Public License v3.0
