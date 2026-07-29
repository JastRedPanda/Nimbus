// Package appicon holds the artwork every Nimbus window is drawn with, and
// nothing else.
//
// It is a package of its own for one blunt reason: go:embed can only reach files
// in its own directory, so an asset two backends need cannot live inside either
// of them. These two PNGs used to be embedded in internal/ui, which put them
// behind that package's build tags - and the Qt build, which excludes the GTK
// files, ended up with no window icon at all while every other build had one.
// Copying the PNGs into a second directory would have fixed the symptom and left
// two sets of artwork to keep in step.
//
// Not to be confused with internal/icons, which GENERATES the tray glyph from
// the current temperature. This is the fixed application artwork.
package appicon

import _ "embed"

// Both sizes come from nimbusicon.ico, the same artwork the Windows build uses:
// 32 is the icon's own frame, drawn for title bars, and 128 covers the window
// switcher and HiDPI without the window manager having to rescale the small one.
//
//go:embed appicon32.png
var png32 []byte

//go:embed appicon128.png
var png128 []byte

// PNG32 and PNG128 are the encoded artwork. They return the package's own
// slices; every caller either decodes them or hands them to a toolkit that
// copies them, and nothing writes to them.
func PNG32() []byte { return png32 }

func PNG128() []byte { return png128 }

// All returns both, smallest first, for a caller that installs every size a
// window manager might ask for.
func All() [][]byte { return [][]byte{png32, png128} }
