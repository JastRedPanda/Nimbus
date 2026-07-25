.PHONY: build deb install-deb rpm clean

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "1.0.0")
DEB_NAME := nimbus_$(VERSION)-1_amd64.deb

build:
	go build -ldflags="-s -w -H windowsgui" -o nimbus.exe .

build-linux:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o nimbus .

deb: build-linux
	mkdir -p dist/debian/nimbus/DEBIAN
	mkdir -p dist/debian/nimbus/usr/bin
	mkdir -p dist/debian/nimbus/usr/share/applications
	mkdir -p dist/debian/nimbus/usr/share/pixmaps
	install -m 755 nimbus dist/debian/nimbus/usr/bin/
	install -m 644 dist/nimbus.desktop dist/debian/nimbus/usr/share/applications/
	install -m 644 nimbus1.png dist/debian/nimbus/usr/share/pixmaps/nimbus.png
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
	rm -f nimbus nimbus.exe dist/nimbus_*.deb dist/rpm/RPMS/*.rpm
