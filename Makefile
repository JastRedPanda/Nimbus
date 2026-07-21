.PHONY: build deb rpm clean

build:
	go build -ldflags="-s -w -H windowsgui" -o nimbus.exe .

build-linux:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o nimbus .

deb: build-linux
	mkdir -p dist/debian/nimbus/DEBIAN
	mkdir -p dist/debian/nimbus/usr/bin
	mkdir -p dist/debian/nimbus/usr/share/applications
	install -m 755 nimbus dist/debian/nimbus/usr/bin/
	install -m 644 dist/nimbus.desktop dist/debian/nimbus/usr/share/applications/
	install -m 644 dist/debian/control dist/debian/nimbus/DEBIAN/
	dpkg-deb --build dist/debian/nimbus dist/nimbus_1.0.0-1_amd64.deb

rpm: build-linux dist/rpm/nimbus.spec
	mkdir -p dist/rpm/BUILD dist/rpm/RPMS dist/rpm/SRPMS
	rpmbuild -bb --define "_topdir $(PWD)/dist/rpm" dist/rpm/nimbus.spec

clean:
	rm -f nimbus nimbus.exe dist/nimbus_*.deb dist/rpm/RPMS/*.rpm
