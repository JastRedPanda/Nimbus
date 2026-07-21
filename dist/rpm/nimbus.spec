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
Source0:        %{url}/archive/v%{version}/%{repo}-%{version}.tar.gz

BuildRequires:  golang >= 1.21
BuildRequires:  gtk3-devel

%if 0%{?suse_version}
BuildRequires:  pkgconfig(gdk-pixbuf-2.0)
BuildRequires:  pkgconfig(gtk+-3.0)
%endif

Requires:       gtk3
Requires:       libappindicator-gtk3%{?_isa}

%description
Nimbus displays current temperature and weather conditions in the
system tray. Features 7-day forecast, configurable units, themes,
and language support (English/Українська).

%prep
%autosetup -n %{repo}-%{version}

%build
%ifarch x86_64
export GOARCH=amd64
%endif
export GOPATH=%{_builddir}/go
mkdir -p $GOPATH/src/%{provider}.%{provider_tld}/%{project}
ln -sf %{_builddir}/%{repo}-%{version} $GOPATH/src/%{provider}.%{provider_tld}/%{project}/%{repo}
cd $GOPATH/src/%{provider}.%{provider_tld}/%{project}/%{repo}
go build -ldflags="-s -w" -o nimbus .

%install
mkdir -p %{buildroot}%{_bindir}
install -m 755 %{_builddir}/%{repo}-%{version}/nimbus %{buildroot}%{_bindir}/nimbus

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
EOF

%files
%{_bindir}/nimbus
%{_datadir}/applications/nimbus.desktop

%changelog
* Mon Jul 21 2026 JastRedPanda <jastredpanda@users.noreply.github.com> - 1.0.0-1
- Initial release
