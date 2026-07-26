.PHONY: build deb install-deb rpm clean resource resource-check

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "1.0.0")
DEB_NAME := nimbus_$(VERSION)-1_amd64.deb
BUILD_DIR := build

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
	go build -ldflags="-s -w -H windowsgui" -o $(BUILD_DIR)/nimbus.exe .

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
build-linux:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BUILD_DIR)/nimbus .

deb: build-linux
	mkdir -p dist/debian/nimbus/DEBIAN
	mkdir -p dist/debian/nimbus/usr/bin
	mkdir -p dist/debian/nimbus/usr/share/applications
	mkdir -p dist/debian/nimbus/usr/share/pixmaps
	mkdir -p dist/debian/nimbus/usr/share/doc/nimbus
	install -m 755 $(BUILD_DIR)/nimbus dist/debian/nimbus/usr/bin/
	install -m 644 dist/nimbus.desktop dist/debian/nimbus/usr/share/applications/
	install -m 644 nimbus1.png dist/debian/nimbus/usr/share/pixmaps/nimbus.png
	install -m 644 LICENSE THIRD-PARTY-LICENSES dist/debian/nimbus/usr/share/doc/nimbus/
	sed 's/^Version:.*/Version: $(VERSION)/' dist/debian/control > dist/debian/nimbus/DEBIAN/control
	dpkg-deb --build dist/debian/nimbus dist/$(DEB_NAME)
	@echo ""
	@echo "=== Package built: dist/$(DEB_NAME) ==="
	@echo "Install with: sudo apt install ./dist/$(DEB_NAME)"
	@echo "     or: make install-deb"

install-deb: deb
	sudo apt install ./dist/$(DEB_NAME)

rpm: build-linux dist/rpm/nimbus.spec
	mkdir -p dist/rpm/BUILD dist/rpm/RPMS dist/rpm/SRPMS
	rpmbuild -bb --define "_topdir $(PWD)/dist/rpm" dist/rpm/nimbus.spec

clean:
	rm -rf $(BUILD_DIR) dist/debian/nimbus dist/rpm/BUILD dist/rpm/RPMS dist/rpm/SRPMS
	rm -f dist/nimbus_*.deb
