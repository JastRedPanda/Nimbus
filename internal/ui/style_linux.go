//go:build linux

package ui

// forecastCSS is the application stylesheet for the forecast panel.
//
// Four rules govern everything here, all learned by measurement against the
// BlackMATE desktop theme and the Marco window manager:
//
// Every selector is scoped under #nimbus-forecast. The provider is installed
// screen-wide, so an unscoped rule would restyle the tray menu, the About
// window and the error dialog too - and since the sheet is installed once per
// process, that mistake could not be undone at runtime.
//
// Colour, font and spacing are stated on the nodes that draw them, never
// inherited from an ancestor. Provider priority beats the theme for any node a
// selector matches, but it does not beat "specified on the child" versus
// "inherited from your ancestor": BlackMATE ships `label { color: ... }` and
// `image, label { padding: 3px; }`, so a colour set on the window reaches no
// label at all, and every label silently gains 6px unless told otherwise.
//
// Separators are the one node the reset must not touch. A GtkSeparator has no
// content: its entire appearance is a 1px minimum size plus a background
// colour, so the `min-height: 0` that the reset needs everywhere else would
// make every rule in the table vanish. It gets its own reset, one line
// different.
//
// Four palettes live in this one sheet - dark and light, each solid and
// translucent - because LoadCSS installs a single provider for the life of the
// process. The translucent halves are only ever selected when the window
// actually got an RGBA visual; without a compositor their alpha would be
// composited against black, and the rounded corners the translucent palettes
// ask for would render as black notches.
//
// THE SYSTEM LOOK USES THIS SHEET WITH NO PALETTE CLASS, which is how "colours
// from the desktop theme" is implemented. It carries one marker class, .system,
// whose single rule sets a thickness and never a colour - see the block below the
// font sizes for what that is for. Every colour, every background and the
// one border-radius below hangs off .dark/.light or .solid/.translucent, so a
// window carrying nothing but the #nimbus-forecast name matches the two resets,
// the page padding and the two font sizes and not one rule more; the theme then
// paints the window, the labels, the header and the separators itself. That is
// also why the resets are not folded into the palette selectors even though it
// would shorten the sheet by two blocks: the system look would get BlackMATE's
// 6px label padding and its label colours back, with nothing in this file left to
// explain why. Measured under BlackMATE with no class applied: opaque grey
// window, legible near-white labels, and separators the theme paints a shade
// darker than the background - so the system look needs no colour rule of its own
// and has none.
const forecastCSS = `
/* Reset: undo what the desktop theme states on these nodes. */
#nimbus-forecast,
#nimbus-forecast box,
#nimbus-forecast grid,
#nimbus-forecast label,
#nimbus-forecast image,
#nimbus-forecast button {
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

/* The system look's only rule, and it names no colour on purpose.
   With nothing but the resets matching, the header rule and the row hairlines
   both fell through to the theme's single separator colour and the two weights
   the table is built on became one - 1.08:1 against the background under
   Adwaita:dark, which is not visible at all. Thickness restores the hierarchy
   while the colour stays the theme's, which is the whole point of the look. */
#nimbus-forecast.system .rule {
  min-height: 2px;
}

/* The Modern panel has no title bar, so closing it needs a visible affordance.
   The system look packs no such button - the title bar carries one - so these
   declarations simply match nothing there. */
/* Bold as well as larger: U+00D7 has thin strokes and little ink for its em,
   so size alone barely reads. It is kept over a heavier codepoint like U+2715
   because every font ships it. */
#nimbus-forecast .close {
  font-size: 15pt;     /* closePt */
  font-weight: bold;
  padding: 0 9px;      /* closePadY closePadX */
  border-radius: 8px;  /* closeRadiusPt */
  background-color: transparent;
}

/* ---- dark ---- */
#nimbus-forecast.dark label            { color: #f2f4f7; }
#nimbus-forecast.dark .thead           { color: #9aa3b0; }
#nimbus-forecast.dark separator        { background-color: rgba(255,255,255,0.10); }
#nimbus-forecast.dark .rule            { background-color: rgba(255,255,255,0.28); }
#nimbus-forecast.dark .close           { color: #9aa3b0; }
#nimbus-forecast.dark .close:hover     { background-color: rgba(255,255,255,0.10); color: #f2f4f7; }

#nimbus-forecast.dark.solid            { background-color: #1c1f26; }
#nimbus-forecast.dark.translucent      { background-color: rgba(28,31,38,0.96);
                                         border-radius: 14px; /* sheetRadiusPt */ }

/* ---- light ---- */
#nimbus-forecast.light label            { color: #14161a; }
#nimbus-forecast.light .thead           { color: #5b6472; }
#nimbus-forecast.light separator        { background-color: rgba(0,0,0,0.10); }
#nimbus-forecast.light .rule            { background-color: rgba(0,0,0,0.24); }
#nimbus-forecast.light .close           { color: #5b6472; }
#nimbus-forecast.light .close:hover     { background-color: rgba(0,0,0,0.08); color: #14161a; }

#nimbus-forecast.light.solid            { background-color: #ffffff; }
#nimbus-forecast.light.translucent      { background-color: rgba(255,255,255,0.98);
                                          border-radius: 14px; /* sheetRadiusPt */ }
`
