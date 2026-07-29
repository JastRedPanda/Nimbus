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
- Language: English / Українська
- **Forecast panel** - an ordinary window, closes only from its own close button or the tray icon, dragged by any part of its body, remembers where it was dropped / **Панель прогнозу** - звичайне вікно, закривається лише власною кнопкою закриття або кліком по іконці в треї, перетягується за будь-яке місце і запамʼятовує, куди її поставили
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
- Language (English / Українська)
- Font scale 1–100% for tray text / Масштаб шрифту в треї
- Update interval (5 min – 24 h) / Інтервал оновлення

#### Forecast panel / Панель прогнозу

The panel is an ordinary application window: the window manager draws its frame,
title bar and close button, so there is no close button inside the panel itself.
It is opaque, square-cornered, and coloured by the desktop theme. It stays above
other windows, stays off the taskbar and the pager, is dragged by any part of its
body, and toggles from the tray icon.

It closes in exactly two ways: the title bar's close button, or another click on
the tray icon. `Esc` does not close it and neither does losing focus - it stays
where it was put until it is dismissed on purpose.

Where it was put is remembered: the position is stored in `forecast_x` /
`forecast_y` and written only after an actual drag, so a panel that has never been
moved keeps opening at the work-area corner nearest the pointer.

The theme option (Auto / Dark / Light) governs every window Nimbus opens,
including this one, on all three toolkits. **Auto** is not a third colour: it
means Nimbus states no preference at all and the desktop paints its windows, the
way any native application behaves. **Dark** and **Light** ask the system for the
colours of that scheme - the desktop theme's dark variant on GTK, the
application colour scheme on Qt, and on Windows the system colours for light and
the dark surface Windows applications paint themselves for dark, since
`GetSysColor` answers light even when Windows is set to dark apps.

The option does NOT affect the tray icon, which is coloured by the temperature.

One limitation, on Qt only: switching the colour scheme per application arrived
in **Qt 6.8**, and released builds compile the Qt half against Qt 6.2 so that
they run on Ubuntu 22.04, Debian 12 and RHEL 9. Where the toolkit cannot honour
the option, the settings window does not offer it at all and the windows follow
the desktop. A locally built `make qt` on Qt 6.8 or newer offers it.

The browser fallback (used when the toolkit a build needs cannot be loaded) shows
the forecast as an ordinary page in a tab, not a window Nimbus draws, so none of
the above applies there; whatever `forecast_x` / `forecast_y` hold stays untouched.

A configuration file written by an older version of Nimbus still loads. It may
carry an `appearance` or a `forecast_pinned` key, from versions that offered a
Modern look and a pin checkbox; neither exists any more, `encoding/json` ignores
keys it does not know, and both simply disappear the next time the file is saved.

Панель - звичайне вікно застосунку: менеджер вікон малює її рамку, заголовок і
кнопку закриття, тому власної кнопки закриття всередині панелі немає. Вона
непрозора, з прямими кутами, і кольори бере з теми стільниці. Вона тримається
поверх інших вікон, не показується в панелі задач і в перемикачі робочих столів,
тягнеться за будь-яке місце свого тіла і перемикається кліком по іконці в треї.

Закрити її можна лише двома способами: кнопкою заголовка або ще одним кліком по
іконці в треї. `Esc` її не закриває, і втрата фокуса теж - вона лишається там,
куди її поставили, доки її не закриють навмисно.

Місце, куди її поставили, запамʼятовується: позиція зберігається в `forecast_x` /
`forecast_y` і записується лише після реального перетягування, тому панель, яку
жодного разу не переміщали, і далі зʼявляється біля найближчого до курсора кута
робочої області.

Опція теми (Авто / Темна / Світла) керує **всіма** вікнами Nimbus, включно з
панеллю, на всіх трьох тулкітах. **Авто** - це не третій колір: Nimbus не
висловлює жодних побажань і вікна малює стільниця, як і належить рідному
застосунку. **Темна** і **Світла** просять у системи кольори відповідної схеми -
темний варіант теми стільниці в GTK, кольорову схему застосунку в Qt, а в
Windows системні кольори для світлої та ту темну поверхню, яку застосунки Windows
малюють самі, бо `GetSysColor` відповідає світлим навіть коли Windows налаштована
малювати застосунки темними.

На піктограму в треї опція **не** впливає - її колір задає температура.

Одне обмеження, лише для Qt: перемикання кольорової схеми на рівні застосунку
зʼявилося в **Qt 6.8**, а релізні збірки компілюють Qt-частину проти Qt 6.2, щоб
вони працювали на Ubuntu 22.04, Debian 12 і RHEL 9. Там, де тулкіт не може
виконати опцію, вікно налаштувань її взагалі не показує, а вікна йдуть за
стільницею. Зібрана локально `make qt` на Qt 6.8 або новішій - показує.

Резервна веб-форма (використовується, коли потрібний тулкіт не вдалося завантажити)
показує прогноз як звичайну сторінку у вкладці, а не вікно, яке малює Nimbus, тому
нічого з написаного вище до неї не стосується; що б не було записано в
`forecast_x` / `forecast_y`, лишається незмінним.

Файл конфігурації, написаний старішою версією Nimbus, і далі завантажується. У
ньому можуть бути ключі `appearance` або `forecast_pinned` - з версій, які
пропонували вигляд Modern і галочку "закріпити"; жодного з них більше немає,
`encoding/json` ігнорує невідомі ключі, і обидва просто зникають при наступному
збереженні файлу.

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
| `icon_theme` | `auto` / `dark` / `light` | Theme for every window, the panel included; `auto` lets the desktop paint. Not the tray icon / Тема всіх вікон, разом із панеллю; `auto` віддає малювання стільниці. Піктограми в треї не стосується |
| `language` | `en` / `uk` | UI language / Мова інтерфейсу |
| `font_scale` | int 1–100 | Tray font scale % / Масштаб шрифту в треї (%) |
| `forecast_x`, `forecast_y` | int, optional | Remembered panel position; absent until it is dragged / Запамʼятована позиція панелі; відсутня, доки її не перетягнули |

## Weather API / API погоди

Uses [Open-Meteo](https://open-meteo.com/) — free, no API key required.  
IP geolocation via [ip-api.com](http://ip-api.com/).

## License / Ліцензія

GNU General Public License v3.0
