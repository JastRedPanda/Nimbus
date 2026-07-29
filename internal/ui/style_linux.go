//go:build linux && !qt

package ui

// forecastCSS is the application stylesheet for the forecast panel.
//
// It names no colour anywhere, and that is the design rather than an omission:
// the panel is an ordinary application window, so the desktop theme paints its
// background, its labels and its separators, and all this sheet does is undo what
// a theme states on the nodes a table is built from and set the two sizes and the
// one thickness that are ours. Measured under BlackMATE: opaque grey window,
// legible near-white labels, and separators the theme paints a shade darker than
// the background.
//
// The rules that shape what is left, all learned by measurement against that
// theme and the Marco window manager:
//
// Every selector is scoped under #nimbus-forecast. The provider is installed
// screen-wide, so an unscoped rule would restyle the tray menu, the About
// window and the error dialog too - and since the sheet is installed once per
// process, that mistake could not be undone at runtime.
//
// Spacing is stated on the nodes that draw it, never inherited from an ancestor.
// Provider priority beats the theme for any node a selector matches, but it does
// not beat "specified on the child" versus "inherited from your ancestor":
// BlackMATE ships `image, label { padding: 3px; }`, so every label silently gains
// 6px unless told otherwise. That is also why the reset lists the node types one
// by one instead of leaning on the window it is scoped to.
//
// Separators are the one node the reset must not touch. A GtkSeparator has no
// content: its entire appearance is a 1px minimum size plus a background
// colour, so the `min-height: 0` that the reset needs everywhere else would
// make every rule in the table vanish. It gets its own reset, one line
// different.
//
// The reset names no button on purpose. The panel packs none, so the only
// buttons under #nimbus-forecast are the ones a client-side-decorating GTK draws
// into the title bar, and stripping their padding and background would deface the
// very frame the panel asks for.
const forecastCSS = `
/* Reset: undo what the desktop theme states on these nodes. */
#nimbus-forecast,
#nimbus-forecast box,
#nimbus-forecast grid,
#nimbus-forecast label,
#nimbus-forecast image {
  padding: 0;
  margin: 0;
  min-width: 0;
  min-height: 0;
  border: none;
  background-clip: border-box;
  background-image: none;
  box-shadow: none;
  text-shadow: none;
}

/* Same reset, except for the min-height that IS the rule. */
#nimbus-forecast separator {
  padding: 0;
  margin: 0;
  min-width: 0;
  min-height: 1px;
  border: none;
  background-image: none;
  box-shadow: none;
}

#nimbus-forecast .page {
  padding: 4px 14px 14px 14px;   /* pagePadTop, then pagePad */
}

/* Table type. One size for the whole table: a header row that shouts is a
   card-layout habit, and weight alone separates it from the data. */
#nimbus-forecast .thead {
  font-size: 11pt;     /* theadPt */
  font-weight: 600;
}

#nimbus-forecast .cell {
  font-size: 11pt;     /* cellPt */
}

/* The only rule here that is not a reset, and it names no colour on purpose.
   With the resets alone, the header rule and the row hairlines both fell through
   to the theme's single separator colour and the two weights the table is built
   on became one - 1.08:1 against the background under Adwaita:dark, which is not
   visible at all. Thickness restores the hierarchy while the colour stays the
   theme's. The Win32 backend restores it with two alphas of COLOR_3DSHADOW. */
#nimbus-forecast.system .rule {
  min-height: 2px;
}
`
