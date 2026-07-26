.PHONY: build deb install-deb rpm clean

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "1.0.0")
DEB_NAME := nimbus_$(VERSION)-1_amd64.deb
BUILD_DIR := build

build:
	mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w -H windowsgui" -o $(BUILD_DIR)/nimbus.exe .

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
