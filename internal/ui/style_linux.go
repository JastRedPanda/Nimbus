//go:build linux

package ui

// forecastCSS is the application stylesheet for the forecast panel.
//
// Three rules govern everything here, all learned by measurement against the
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
// Four palettes live in this one sheet - dark and light, each solid and
// translucent - because LoadCSS installs a single provider for the life of the
// process. The translucent halves are only ever selected when the window
// actually got an RGBA visual; without a compositor their alpha would be
// composited against black and every rounded corner would render as a black
// notch.
const forecastCSS = `
/* Reset: undo what the desktop theme states on these nodes. */
#nimbus-forecast,
#nimbus-forecast box,
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

#nimbus-forecast .page {
  padding: 14px;
}

#nimbus-forecast .card {
  border-radius: 14px;
  padding: 16px 18px;
}

#nimbus-forecast .hero {
  font-size: 34pt;
  font-weight: 300;
}

#nimbus-forecast .cond  { font-size: 13pt; }
#nimbus-forecast .muted { font-size: 11pt; }

#nimbus-forecast .day {
  border-radius: 12px;
  padding: 12px 6px;
  border: 1px solid transparent;
}

#nimbus-forecast .day label { font-size: 11pt; }

#nimbus-forecast .daytemp {
  font-size: 11pt;
  font-weight: 600;
}

/* The panel has no title bar, so closing it needs a visible affordance. */
#nimbus-forecast .close {
  font-size: 13pt;
  padding: 2px 8px;
  border-radius: 8px;
  background-color: transparent;
}

/* ---- dark ---- */
#nimbus-forecast.dark label            { color: #f2f4f7; }
#nimbus-forecast.dark .muted           { color: #9aa3b0; }
#nimbus-forecast.dark .cond            { color: #d6dbe3; }
#nimbus-forecast.dark .close           { color: #9aa3b0; }
#nimbus-forecast.dark .close:hover     { background-color: rgba(255,255,255,0.10); color: #f2f4f7; }
#nimbus-forecast.dark .day.today       { border-color: #6f7b8d; }

#nimbus-forecast.dark.solid                  { background-color: #16181d; }
#nimbus-forecast.dark.solid .card,
#nimbus-forecast.dark.solid .day             { background-color: #21242b; }
#nimbus-forecast.dark.solid .day.today       { background-color: #272b34; }

#nimbus-forecast.dark.translucent            { background-color: rgba(0,0,0,0); }
#nimbus-forecast.dark.translucent .card,
#nimbus-forecast.dark.translucent .day       { background-color: rgba(28,31,38,0.82);
                                               box-shadow: 0 2px 10px rgba(0,0,0,0.45); }
#nimbus-forecast.dark.translucent .day.today { background-color: rgba(42,47,57,0.88); }

/* ---- light ---- */
#nimbus-forecast.light label            { color: #14161a; }
#nimbus-forecast.light .muted           { color: #5b6472; }
#nimbus-forecast.light .cond            { color: #38414f; }
#nimbus-forecast.light .close           { color: #5b6472; }
#nimbus-forecast.light .close:hover     { background-color: rgba(0,0,0,0.08); color: #14161a; }
#nimbus-forecast.light .day.today       { border-color: #9aa6b6; }

#nimbus-forecast.light.solid                  { background-color: #eef1f5; }
#nimbus-forecast.light.solid .card,
#nimbus-forecast.light.solid .day             { background-color: #ffffff; }
#nimbus-forecast.light.solid .day.today       { background-color: #e7ecf3; }

#nimbus-forecast.light.translucent            { background-color: rgba(0,0,0,0); }
#nimbus-forecast.light.translucent .card,
#nimbus-forecast.light.translucent .day       { background-color: rgba(255,255,255,0.88);
                                                box-shadow: 0 2px 10px rgba(0,0,0,0.22); }
#nimbus-forecast.light.translucent .day.today { background-color: rgba(226,233,242,0.94); }
`
