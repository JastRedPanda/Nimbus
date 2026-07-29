%global provider       github
%global provider_tld   com
%global project        JastRedPanda
%global repo           Nimbus

Name:           nimbus-qt
Version:        1.0.0
Release:        1%{?dist}
Summary:        Weather tray app with 7-day forecast and settings GUI (Qt)

License:        GPL-3.0-or-later
URL:            https://github.com/JastRedPanda/Nimbus

# A soname dependency rather than a package name - see the sibling spec for why.
# Nothing about Qt is needed to BUILD this package: the Qt half is a shared
# object compiled separately and embedded in the binary, which loads it from
# memory at startup.
Requires:       libQt6Widgets.so.6()(64bit)

%description
Nimbus displays current temperature and weather conditions in the
system tray. Features 7-day forecast, configurable units, themes,
and language support (English/Українська).

This build draws its windows with Qt 6 and contains no GTK code at
all. For a GTK desktop install nimbus-gtk instead; the two can be
installed side by side and share one configuration file.

%prep
mkdir -p build-rpm

%build
echo "Binary built separately in workflow"

%install
mkdir -p %{buildroot}%{_bindir}
install -m 755 build-rpm/nimbus-qt %{buildroot}%{_bindir}/nimbus-qt

mkdir -p %{buildroot}%{_datadir}/applications
cat > %{buildroot}%{_datadir}/applications/nimbus-qt.desktop << EOF
[Desktop Entry]
Type=Application
Name=Nimbus (Qt)
Comment=Weather tray app
Exec=nimbus-qt
Terminal=false
Categories=Utility;
StartupNotify=false
Icon=nimbus-qt
EOF

mkdir -p %{buildroot}%{_datadir}/pixmaps
install -m 644 build-rpm/nimbus1.png %{buildroot}%{_datadir}/pixmaps/nimbus-qt.png
install -m 644 build-rpm/LICENSE build-rpm/THIRD-PARTY-LICENSES .

%files
%license LICENSE
%doc THIRD-PARTY-LICENSES
%{_bindir}/nimbus-qt
%{_datadir}/applications/nimbus-qt.desktop
%{_datadir}/pixmaps/nimbus-qt.png

%changelog
* Mon Jul 21 2026 JastRedPanda <jastredpanda@users.noreply.github.com> - 1.0.0-1
- Initial release