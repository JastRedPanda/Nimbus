# deb-gtk, deb-qt, rpm-gtk and rpm-qt are deliberately NOT phony: make skips
# implicit-rule search for a phony target, so listing them here would leave the
# four of them with prerequisites and no recipe - they would build the binary
# and then quietly produce no package at all. There is no file by any of those
# names for make to confuse them with.
.PHONY: build build-linux gtk qt shim install-deb-gtk install-deb-qt \
        clean clean-shim resource resource-check

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "1.0.0")
DATE := $(shell date +%m.%Y)
BUILD_DIR := build

# TWO LINUX FLAVOURS, and each carries exactly one toolkit.
#
#   nimbus-gtk   GTK windows. No Qt anywhere in it.
#   nimbus-qt    Qt windows. No GTK anywhere in it - internal/ui's GTK files and
#                internal/tray's GTK loop are both behind !qt, so the binary can
#                never dlopen libgtk-3.
#
# They install side by side: different binary names, different .desktop files,
# different icons. Which one to run is the user's choice at install time rather
# than a guess the program makes about the desktop it woke up on.
#
# What both keep: CGO_ENABLED=0, no versioned glibc symbol, one file. The Qt
# half is a shared object built by `shim` and embedded in the binary - see the
# qt target.

# The version and date the About window shows. Injected into every target here,
# because a build whose About box says 1.0.0 when the tag says otherwise is a
# bug report nobody can act on.
LDFLAGS := -s -w \
	-X github.com/JastRedPanda/Nimbus/internal/build.Version=$(VERSION) \
	-X github.com/JastRedPanda/Nimbus/internal/build.Date=$(DATE)

# Windows resource object: application icon, application manifest and
# VERSIONINFO, in one COFF object the Go linker picks up on its own.
#
# The _windows_amd64 suffix is a build constraint, not decoration. An
# unsuffixed nimbus.syso is offered to every target: go list -f
# '{{.SysoFiles}}' reported it under GOOS=linux, `go tool pack` stuffed it into
# the Linux package archive and cmd/link parsed the COFF (it dispatches on the
# object's magic, not on the target). The resource bytes did not reach the ELF
# - only the PE writer emits a .rsrc section, so they were dropped as dead
# weight, and a Linux binary built with and without the file is byte-identical.
# What the rename does fix is a windows/386 or windows/arm64 build being handed
# an amd64 COFF object. Now:
#   GOOS=linux   -> []        GOOS=windows GOARCH=amd64 -> [nimbus_windows_amd64.syso]
#   windows/386  -> []        windows/arm64             -> []
#
# The object is checked in so that a plain `go build` still works with no
# tooling and no network - CI does exactly that. This target is how it gets
# regenerated, and it is reproducible: same inputs, byte-identical output.
SYSO := nimbus_windows_amd64.syso

build:
	mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS) -H windowsgui" -o $(BUILD_DIR)/nimbus.exe .

# Regenerate $(SYSO) from winres/. Run this after editing the manifest, the
# version strings or nimbusicon.ico - and before cutting a release, so that the
# version in the .exe Properties dialog matches the tag that -ldflags injects
# into internal/build.Version. Nothing rebuilds it automatically: the object is
# tracked, and a release build should not silently depend on the network.
#
# winres/ is its own Go module on purpose, so goversioninfo and rsrc stay out
# of the application's dependency graph. That is also why this recipe cds into
# it instead of using `go run ./winres`.
resource:
	cd winres && go run . \
		-icon ../nimbusicon.ico \
		-manifest nimbus.exe.manifest \
		-versioninfo versioninfo.json \
		-version $(VERSION) \
		-arch amd64 \
		-out ../$(SYSO)

# Read the object back and check the things that would otherwise only fail on
# a user's desktop: manifest under RT_MANIFEST id 1, icon group under id 1 (the
# ID internal/ui passes to LoadIcon), a well-formed VS_VERSIONINFO, and
# resource directories sorted the way the PE loader binary-searches them.
# Works on Linux - no Windows and no mingw involved.
resource-check:
	cd winres && go run . -dump ../$(SYSO)

# CGO_ENABLED=0: nothing in the project uses cgo since the tray moved to a
# pure-Go implementation, and a cgo build binds the binary to the build
# machine's glibc - GLIBC_2.34 here, which will not start on Debian 11,
# Ubuntu 20.04 or RHEL 8. The pure build requires no versioned glibc symbol
# at all, and is 319 KB smaller.
gtk:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/nimbus-gtk .

# The old name, kept because fingers and scripts remember it.
build-linux: gtk

# The Qt backend, for desktops where Qt is what everything else is drawn with.
#
# Two builds, both `go build` and nothing else. The first compiles qtshim/ - a
# nested module, like winres/, so that a C++ toolchain and Qt never enter the
# application's dependency graph - into a shared object with cgo. The second
# embeds that object into an ordinary CGO_ENABLED=0 binary, which loads it from
# memory at startup and falls back to GTK when Qt is missing.
#
# So the Qt-capable binary keeps what the plain one has: one file, no versioned
# glibc symbol, and a graceful answer on a machine with no toolkit at all.
#
# Needs Qt development packages on the BUILD machine only (qt6-base-dev, or
# qt6-base on Arch). An ordinary `make build-linux` needs none of it.
# The Qt half, built by cgo from qtshim/ - a nested module, like winres/, so that
# a C++ toolchain and Qt never enter the application's dependency graph.
#
# -s -w halves it, from 2.7 MB to 1.4 MB, and the 28 entry points stay exported:
# they are dynamic symbols, which stripping does not touch. The object is not
# checked in, which is why internal/qt is behind a build tag - an ordinary build
# must not need a C++ toolchain, or Qt, or this file to exist.
#
# Build it where you want the FLOOR to be. It is the only part of Nimbus that
# links anything at build time, so the glibc and Qt it is built against are the
# oldest it will run on; the release workflow builds it in an ubuntu:22.04
# container for that reason and checks the result.
#
# Qt is located with qmake6 rather than with pkg-config, because Debian and
# Ubuntu ship no .pc files for Qt 6 at all - qt6-base-dev installs none, and
# `#cgo pkg-config: Qt6Widgets` therefore failed on every distribution except the
# one this was first written on. qmake6 is part of the Qt 6 development package
# everywhere and answers for the Qt it belongs to.
QT_HEADERS := $(shell qmake6 -query QT_INSTALL_HEADERS 2>/dev/null)
QT_LIBS := $(shell qmake6 -query QT_INSTALL_LIBS 2>/dev/null)

shim:
	@test -n "$(QT_HEADERS)" || { echo "qmake6 not found: install the Qt 6 development package (qt6-base-dev, qt6-base, qt6-qtbase-devel)"; exit 1; }
	cd qtshim && \
		CGO_CXXFLAGS="-I$(QT_HEADERS) -I$(QT_HEADERS)/QtCore -I$(QT_HEADERS)/QtGui -I$(QT_HEADERS)/QtWidgets" \
		CGO_LDFLAGS="-L$(QT_LIBS) -lQt6Widgets -lQt6Gui -lQt6Core" \
		go build -buildmode=c-shared -ldflags="-s -w" -o ../internal/qt/libnimbusqt.so .

qt: shim
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -tags qt -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/nimbus-qt .

# The two packages are built by one recipe each, with $* standing for the
# flavour, because everything that differs between them - the dependency, the
# binary, the .desktop, the icon - is named after it. dist/debian/control-gtk and
# control-qt are the only files that state anything flavour-specific.
deb-gtk: gtk
deb-qt: qt
deb-%:
	rm -rf dist/debian/nimbus-$*
	mkdir -p dist/debian/nimbus-$*/DEBIAN
	mkdir -p dist/debian/nimbus-$*/usr/bin
	mkdir -p dist/debian/nimbus-$*/usr/share/applications
	mkdir -p dist/debian/nimbus-$*/usr/share/pixmaps
	mkdir -p dist/debian/nimbus-$*/usr/share/doc/nimbus-$*
	install -m 755 $(BUILD_DIR)/nimbus-$* dist/debian/nimbus-$*/usr/bin/
	install -m 644 dist/nimbus-$*.desktop dist/debian/nimbus-$*/usr/share/applications/
	install -m 644 nimbus1.png dist/debian/nimbus-$*/usr/share/pixmaps/nimbus-$*.png
	install -m 644 LICENSE THIRD-PARTY-LICENSES dist/debian/nimbus-$*/usr/share/doc/nimbus-$*/
	sed 's/^Version:.*/Version: $(VERSION)/' dist/debian/control-$* > dist/debian/nimbus-$*/DEBIAN/control
	dpkg-deb --build dist/debian/nimbus-$* dist/nimbus-$*_$(VERSION)-1_amd64.deb
	@echo ""
	@echo "=== Package built: dist/nimbus-$*_$(VERSION)-1_amd64.deb ==="
	@echo "Install with: sudo apt install ./dist/nimbus-$*_$(VERSION)-1_amd64.deb"

install-deb-gtk: deb-gtk
	sudo apt install ./dist/nimbus-gtk_$(VERSION)-1_amd64.deb

install-deb-qt: deb-qt
	sudo apt install ./dist/nimbus-qt_$(VERSION)-1_amd64.deb

# The spec installs from build-rpm/, so the files have to be put there first.
# That copy used to live only in the release workflow, which meant `make rpm`
# alone produced a package with no binary in it.
rpm-gtk: gtk
rpm-qt: qt
rpm-%:
	mkdir -p dist/rpm/BUILD/build-rpm dist/rpm/RPMS dist/rpm/SRPMS
	cp $(BUILD_DIR)/nimbus-$* nimbus1.png LICENSE THIRD-PARTY-LICENSES dist/rpm/BUILD/build-rpm/
	sed 's/^Version:.*/Version: $(VERSION)/' dist/rpm/nimbus-$*.spec > dist/rpm/nimbus-$*-build.spec
	rpmbuild -bb --define "_topdir $(PWD)/dist/rpm" dist/rpm/nimbus-$*-build.spec

clean-shim:
	rm -f internal/qt/libnimbusqt.so

clean:
	rm -rf $(BUILD_DIR) dist/debian/nimbus-gtk dist/debian/nimbus-qt
	rm -rf dist/rpm/BUILD dist/rpm/RPMS dist/rpm/SRPMS
	rm -f dist/rpm/nimbus-*-build.spec internal/qt/libnimbusqt.so
	rm -f dist/nimbus-*.deb
