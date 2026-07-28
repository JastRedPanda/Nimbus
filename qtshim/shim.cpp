// The Qt half of the Qt backend.
//
// This is not a binding and not a translation layer. It IS the user interface,
// the same way internal/ui/*_linux.go is the GTK one and internal/ui/*_windows.go
// is the Win32 one - it just happens to be the only one of the three that cannot
// be written in Go, because Qt exposes no C ABI to write it against.
//
// Everything Qt-shaped lives here: widgets, layouts, the event loop, window
// flags, the palette. The Go side sees the C functions in shim.h, taking ints and
// strings.
//
// THREAD RULE, and it is the same rule the other two backends live under: Qt
// owns the thread that called nimbus_qt_init, and every widget call has to happen
// on it. The Go side guarantees this by locking that goroutine to its OS thread
// before init and never calling in from anywhere else - see internal/qt.
//
// NO Q_OBJECT ANYWHERE IN THIS FILE, and that is a build decision rather than a
// style one. The macro is what needs moc, Qt's code generator, and needing moc
// would mean a second build step that `go build` cannot perform - which is the
// one thing this whole arrangement exists to avoid. Everything here is either a
// virtual override, which needs no metaobject, or a connect() to a lambda on a
// stock widget that already has its own. Adding a signal or a slot to a class in
// this file would break the build with an error message that says nothing about
// moc, so: do not.
#include "shim.h"

#include <QApplication>
#include <QByteArray>
#include <QCheckBox>
#include <QCloseEvent>
#include <QComboBox>
#include <QCursor>
#include <QDialog>
#include <QFontDatabase>
#include <QGridLayout>
#include <QGroupBox>
#include <QGuiApplication>
#include <QIcon>
#include <QHBoxLayout>
#include <QKeyEvent>
#include <QLabel>
#include <QLineEdit>
#include <QListWidget>
#include <QMessageBox>
#include <QMouseEvent>
#include <QPainter>
#include <QPainterPath>
#include <QPixmap>
#include <QPointer>
#include <QPushButton>
#include <QRadioButton>
#include <QScreen>
#include <QScrollArea>
#include <QSlider>
#include <QString>
#include <QStringList>
#include <QTimer>
#include <QVBoxLayout>
#include <QVector>
#include <QWindow>

namespace {

QApplication *app = nullptr;

// The single error dialog. A QPointer rather than a raw one so that a dialog the
// user closed - which deletes it, because of WA_DeleteOnClose - reads back as
// null here instead of dangling. That is the whole reason this is a QPointer and
// not a QDialog*.
QPointer<QMessageBox> errorBox;

// Same for About: reopening while it is up should raise the existing window
// rather than stack a second one, which is what the GTK backend does.
QPointer<QDialog> aboutBox;

// The family name the weather face registered itself under. Empty until
// nimbus_qt_font has been called, and still empty if Qt refused the data - which
// leaves the symbol column blank, the same answer the GTK panel gives for a
// codepoint the face does not carry. An empty cell says less than a wrong symbol
// does.
QString symbolFamily;

QString utf8(const char *s) { return s ? QString::fromUtf8(s) : QString(); }

// ---------------------------------------------------------------------------
// The forecast panel.
//
// Every metric below is the one in internal/ui/style_linux.go and in the named
// constants of forecast_windows.go. The point of three files that agree to the
// pixel is that the three platforms look like one product.
// ---------------------------------------------------------------------------

const int forecastWidth = 620;
const int symbolPt = 20;
const int rowGapY = 6;
const int colGapX = 18;
const int pageGapY = 2;
const int pagePad = 14;
const int pagePadTop = 4;
const int theadPt = 11;
const int cellPt = 11;
const int sheetRadius = 14;
const int panelMargin = 12;

// How the settler for a suppressed dismissal is paced - see Panel::settle. The
// GTK panel's dragPoll and dragSettleMax in milliseconds, and for the reasons
// spelled out there: fast enough that the panel closes as the user lets go, and
// backstopped far past any real drag, because the only thing it guards against is
// a pointer that reports a button as held forever.
const int dragPollMs = 100;
const int dragSettleMaxMs = 20000;

// placeSettleMs is how long the system look waits before taking the window's
// position as the baseline a later title-bar drag is measured against. Long
// enough for a window manager to finish placing a window it has just been given,
// short enough that a user cannot drag the panel before it elapses. The GTK
// panel's placeSettle, and it is here for the same reason - see the call site.
const int placeSettleMs = 400;

// The Modern palette, straight out of style_linux.go. The two sheet alphas are
// its 0.96 and 0.98 rounded to eight bits.
struct Palette {
    QColor sheet, ink, head, hair, rule;
    QString hover;
};

Palette paletteFor(bool dark) {
    Palette p;
    if (dark) {
        p.sheet = QColor(0x1c, 0x1f, 0x26, 245);
        p.ink = QColor(0xf2, 0xf4, 0xf7);
        p.head = QColor(0x9a, 0xa3, 0xb0);
        p.hair = QColor(255, 255, 255, 26); // rgba(255,255,255,0.10)
        p.rule = QColor(255, 255, 255, 71); // rgba(255,255,255,0.28)
        p.hover = QStringLiteral("rgba(255,255,255,0.10)");
    } else {
        p.sheet = QColor(0xff, 0xff, 0xff, 250);
        p.ink = QColor(0x14, 0x16, 0x1a);
        p.head = QColor(0x5b, 0x64, 0x72);
        p.hair = QColor(0, 0, 0, 26); // rgba(0,0,0,0.10)
        p.rule = QColor(0, 0, 0, 61);  // rgba(0,0,0,0.24)
        p.hover = QStringLiteral("rgba(0,0,0,0.08)");
    }
    return p;
}

// resolveDark answers the "auto" theme the way the GTK panel does: by asking the
// desktop theme which way round it draws text. A light foreground means a dark
// theme. Reading the theme's own ink is more reliable than any prefer-dark flag,
// which stays false under themes that are simply dark.
bool resolveDark(int dark) {
    if (dark >= 0) {
        return dark != 0;
    }
    return QApplication::palette().color(QPalette::WindowText).lightness() > 127;
}

Qt::Alignment alignFor(int align) {
    switch (align) {
    case NIMBUS_QT_ALIGN_CENTER:
        return Qt::AlignHCenter | Qt::AlignVCenter;
    case NIMBUS_QT_ALIGN_END:
        return Qt::AlignRight | Qt::AlignVCenter;
    default:
        return Qt::AlignLeft | Qt::AlignVCenter;
    }
}

struct Column {
    QString caption;
    int align = NIMBUS_QT_ALIGN_START;
};

struct Day {
    QString date, symbol, temp, wind, precip;
};

// What the builder calls have accumulated so far. Discarded by the next begin, so
// a panel that was built and never shown cannot leak into the following one.
struct PanelSpec {
    QString title;
    bool system = false;
    int dark = -1;
    QVector<Column> cols;
    QVector<Day> days;
} spec;

class Panel : public QWidget {
public:
    Panel(const PanelSpec &s, unsigned long long id,
          int (*pinned)(unsigned long long),
          void (*event)(unsigned long long, long long, long long, long long));

    // dismiss is the panel's single exit: the x button, Escape, focus loss, the
    // title bar's close button and the tray toggle all leave through here, which
    // is the only reason the position gets reported at all.
    //
    // byFocus is passed on because only a focus-loss close may arm the tray's
    // toggle grace. Arming it after a deliberate close is what once ate twelve
    // consecutive clicks from a user tapping the tray icon.
    void dismiss(bool byFocus);

    // place is called between construction and show, when the layout has run but
    // the window is not yet on screen.
    void place(bool haveAt, int x, int y);

protected:
    void paintEvent(QPaintEvent *) override;
    void resizeEvent(QResizeEvent *) override;
    void mousePressEvent(QMouseEvent *) override;
    void keyPressEvent(QKeyEvent *) override;
    void changeEvent(QEvent *) override;
    void closeEvent(QCloseEvent *) override;

private:
    QWidget *hairline(int h, const QColor &c);
    QLabel *cell(const QString &text, int align, const QColor &c, int pt, bool bold);
    void buildTable(const PanelSpec &s);
    bool pinned() const { return pinned_ && pinned_(id_) != 0; }
    bool inMove();
    void armSettle();
    void settle();

    unsigned long long id_ = 0;
    int (*pinned_)(unsigned long long) = nullptr;
    void (*event_)(unsigned long long, long long, long long, long long) = nullptr;

    bool system_ = false;
    Palette pal_;

    bool closed_ = false;
    // handedOff_ and origin_ are the two halves of what licenses reporting a
    // position: a press was handed to the window manager's move loop at least once
    // during this showing, AND the window is no longer where it was at that first
    // handoff. Neither half is enough - a bare click is a handoff too, and without
    // the handoff the first open of a panel nobody moved would write down a corner
    // the user never chose.
    bool handedOff_ = false;
    QPoint origin_;
    // armed_ delays the focus-loss rule until the panel has actually held focus
    // once: an undecorated window is not guaranteed focus when it is mapped, and
    // without this the first deactivation closes the panel before the user has
    // seen it.
    bool armed_ = false;
    // dragging_ says a press was handed to the move loop and the button has not
    // been seen up since. deferred_ is a focus loss that arrived during a move and
    // has not been settled; settling_ keeps a burst of focus events to one settler.
    bool dragging_ = false;
    bool deferred_ = false;
    bool settling_ = false;
    int waited_ = 0;
};

Panel *panel = nullptr;

Panel::Panel(const PanelSpec &s, unsigned long long id,
             int (*pinned)(unsigned long long),
             void (*event)(unsigned long long, long long, long long, long long))
    : id_(id), pinned_(pinned), event_(event), system_(s.system) {
    pal_ = paletteFor(resolveDark(s.dark));

    setAttribute(Qt::WA_DeleteOnClose);
    setWindowTitle(s.title);

    // Qt::Tool is what keeps the panel off the taskbar and the pager: it asks for
    // _NET_WM_WINDOW_TYPE_UTILITY, which a window manager reads as "not an
    // application window". The system look keeps the frame the manager draws for
    // it; Modern asks for none.
    //
    // Not sticky across workspaces, which is the one panel hint the GTK backend
    // sets and this cannot: Qt exposes no equivalent of gtk_window_stick, and
    // reaching around it would mean talking to X11 directly and giving up Wayland.
    Qt::WindowFlags flags = Qt::Tool | Qt::WindowStaysOnTopHint;
    if (!system_) {
        flags |= Qt::FramelessWindowHint;
        setAttribute(Qt::WA_TranslucentBackground);
    }
    setWindowFlags(flags);

    QVBoxLayout *page = new QVBoxLayout(this);
    page->setContentsMargins(pagePad, pagePadTop, pagePad, pagePad);
    page->setSpacing(pageGapY);

    // The close row is Modern's stand-in for a title bar and belongs to it alone.
    // The system look has the real thing, and two close buttons in one window is
    // one too many.
    if (!system_) {
        // U+00D7 MULTIPLICATION SIGN. Bold as well as larger: it has thin strokes
        // and little ink for its em, so size alone barely reads. Kept over a
        // heavier codepoint like U+2715 because every font ships it.
        QPushButton *x = new QPushButton(QString::fromUtf8("\xc3\x97"), this);
        x->setStyleSheet(QStringLiteral("QPushButton{border:none;background:transparent;"
                                        "font-size:15pt;font-weight:bold;padding:0 9px;"
                                        "border-radius:8px;color:%1;}"
                                        "QPushButton:hover{background:%2;color:%3;}")
                             .arg(pal_.head.name(), pal_.hover, pal_.ink.name()));
        x->setFocusPolicy(Qt::NoFocus);
        QObject::connect(x, &QPushButton::clicked, this, [this]() { dismiss(false); });

        QHBoxLayout *row = new QHBoxLayout();
        row->setContentsMargins(0, 0, 0, 0);
        row->addStretch(1);
        row->addWidget(x);
        page->addLayout(row);
    }

    buildTable(s);
    setMinimumWidth(forecastWidth);

    // The system look needs a second way to establish an origin, because the
    // obvious gesture in it - dragging the window by its title bar - never reaches
    // the toolkit at all. The window manager owns the caption and moves the window
    // itself, so no mouse press is delivered, handedOff_ stays false, and a panel
    // the user dragged across the screen by its title bar would report nothing.
    //
    // So in that look the window's settled placement is taken as the origin once,
    // shortly after it is shown, and everything after that is a move. The delay is
    // what makes it a SETTLED position rather than a transient one: the manager may
    // adjust a freshly mapped window, and a baseline read mid-adjustment would make
    // the adjustment itself look like something the user did.
    //
    // Modern needs none of this - it has no frame for a manager to move it by, so
    // the press is always ours to see. The Win32 backend has the same problem and
    // solves it in the same spirit, by capturing the origin in WM_ENTERSIZEMOVE.
    if (system_) {
        QTimer::singleShot(placeSettleMs, this, [this]() {
            if (closed_ || handedOff_) {
                return;
            }
            handedOff_ = true;
            origin_ = pos();
        });
    }
}

// hairline is one row separator: a plain widget with a background rather than a
// QFrame, because a QFrame's line is drawn by the style and the Modern look
// states its own colours precisely so that the desktop theme does not.
QWidget *Panel::hairline(int h, const QColor &c) {
    QWidget *w = new QWidget(this);
    w->setFixedHeight(h);
    QPalette p = w->palette();
    p.setColor(QPalette::Window, c);
    w->setPalette(p);
    w->setAutoFillBackground(true);
    return w;
}

QLabel *Panel::cell(const QString &text, int align, const QColor &c, int pt, bool bold) {
    QLabel *l = new QLabel(text, this);
    l->setAlignment(alignFor(align));
    QFont f = l->font();
    f.setPointSize(pt);
    f.setBold(bold);
    l->setFont(f);
    if (c.isValid()) {
        QPalette p = l->palette();
        p.setColor(QPalette::WindowText, c);
        l->setPalette(p);
    }
    return l;
}

// buildTable lays the columns out: captions, a rule, then one row per day with a
// hairline between neighbours.
//
// The system look passes an invalid colour for the text, which leaves every label
// to the desktop theme - that is what "colours from the desktop theme" is
// implemented as. Its separators take the theme's own Mid, and it keeps ONE thing
// of its own: the rule's extra pixel. With the theme painting both weights, the
// header rule and the row hairlines otherwise collapse into a single
// indistinguishable line. Thickness is ours, colour is theirs, which is the same
// split style_linux.go makes.
void Panel::buildTable(const PanelSpec &s) {
    const QColor ink = system_ ? QColor() : pal_.ink;
    const QColor head = system_ ? QColor() : pal_.head;
    const QColor hair = system_ ? palette().color(QPalette::Mid) : pal_.hair;
    const QColor rule = system_ ? palette().color(QPalette::Mid) : pal_.rule;
    const int ruleH = system_ ? 2 : 1;

    QGridLayout *grid = new QGridLayout();
    grid->setContentsMargins(0, 0, 0, 0);
    grid->setVerticalSpacing(rowGapY);
    grid->setHorizontalSpacing(colGapX);

    const int cols = s.cols.size();
    const int span = cols > 0 ? cols : 1;
    for (int i = 0; i < cols; ++i) {
        grid->addWidget(cell(s.cols[i].caption, s.cols[i].align, head, theadPt, true), 0, i);
    }
    grid->addWidget(hairline(ruleH, rule), 1, 0, 1, span);

    int row = 2;
    for (int i = 0; i < s.days.size(); ++i) {
        if (i > 0) {
            grid->addWidget(hairline(1, hair), row, 0, 1, span);
            ++row;
        }
        const Day &d = s.days[i];
        const QString values[5] = {d.date, QString(), d.temp, d.wind, d.precip};
        for (int c = 0; c < cols && c < 5; ++c) {
            if (c == 1) {
                // The weather symbol, drawn as text in the registered face rather
                // than as a bitmap: Qt renders the glyph at any size and in any
                // colour from the same bytes, which the rasterise-and-hand-over
                // route the GTK backend has to take cannot do.
                if (symbolFamily.isEmpty() || d.symbol.isEmpty()) {
                    continue;
                }
                QLabel *sym = cell(d.symbol, s.cols[c].align, ink, symbolPt, false);
                QFont f(symbolFamily);
                f.setPointSize(symbolPt);
                sym->setFont(f);
                grid->addWidget(sym, row, c);
                continue;
            }
            grid->addWidget(cell(values[c], s.cols[c].align, ink, cellPt, false), row, c);
        }
        ++row;
    }

    // A phantom column absorbing the slack, so the real ones keep their natural
    // widths and the table hugs the leading edge instead of being stretched across
    // whatever width the panel ended up with. That is what the GTK grid does by
    // being packed without expanding.
    grid->setColumnStretch(cols, 1);

    static_cast<QVBoxLayout *>(layout())->addLayout(grid);
}

void Panel::paintEvent(QPaintEvent *) {
    if (system_) {
        // Nothing of our own: an ordinary application window, painted by the style
        // and coloured by the desktop theme.
        return;
    }
    QPainter p(this);
    p.setRenderHint(QPainter::Antialiasing, true);
    p.setPen(Qt::NoPen);
    p.setBrush(pal_.sheet);
    p.drawRoundedRect(QRectF(rect()), sheetRadius, sheetRadius);
}

// resizeEvent keeps the rounded corners honest on a desktop with no compositor.
//
// Qt 6 offers no public way to ask whether one is running - QX11Info went away,
// and QNativeInterface would mean talking to X11 directly, which would cost
// Wayland - so the panel is drawn to look right either way. The alpha in the fill
// is honoured where there is a compositor and ignored where there is not; the
// MASK is what deals with the corners, which would otherwise be four black
// notches wherever the alpha is ignored, because an ARGB window with nothing
// compositing it shows its unpainted pixels as black.
//
// The cost is a one-pixel staircase on four 14-pixel corners where a compositor
// would have antialiased them. That is the right way round: barely visible
// everywhere beats glaring on one real configuration.
void Panel::resizeEvent(QResizeEvent *) {
    if (system_) {
        return;
    }
    QPainterPath path;
    path.addRoundedRect(QRectF(rect()), sheetRadius, sheetRadius);
    setMask(QRegion(path.toFillPolygon().toPolygon()));
}

// A press anywhere on the body starts a window-manager move, which is what makes
// the panel draggable by any part of itself. The close button never gets here: it
// is a child widget and consumes its own press.
//
// Only the FIRST handoff records an origin. A later press happens wherever the
// user has already dragged the panel to, so keeping the newest origin would make
// a drag followed by one idle click report nothing at all.
void Panel::mousePressEvent(QMouseEvent *e) {
    if (e->button() != Qt::LeftButton) {
        QWidget::mousePressEvent(e);
        return;
    }
    if (!handedOff_) {
        handedOff_ = true;
        origin_ = pos();
    }
    dragging_ = true;
    // Watch for the button coming up. Nothing else can tell the latch the move is
    // over: the release goes to whoever holds the pointer grab for the move, which
    // is the window manager and never this window.
    armSettle();
    if (QWindow *w = windowHandle()) {
        w->startSystemMove();
    }
}

void Panel::keyPressEvent(QKeyEvent *e) {
    if (e->key() != Qt::Key_Escape) {
        QWidget::keyPressEvent(e);
        return;
    }
    if (pinned()) {
        return;
    }
    if (inMove()) {
        // Dropped, not deferred, which is the opposite of what happens to a focus
        // loss and is deliberate: during a move Escape is the window manager's own
        // cancel-the-drag key, so one that arrives here was aimed at the drag
        // rather than at the panel. Nothing is lost - unlike a deactivation,
        // Escape is not delivered on a transition, so a user who did mean "close"
        // presses it again once the panel has stopped moving.
        return;
    }
    dismiss(false);
}

// changeEvent is where focus loss arrives: Qt reports activation as a state
// change on the window rather than as an event of its own.
void Panel::changeEvent(QEvent *e) {
    if (e->type() != QEvent::ActivationChange) {
        QWidget::changeEvent(e);
        return;
    }
    if (isActiveWindow()) {
        armed_ = true;
        return;
    }
    if (!armed_ || pinned()) {
        return;
    }
    if (inMove()) {
        // Suppressed rather than dropped, and settled when the button comes up -
        // see settle for why a focus loss cannot simply be discarded.
        deferred_ = true;
        armSettle();
        return;
    }
    dismiss(true);
}

void Panel::closeEvent(QCloseEvent *e) {
    // The system look's title-bar button, a session shutdown or a wmctrl -c all
    // land here. Routing them through dismiss is what makes them report the
    // panel's position like every other exit; without it a panel the user dragged
    // and then closed by its title bar would forget where it had been put.
    dismiss(false);
    e->accept();
}

// inMove reports whether the window manager is moving the panel right now, and it
// recomputes the answer from the pointer every time rather than trusting a latch.
//
// That is what makes an abnormally ended drag harmless. Immunity to dismissal
// lasts exactly as long as the button is physically down: a manager that dies
// mid-move, a drag the manager cancels, a window that never gets a configure -
// none of them can hold a button down. The latch alone would suppress dismissals
// for the rest of the panel's life, since nothing announces the end of a move;
// the button alone would suppress them whenever the user happens to be holding a
// button somewhere else entirely.
bool Panel::inMove() {
    if (!dragging_) {
        return false;
    }
    if (!(QGuiApplication::mouseButtons() & Qt::LeftButton)) {
        dragging_ = false;
        return false;
    }
    return true;
}

void Panel::armSettle() {
    if (settling_) {
        return;
    }
    settling_ = true;
    waited_ = 0;
    QTimer::singleShot(dragPollMs, this, [this]() { settle(); });
}

// settle disposes of a focus loss that was suppressed during a move, and is the
// only thing here that polls.
//
// THE CHOICE, stated once: a suppressed focus loss is HONOURED when the move
// ends, unless the panel has the focus back by then or has been pinned in the
// meantime - the state is re-read now rather than the deferred decision trusted.
// Dropping it outright would be wrong because deactivation is delivered on the
// transition, so a panel that is already inactive is never told again, and an
// unpinned one would sit above whatever the user switched to. Honouring it
// unconditionally would be equally wrong: the ordinary end of a drag hands the
// focus straight back, and closing then would make the panel vanish the instant
// the user let go of it.
//
// The timer is bound to this widget, so a panel that something else closed in the
// meantime simply never gets the callback.
void Panel::settle() {
    if (closed_) {
        settling_ = deferred_ = false;
        return;
    }
    if (inMove()) {
        waited_ += dragPollMs;
        if (waited_ < dragSettleMaxMs) {
            QTimer::singleShot(dragPollMs, this, [this]() { settle(); });
            return;
        }
        // The only way to be here is a button Qt keeps reporting as down long past
        // any real drag. Acting anyway is the lesser evil: the panel is otherwise
        // immune to focus loss for as long as the lie lasts.
        dragging_ = false;
    }
    settling_ = false;
    if (!deferred_) {
        return;
    }
    deferred_ = false;
    if (pinned() || isActiveWindow()) {
        return;
    }
    dismiss(true);
}

void Panel::dismiss(bool byFocus) {
    if (closed_) {
        return;
    }
    closed_ = true;
    if (panel == this) {
        // Cleared here rather than in a destructor: WA_DeleteOnClose deletes the
        // widget through deleteLater, so between this call and the deletion the
        // event loop can perfectly well deliver another tray click, and a panel on
        // its way out must not answer it.
        panel = nullptr;
    }
    if (event_) {
        // Read while the window still exists, and compared against the origin
        // rather than against the placement that was asked for. Both reads go
        // through the same call, which is what keeps this correct on Wayland: a
        // client there cannot know its own position and Qt answers 0,0 for every
        // window, so the two reads are equal and nothing is reported - the right
        // answer, since 0,0 is not a position the user chose.
        if (handedOff_) {
            const QPoint p = pos();
            if (p != origin_) {
                event_(id_, NIMBUS_QT_EV_MOVED, p.x(), p.y());
            }
        }
        event_(id_, NIMBUS_QT_EV_CLOSED, byFocus ? 1 : 0, 0);
    }
    close();
}

// place puts the panel where the user last dragged it, or at the work-area corner
// nearest the pointer.
//
// The order matters: a content-hugging layout has no size until the layout has
// run, so both placements come after adjustSize. The remembered position is
// honoured only if some screen still contains it - a display that was unplugged
// or a rearranged desktop can leave a position that was legitimate when it was
// written with the whole panel off screen, and a pinned panel whose close button
// is off screen cannot be closed from the window at all.
void Panel::place(bool haveAt, int x, int y) {
    adjustSize();
    const QSize sz = size();

    if (haveAt) {
        if (QScreen *s = QGuiApplication::screenAt(QPoint(x, y))) {
            const QRect area = s->availableGeometry();
            // The lower bounds are applied last on purpose. When the panel is
            // larger than the work area the upper bound comes out below the lower
            // one, and the top-left corner is then the only answer worth having:
            // the header row and the first days stay readable.
            if (x + sz.width() > area.right() + 1) {
                x = area.right() + 1 - sz.width();
            }
            if (y + sz.height() > area.bottom() + 1) {
                y = area.bottom() + 1 - sz.height();
            }
            move(qMax(x, area.left()), qMax(y, area.top()));
            return;
        }
    }

    const QPoint p = QCursor::pos();
    QScreen *s = QGuiApplication::screenAt(p);
    if (!s) {
        s = QGuiApplication::primaryScreen();
    }
    if (!s) {
        return;
    }
    // The corner on the same side as the pointer, so the panel opens towards the
    // middle of the screen rather than off its edge. The work area rather than the
    // monitor geometry: the difference is exactly the desktop panels, and ignoring
    // it puts the forecast underneath them.
    const QRect area = s->availableGeometry();
    int px = area.left() + panelMargin;
    if (p.x() > area.left() + area.width() / 2) {
        px = area.right() + 1 - sz.width() - panelMargin;
    }
    int py = area.top() + panelMargin;
    if (p.y() > area.top() + area.height() / 2) {
        py = area.bottom() + 1 - sz.height() - panelMargin;
    }
    move(qMax(px, area.left()), qMax(py, area.top()));
}

// ---------------------------------------------------------------------------
// The settings form.
//
// The shim knows nothing about what any field MEANS: each carries an integer key
// the caller chose, and hands its value back under that key. That is what keeps
// the vocabulary of the settings - units, themes, intervals - in Go, where the
// configuration lives, while the widgets stay here, where Qt is.
// ---------------------------------------------------------------------------

const int formWidth = 460;
// What the window needs around the scrolling page: the button row, the margins
// and a title bar. Deliberately generous - a ceiling a few pixels too low costs a
// scrollbar nobody needed, one too high costs the buttons.
const int formChromeH = 120;

enum FieldKind { F_TEXT, F_LIST, F_CHOICE, F_COMBO, F_CHECK, F_SLIDER };

struct Field {
    int key = 0;
    FieldKind kind = F_TEXT;
    QWidget *w = nullptr;   // the input itself
    QWidget *aux = nullptr; // its action button, when it has one
    QVector<QRadioButton *> radios;
};

struct FormState {
    QPointer<QDialog> dlg;
    QWidget *pageW = nullptr;     // the scrolled page, kept for its size hint
    QScrollArea *scroll = nullptr;
    QVBoxLayout *page = nullptr;  // where fields are added right now
    QVBoxLayout *outer = nullptr; // the dialog's own box, which holds the buttons
    QVBoxLayout *groupBox = nullptr;
    QVector<Field *> fields;
    unsigned long long id = 0;
    void (*event)(unsigned long long, long long, long long, long long) = nullptr;
    void (*field)(unsigned long long, long long, const char *) = nullptr;
    int action = NIMBUS_QT_ACTION_CANCEL;
} form;

Field *fieldFor(int key) {
    for (Field *f : form.fields) {
        if (f->key == key) {
            return f;
        }
    }
    return nullptr;
}

// where returns the layout a new field belongs in: the open group if there is
// one, the page otherwise.
QVBoxLayout *where() { return form.groupBox ? form.groupBox : form.page; }

QString valueOf(const Field *f) {
    switch (f->kind) {
    case F_TEXT:
        return static_cast<QLineEdit *>(f->w)->text();
    case F_CHECK:
        return static_cast<QCheckBox *>(f->w)->isChecked() ? QStringLiteral("1") : QStringLiteral("0");
    case F_COMBO:
        return QString::number(static_cast<QComboBox *>(f->w)->currentIndex());
    case F_SLIDER:
        return QString::number(static_cast<QSlider *>(f->w)->value());
    case F_CHOICE:
        for (int i = 0; i < f->radios.size(); ++i) {
            if (f->radios[i]->isChecked()) {
                return QString::number(i);
            }
        }
        // No member is checked, which the caller has to be able to tell from a
        // selection: writing 0 here would silently move the user to the first
        // option of a group that could not be built.
        return QStringLiteral("-1");
    default:
        return QString();
    }
}

// finishForm reports the outcome exactly once and takes the window with it. Every
// exit routes through it - Save, Cancel, Delete, Escape and the window manager's
// own close button - so a caller blocked on the answer can never be stranded.
void finishForm(int action) {
    if (!form.event) {
        return;
    }
    void (*ev)(unsigned long long, long long, long long, long long) = form.event;
    void (*fld)(unsigned long long, long long, const char *) = form.field;
    const unsigned long long id = form.id;
    // Cleared before the callbacks rather than after: the Go side answers a
    // blocked goroutine from inside them, and may well ask for the next window
    // before this returns.
    form.event = nullptr;
    form.field = nullptr;

    if (action == NIMBUS_QT_ACTION_SAVE && fld) {
        for (Field *f : form.fields) {
            if (f->kind == F_LIST) {
                continue;
            }
            const QByteArray v = valueOf(f).toUtf8();
            fld(id, f->key, v.constData());
        }
    }
    ev(id, NIMBUS_QT_EV_ACTION, action, 0);

    qDeleteAll(form.fields);
    form.fields.clear();
    if (form.dlg) {
        form.dlg->deleteLater();
    }
    form.dlg = nullptr;
    form.page = form.outer = form.groupBox = nullptr;
    form.pageW = nullptr;
    form.scroll = nullptr;
}

} // namespace

int nimbus_qt_init(void) {
    if (app) {
        return 1;
    }
    // argc must outlive the QApplication - Qt keeps the reference. Static rather
    // than a local for exactly that reason; this is documented Qt behaviour and a
    // classic way to get a dangling reference.
    static int argc = 1;
    static char name[] = "nimbus";
    static char *argv[] = {name, nullptr};

    // QApplication aborts the process when it cannot connect to a display, which
    // would turn "no display" into a crash instead of the fallback the caller
    // expects. qputenv of QT_QPA_PLATFORM is not enough to make it soft, so the
    // check happens on the Go side before this is ever called.
    app = new QApplication(argc, argv);
    if (!app) {
        return 0;
    }
    // The tray, not the windows, keeps this process alive. Without this Qt exits
    // its loop the moment the last window closes, which for a tray applet means
    // the program vanishes when the user closes the forecast panel.
    app->setQuitOnLastWindowClosed(false);
    return 1;
}

void nimbus_qt_run(void) {
    if (app) {
        app->exec();
    }
}

void nimbus_qt_quit(void) {
    if (app) {
        // Queued, so this is safe from a thread that does not own the loop: it is
        // posted to the application and runs on the loop's own thread.
        QMetaObject::invokeMethod(app, "quit", Qt::QueuedConnection);
    }
}

void nimbus_qt_invoke(void (*fn)(unsigned long long), unsigned long long id) {
    if (!app || !fn) {
        return;
    }
    // Queued on the application object, so the lambda runs on the loop's thread
    // whichever thread posted it. This is the one call in the file that is safe to
    // make from anywhere.
    QMetaObject::invokeMethod(app, [fn, id]() { fn(id); }, Qt::QueuedConnection);
}

void nimbus_qt_about(const char *title, const char *body, const char *version) {
    if (!app) {
        return;
    }
    if (aboutBox) {
        aboutBox->raise();
        aboutBox->activateWindow();
        return;
    }

    QDialog *d = new QDialog();
    d->setAttribute(Qt::WA_DeleteOnClose);
    d->setWindowTitle(utf8(title));

    QVBoxLayout *box = new QVBoxLayout(d);
    box->setContentsMargins(20, 20, 20, 20);
    box->setSpacing(12);

    QLabel *name = new QLabel(QStringLiteral("<b style='font-size:20pt'>Nimbus</b>"), d);
    name->setAlignment(Qt::AlignCenter);
    box->addWidget(name);

    QLabel *sub = new QLabel(utf8(body), d);
    sub->setAlignment(Qt::AlignCenter);
    box->addWidget(sub);

    // Muted, and muted by asking the palette rather than by naming a grey. This
    // backend exists so the windows follow the desktop, so a hardcoded colour here
    // would defeat the only reason to have it.
    QLabel *ver = new QLabel(utf8(version), d);
    ver->setAlignment(Qt::AlignCenter);
    ver->setEnabled(false);
    box->addWidget(ver);

    // A third of the window wide and centred, the same shape the GTK and Win32
    // About boxes use.
    QPushButton *ok = new QPushButton(QStringLiteral("OK"), d);
    ok->setDefault(true);
    ok->setFixedWidth(106);
    box->addWidget(ok, 0, Qt::AlignHCenter);
    QObject::connect(ok, &QPushButton::clicked, d, &QDialog::close);

    aboutBox = d;
    d->show();
}

void nimbus_qt_error(const char *title, const char *message) {
    if (!app) {
        return;
    }
    if (errorBox) {
        // Update the text rather than stacking a second box: a later failure can
        // carry a different message, and without this only the first would ever be
        // read. Same rule as gtk.ShowError.
        errorBox->setText(utf8(message));
        errorBox->raise();
        errorBox->activateWindow();
        return;
    }

    QMessageBox *m = new QMessageBox(QMessageBox::Critical, utf8(title), utf8(message),
                                     QMessageBox::Ok);
    m->setAttribute(Qt::WA_DeleteOnClose);
    // Not modal, for the reason the GTK backend spells out: a modal dialog with no
    // parent takes input away from every other window this process owns, and an
    // error about a failed fetch is not a question that has to be answered before
    // the program can go on.
    m->setModal(false);
    errorBox = m;
    m->show();
}

void nimbus_qt_font(const void *data, int len) {
    if (!app || !data || len <= 0 || !symbolFamily.isEmpty()) {
        return;
    }
    const int id = QFontDatabase::addApplicationFontFromData(
        QByteArray(static_cast<const char *>(data), len));
    if (id < 0) {
        return;
    }
    const QStringList families = QFontDatabase::applicationFontFamilies(id);
    if (!families.isEmpty()) {
        symbolFamily = families.first();
    }
}

void nimbus_qt_icon(const void *data, int len) {
    if (!app || !data || len <= 0) {
        return;
    }
    QPixmap pm;
    if (!pm.loadFromData(static_cast<const uchar *>(data), static_cast<uint>(len))) {
        // A size that will not decode is not worth a failed startup: the window
        // manager falls back to its own default icon, which is what happened
        // before there was an icon here at all.
        return;
    }
    // Static, so that the second call adds to the first rather than replacing it.
    // QIcon holds every size it is given and hands the window manager whichever
    // one it asks for; that is the whole reason two are sent.
    static QIcon icon;
    icon.addPixmap(pm);
    QApplication::setWindowIcon(icon);
}

void nimbus_qt_forecast_begin(const char *title, int system, int dark) {
    spec = PanelSpec();
    spec.title = utf8(title);
    spec.system = system != 0;
    spec.dark = dark;
}

void nimbus_qt_forecast_header(const char *caption, int align) {
    Column c;
    c.caption = utf8(caption);
    c.align = align;
    spec.cols.append(c);
}

void nimbus_qt_forecast_row(const char *day, const char *symbol, const char *temp,
                            const char *wind, const char *precip) {
    Day d;
    d.date = utf8(day);
    d.symbol = utf8(symbol);
    d.temp = utf8(temp);
    d.wind = utf8(wind);
    d.precip = utf8(precip);
    spec.days.append(d);
}

void nimbus_qt_forecast_show(unsigned long long id, int have_at, int x, int y,
                             int (*pinned)(unsigned long long),
                             void (*event)(unsigned long long, long long, long long, long long)) {
    if (!app) {
        return;
    }
    if (panel) {
        // Already up. Raising it is what the GTK panel does, and it keeps the
        // caller's bookkeeping honest: no second panel and no second close.
        panel->raise();
        panel->activateWindow();
        spec = PanelSpec();
        return;
    }
    Panel *p = new Panel(spec, id, pinned, event);
    panel = p;
    p->place(have_at != 0, x, y);
    p->show();
    p->raise();
    p->activateWindow();
    spec = PanelSpec();
}

int nimbus_qt_forecast_close(void) {
    if (!panel) {
        return 0;
    }
    panel->dismiss(false);
    return 1;
}

void nimbus_qt_form_begin(const char *title) {
    if (form.dlg) {
        // A half-built form from a call that never reached show. Nothing is
        // waiting on it - show is what publishes the callbacks - so dropping it is
        // safe, and is the only way not to leak it.
        qDeleteAll(form.fields);
        form.fields.clear();
        delete form.dlg;
    }
    form = FormState();

    QDialog *d = new QDialog();
    d->setWindowTitle(utf8(title));
    form.dlg = d;

    form.outer = new QVBoxLayout(d);
    form.outer->setContentsMargins(14, 14, 14, 14);
    form.outer->setSpacing(10);

    // The page scrolls; the buttons do not. A window whose only height is its
    // content's natural height pushes the button row further down with every
    // setting added to it, until on a small screen Save sits below the bottom edge
    // with no way to reach it. Keeping the buttons out of the scrolled area means
    // the window can run out of room for the settings without ever running out of
    // room for the way to accept them.
    QWidget *pageW = new QWidget();
    form.pageW = pageW;
    form.page = new QVBoxLayout(pageW);
    form.page->setContentsMargins(0, 0, 0, 0);
    form.page->setSpacing(10);

    QScrollArea *scroll = new QScrollArea(d);
    form.scroll = scroll;
    scroll->setWidget(pageW);
    scroll->setWidgetResizable(true);
    scroll->setFrameShape(QFrame::NoFrame);
    scroll->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    form.outer->addWidget(scroll, 1);
}

void nimbus_qt_form_group(const char *caption) {
    if (!form.dlg) {
        return;
    }
    const QString title = utf8(caption);
    if (title.isEmpty()) {
        form.groupBox = nullptr;
        return;
    }
    QGroupBox *g = new QGroupBox(title);
    QVBoxLayout *v = new QVBoxLayout(g);
    v->setSpacing(8);
    form.page->addWidget(g);
    form.groupBox = v;
}

void nimbus_qt_form_text(int key, const char *label, const char *value, const char *action) {
    if (!form.dlg) {
        return;
    }
    Field *f = new Field();
    f->key = key;
    f->kind = F_TEXT;

    QLineEdit *e = new QLineEdit(utf8(value));
    f->w = e;

    QHBoxLayout *row = new QHBoxLayout();
    row->setContentsMargins(0, 0, 0, 0);
    row->setSpacing(6);
    const QString cap = utf8(label);
    if (!cap.isEmpty()) {
        row->addWidget(new QLabel(cap));
    }
    row->addWidget(e, 1);

    const QString act = utf8(action);
    if (!act.isEmpty()) {
        QPushButton *b = new QPushButton(act);
        f->aux = b;
        // The id and the callback are read from form at click time rather than
        // captured here, so a stale button from a form that has already finished
        // cannot report into a caller that has moved on: finishForm clears both.
        QObject::connect(b, &QPushButton::clicked, b, [key]() {
            if (form.event) {
                form.event(form.id, NIMBUS_QT_EV_SEARCH, key, 0);
            }
        });
        row->addWidget(b);
    }

    where()->addLayout(row);
    form.fields.append(f);
}

void nimbus_qt_form_list(int key, int min_height) {
    if (!form.dlg) {
        return;
    }
    Field *f = new Field();
    f->key = key;
    f->kind = F_LIST;

    QListWidget *list = new QListWidget();
    list->setMinimumHeight(min_height);
    // Elided rather than scrolled sideways: a row is "city, country (lat, lon)"
    // and the tail is the least interesting part of it, so losing the end of a
    // long one costs nothing, while a horizontal scrollbar costs a sixth of a
    // list that is only four rows tall to begin with.
    list->setTextElideMode(Qt::ElideRight);
    list->setHorizontalScrollBarPolicy(Qt::ScrollBarAlwaysOff);
    f->w = list;
    QObject::connect(list, &QListWidget::currentRowChanged, list, [key](int row) {
        if (row >= 0 && form.event) {
            form.event(form.id, NIMBUS_QT_EV_PICK, key, row);
        }
    });

    where()->addWidget(list);
    form.fields.append(f);
}

void nimbus_qt_form_choice(int key, const char *label) {
    if (!form.dlg) {
        return;
    }
    Field *f = new Field();
    f->key = key;
    f->kind = F_CHOICE;

    QGroupBox *g = new QGroupBox(utf8(label));
    QHBoxLayout *row = new QHBoxLayout(g);
    row->setSpacing(12);
    row->addStretch(1);
    f->w = g;

    where()->addWidget(g);
    form.fields.append(f);
}

void nimbus_qt_form_combo(int key, const char *label) {
    if (!form.dlg) {
        return;
    }
    Field *f = new Field();
    f->key = key;
    f->kind = F_COMBO;

    QComboBox *c = new QComboBox();
    f->w = c;

    QGroupBox *g = new QGroupBox(utf8(label));
    QVBoxLayout *v = new QVBoxLayout(g);
    v->addWidget(c);

    where()->addWidget(g);
    form.fields.append(f);
}

void nimbus_qt_form_option(const char *label, int selected) {
    if (form.fields.isEmpty()) {
        return;
    }
    Field *f = form.fields.last();
    if (f->kind == F_COMBO) {
        QComboBox *c = static_cast<QComboBox *>(f->w);
        c->addItem(utf8(label));
        if (selected) {
            c->setCurrentIndex(c->count() - 1);
        }
        return;
    }
    if (f->kind != F_CHOICE) {
        return;
    }
    QGroupBox *g = static_cast<QGroupBox *>(f->w);
    QRadioButton *b = new QRadioButton(utf8(label), g);
    b->setChecked(selected != 0);
    // Inserted before the trailing stretch, so the buttons stay packed to the
    // leading edge however many of them there turn out to be.
    QHBoxLayout *row = static_cast<QHBoxLayout *>(g->layout());
    row->insertWidget(row->count() - 1, b);
    f->radios.append(b);
}

void nimbus_qt_form_check(int key, const char *label, int checked) {
    if (!form.dlg) {
        return;
    }
    Field *f = new Field();
    f->key = key;
    f->kind = F_CHECK;

    QCheckBox *c = new QCheckBox(utf8(label));
    c->setChecked(checked != 0);
    f->w = c;

    where()->addWidget(c);
    form.fields.append(f);
}

void nimbus_qt_form_slider(int key, const char *label, int min, int max, int value) {
    if (!form.dlg) {
        return;
    }
    Field *f = new Field();
    f->key = key;
    f->kind = F_SLIDER;

    QSlider *s = new QSlider(Qt::Horizontal);
    s->setRange(min, max);
    s->setValue(value);
    f->w = s;

    QLabel *readout = new QLabel(QStringLiteral("%1%").arg(value));
    // The readout follows the thumb; the caller's preview need not. Every step is
    // reported and coalescing is the caller's business - it is the side that knows
    // what its own preview costs.
    QObject::connect(s, &QSlider::valueChanged, s, [readout, key](int v) {
        readout->setText(QStringLiteral("%1%").arg(v));
        if (form.event) {
            form.event(form.id, NIMBUS_QT_EV_SLIDE, key, v);
        }
    });

    QGroupBox *g = new QGroupBox(utf8(label));
    QHBoxLayout *row = new QHBoxLayout(g);
    row->setSpacing(8);
    row->addWidget(s, 1);
    row->addWidget(readout);

    where()->addWidget(g);
    form.fields.append(f);
}

void nimbus_qt_form_buttons(const char *save, const char *cancel, const char *reset) {
    if (!form.dlg) {
        return;
    }
    QHBoxLayout *row = new QHBoxLayout();
    row->setSpacing(8);

    QPushButton *s = new QPushButton(utf8(save));
    s->setDefault(true);
    QObject::connect(s, &QPushButton::clicked, s, []() {
        form.action = NIMBUS_QT_ACTION_SAVE;
        if (form.dlg) {
            form.dlg->accept();
        }
    });
    QPushButton *c = new QPushButton(utf8(cancel));
    QObject::connect(c, &QPushButton::clicked, c, []() {
        form.action = NIMBUS_QT_ACTION_CANCEL;
        if (form.dlg) {
            form.dlg->reject();
        }
    });
    QPushButton *r = new QPushButton(utf8(reset));
    QObject::connect(r, &QPushButton::clicked, r, []() {
        form.action = NIMBUS_QT_ACTION_RESET;
        if (form.dlg) {
            form.dlg->accept();
        }
    });

    row->addWidget(s, 1);
    row->addWidget(c, 1);
    row->addWidget(r, 1);
    // Into the dialog's own box and not the scrolled page: these have to stay
    // reachable however tall the settings get.
    form.outer->addLayout(row);
}

void nimbus_qt_form_show(unsigned long long id,
                         void (*event)(unsigned long long, long long, long long, long long),
                         void (*field)(unsigned long long, long long, const char *)) {
    if (!app || !form.dlg) {
        // Nothing was built, so nothing can report an action - and the caller is
        // blocked waiting for one. Answer it here rather than leave it waiting.
        if (event) {
            event(id, NIMBUS_QT_EV_ACTION, NIMBUS_QT_ACTION_CANCEL, 0);
        }
        return;
    }
    form.id = id;
    form.event = event;
    form.field = field;
    form.action = NIMBUS_QT_ACTION_CANCEL;

    QDialog *d = form.dlg;
    // Escape and the window manager's close button both reject the dialog, which
    // is why finished is connected rather than each button on its own: it is the
    // one place every exit passes through, so the promise that exactly one action
    // is reported holds even for exits this file never sees.
    QObject::connect(d, &QDialog::finished, d, [](int) { finishForm(form.action); });

    // The height the form WANTS is the page's full height plus the chrome, not the
    // dialog's own hint: a QScrollArea reports a small hint by design - it is
    // willing to scroll - so asking the dialog would open every form short enough
    // to need scrolling even on an empty screen. Substituting the page's hint for
    // the scroller's inside the dialog's own arithmetic keeps every margin and the
    // button row accounted for without repeating any of their numbers here.
    int want = d->sizeHint().height();
    if (form.pageW && form.scroll) {
        want += form.pageW->sizeHint().height() - form.scroll->sizeHint().height();
    }
    QScreen *sc = QGuiApplication::screenAt(QCursor::pos());
    if (!sc) {
        sc = QGuiApplication::primaryScreen();
    }
    if (sc) {
        // The work area the pointer is on, less what a title bar and a margin of
        // comfort need. A form that fits opens whole; a taller one scrolls instead
        // of growing past the bottom edge of the screen.
        const int cap = sc->availableGeometry().height() - formChromeH;
        if (cap > 0 && want > cap) {
            want = cap;
        }
    }
    d->resize(formWidth, want);
    d->show();
    d->raise();
    d->activateWindow();
}

void nimbus_qt_form_set(int key, const char *value) {
    Field *f = fieldFor(key);
    if (!f || f->kind != F_TEXT) {
        return;
    }
    static_cast<QLineEdit *>(f->w)->setText(utf8(value));
}

void nimbus_qt_form_enable(int key, int on) {
    Field *f = fieldFor(key);
    if (!f) {
        return;
    }
    // The action button when the field has one: greying out the field itself while
    // its own search is in flight would stop the user correcting the query.
    if (f->aux) {
        f->aux->setEnabled(on != 0);
        return;
    }
    if (f->w) {
        f->w->setEnabled(on != 0);
    }
}

void nimbus_qt_form_report(int key) {
    Field *f = fieldFor(key);
    if (!f || !form.field || f->kind == F_LIST) {
        return;
    }
    const QByteArray v = valueOf(f).toUtf8();
    form.field(form.id, f->key, v.constData());
}

void nimbus_qt_form_list_clear(int key) {
    Field *f = fieldFor(key);
    if (!f || f->kind != F_LIST) {
        return;
    }
    QListWidget *list = static_cast<QListWidget *>(f->w);
    // Signals blocked, because clearing moves the current row and would otherwise
    // report a pick the user never made.
    const bool was = list->blockSignals(true);
    list->clear();
    list->blockSignals(was);
}

void nimbus_qt_form_list_add(int key, const char *label) {
    Field *f = fieldFor(key);
    if (!f || f->kind != F_LIST) {
        return;
    }
    QListWidget *list = static_cast<QListWidget *>(f->w);
    const bool was = list->blockSignals(true);
    list->addItem(utf8(label));
    list->blockSignals(was);
}
