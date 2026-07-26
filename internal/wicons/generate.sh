#!/usr/bin/env bash
# Regenerates the PNGs committed in icons32/, icons64/ and icons128/ from the
# upstream Meteocons static SVG export. Not part of the Go build - the PNGs
# are committed so a normal `go build` needs neither network access nor
# rsvg-convert. Run this only when changing the icon set or adding a code.
#
# Source: @meteocons/svg-static@0.1.0 on npm (MIT, see THIRD-PARTY-LICENSES).
# Pinned by tarball hash so a re-run is reproducible even after upstream
# publishes a newer version.
set -euo pipefail

TARBALL_URL="https://registry.npmjs.org/@meteocons/svg-static/-/svg-static-0.1.0.tgz"
TARBALL_SHA256="7aead065bfd18127392dd39a67f18c30580e45ddb2facef893839caebf0d029b"
LICENSE_SHA256="2c7646145091766e97569d338a2266684c2c9e5fa7093764ef2d2dd620825c08"

# Every icon file this package's name() function references. Keep in sync by
# hand - there is no manifest-driven build here, on purpose: 22 named files is
# easy to audit, a generated list is not.
STEMS=(
  clear-day mostly-clear-day partly-cloudy-day overcast
  fog-day extreme-day-fog
  partly-cloudy-day-drizzle overcast-day-drizzle extreme-day-drizzle
  partly-cloudy-day-sleet extreme-day-sleet overcast-day-sleet
  partly-cloudy-day-rain overcast-day-rain extreme-day-rain
  partly-cloudy-day-snow overcast-day-snow extreme-day-snow
  thunderstorms-day-rain thunderstorms-day-hail extreme-thunderstorms-day-hail
  cloudy
)

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

curl -sSL -o "$work/svg-static.tgz" "$TARBALL_URL"
got_sha="$(sha256sum "$work/svg-static.tgz" | cut -d' ' -f1)"
if [ "$got_sha" != "$TARBALL_SHA256" ]; then
  echo "generate.sh: tarball sha256 mismatch: got $got_sha, want $TARBALL_SHA256" >&2
  exit 1
fi
tar -xzf "$work/svg-static.tgz" -C "$work"

got_license_sha="$(sha256sum "$work/package/LICENSE" | cut -d' ' -f1)"
if [ "$got_license_sha" != "$LICENSE_SHA256" ]; then
  echo "generate.sh: LICENSE sha256 mismatch: got $got_license_sha, want $LICENSE_SHA256 - check THIRD-PARTY-LICENSES is still accurate" >&2
  exit 1
fi

for size in 32 64 128; do
  mkdir -p "$here/icons$size"
  rm -f "$here/icons$size"/*.png
  for stem in "${STEMS[@]}"; do
    rsvg-convert -w "$size" -h "$size" -f png \
      -o "$here/icons$size/$stem.png" "$work/package/fill/$stem.svg"
  done
done

echo "generate.sh: wrote $((${#STEMS[@]} * 3)) PNGs into $here/icons{32,64,128}"
