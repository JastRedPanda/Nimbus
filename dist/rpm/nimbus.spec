%global provider       github
%global provider_tld   com
%global project        JastRedPanda
%global repo           Nimbus

Name:           nimbus
Version:        1.0.0
Release:        1%{?dist}
Summary:        Weather tray app with 7-day forecast and settings GUI

License:        GPL-3.0-or-later
URL:            https://github.com/JastRedPanda/Nimbus

# No build dependency on GTK: the binary is pure Go and loads the library at
# runtime with dlopen, so no headers and no pkg-config are involved.
#
# GTK is Recommends rather than Requires for the same reason - without it the
# app still installs, still shows its tray icon, and opens its windows in the
# browser instead. appindicator is gone entirely: the tray speaks D-Bus now.
Recommends:     gtk3

%description
Nimbus displays current temperature and weather conditions in the
system tray. Features 7-day forecast, configurable units, themes,
and language support (English/Українська).

%prep
mkdir -p build-rpm

%build
echo "Binary built separately in workflow"

%install
mkdir -p %{buildroot}%{_bindir}
install -m 755 build-rpm/nimbus %{buildroot}%{_bindir}/nimbus

mkdir -p %{buildroot}%{_datadir}/applications
cat > %{buildroot}%{_datadir}/applications/nimbus.desktop << EOF
[Desktop Entry]
Type=Application
Name=Nimbus
Comment=Weather tray app
Exec=nimbus
Terminal=false
Categories=Utility;
StartupNotify=false
Icon=nimbus
EOF

mkdir -p %{buildroot}%{_datadir}/pixmaps
install -m 644 build-rpm/nimbus1.png %{buildroot}%{_datadir}/pixmaps/nimbus.png
install -m 644 build-rpm/LICENSE build-rpm/THIRD-PARTY-LICENSES .

%files
%license LICENSE
%doc THIRD-PARTY-LICENSES
%{_bindir}/nimbus
%{_datadir}/applications/nimbus.desktop
%{_datadir}/pixmaps/nimbus.png

%changelog
* Mon Jul 21 2026 JastRedPanda <jastredpanda@users.noreply.github.com> - 1.0.0-1
- Initial release