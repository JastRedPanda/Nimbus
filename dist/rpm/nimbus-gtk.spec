%global provider       github
%global provider_tld   com
%global project        JastRedPanda
%global repo           Nimbus

Name:           nimbus-gtk
Version:        1.0.0
Release:        1%{?dist}
Summary:        Weather tray app with 7-day forecast and settings GUI (GTK)

License:        GPL-3.0-or-later
URL:            https://github.com/JastRedPanda/Nimbus

# A soname dependency rather than a package name, because this one spec has to
# install on Fedora, RHEL and openSUSE, and the three call the GTK package three
# different things. Every rpm distribution auto-provides the sonames its
# libraries carry, so this resolves everywhere without naming anyone's package.
#
# Requires and not Recommends, unlike the single package this replaced: a build
# named -gtk that quietly opens its windows in a browser because GTK is missing
# would be worth a bug report rather than a shrug. The Qt build states the same
# thing about Qt.
Requires:       libgtk-3.so.0()(64bit)

# The rename dance. This package IS the old nimbus - same binary, same windows -
# so an existing installation has to be upgraded rather than left behind owning
# /usr/bin/nimbus forever. nimbus-qt deliberately carries none of this: it is a
# new thing that installs alongside, not a replacement for anything.
Provides:       nimbus = %{version}-%{release}
Obsoletes:      nimbus < %{version}-%{release}

%description
Nimbus displays current temperature and weather conditions in the
system tray. Features 7-day forecast, configurable units, themes,
and language support (English/Українська).

This build draws its windows with GTK 3. For a Qt desktop install
nimbus-qt instead; the two can be installed side by side and share
one configuration file.

%prep
mkdir -p build-rpm

%build
echo "Binary built separately in workflow"

%install
mkdir -p %{buildroot}%{_bindir}
install -m 755 build-rpm/nimbus-gtk %{buildroot}%{_bindir}/nimbus-gtk

mkdir -p %{buildroot}%{_datadir}/applications
cat > %{buildroot}%{_datadir}/applications/nimbus-gtk.desktop << EOF
[Desktop Entry]
Type=Application
Name=Nimbus (GTK)
Comment=Weather tray app
Exec=nimbus-gtk
Terminal=false
Categories=Utility;
StartupNotify=false
Icon=nimbus-gtk
EOF

mkdir -p %{buildroot}%{_datadir}/pixmaps
install -m 644 build-rpm/nimbus1.png %{buildroot}%{_datadir}/pixmaps/nimbus-gtk.png
install -m 644 build-rpm/LICENSE build-rpm/THIRD-PARTY-LICENSES .

%files
%license LICENSE
%doc THIRD-PARTY-LICENSES
%{_bindir}/nimbus-gtk
%{_datadir}/applications/nimbus-gtk.desktop
%{_datadir}/pixmaps/nimbus-gtk.png

%changelog
* Mon Jul 21 2026 JastRedPanda <jastredpanda@users.noreply.github.com> - 1.0.0-1
- Initial release