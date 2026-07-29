//go:build linux && qt

package tray

// The Qt backend registers itself from its own init, so it has to be linked in,
// and nothing imports it for its API - the same blank import that pulls in the
// GTK and Win32 backends above.
//
// Behind the qt tag because the package embeds a shared object that an ordinary
// build neither has nor needs: qtshim/ is compiled separately by `make qt`, and
// without the tag this import would make every build require a C++ toolchain and
// Qt development packages.
import _ "github.com/JastRedPanda/Nimbus/internal/qt"
