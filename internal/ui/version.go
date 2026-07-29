package ui

import "github.com/JastRedPanda/Nimbus/internal/build"

// versionLine is what the About window shows underneath the subtitle. The
// formatting lives in internal/build because all three backends need it and none
// of them owns it.
func versionLine() string { return build.Line() }
