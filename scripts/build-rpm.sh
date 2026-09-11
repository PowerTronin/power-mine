#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

APP_ID="${APP_ID:-power-mine}"
APP_NAME="${APP_NAME:-Power Mine}"
BIN_NAME="${BIN_NAME:-power-mine}"
PACKAGE_NAME="${PACKAGE_NAME:-$APP_ID}"
TARGET="${TARGET:-linux/amd64}"
WAILS="${WAILS:-wails}"
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist}"
RPM_WORK_DIR="${RPM_WORK_DIR:-$ROOT_DIR/build/rpm}"
WAILS_BUILD_TAGS="${WAILS_BUILD_TAGS:-}"
LICENSE_ID="${LICENSE_ID:-GPL-3.0-only}"
HOMEPAGE="${HOMEPAGE:-https://github.com/PowerTronin/power-mine}"
RELEASE="${RELEASE:-1}"

version_from_source() {
  sed -nE 's/.*Version:[[:space:]]*"([^"]+)".*/\1/p' app.go | head -n 1
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command not found: %s\n' "$1" >&2
    return 1
  fi
}

rpm_arch_for_target() {
  local os="${1%%/*}"
  local arch="${1##*/}"

  if [ "$os" != "linux" ]; then
    printf 'error: .rpm builds require a linux target, got: %s\n' "$1" >&2
    return 1
  fi

  case "$arch" in
    amd64) printf 'x86_64' ;;
    arm64) printf 'aarch64' ;;
    386) printf 'i386' ;;
    *)
      printf 'error: unsupported RPM target architecture: %s\n' "$arch" >&2
      return 1
      ;;
  esac
}

validate_package_metadata() {
  if [[ ! "$PACKAGE_NAME" =~ ^[a-z0-9][a-z0-9+.-]+$ ]]; then
    printf 'error: invalid RPM package name: %s\n' "$PACKAGE_NAME" >&2
    return 1
  fi
  if [[ ! "$VERSION" =~ ^[0-9][A-Za-z0-9.+_~]*$ ]]; then
    printf 'error: invalid RPM package version: %s\n' "$VERSION" >&2
    return 1
  fi
  if [[ ! "$RELEASE" =~ ^[0-9A-Za-z][A-Za-z0-9.+_~]*$ ]]; then
    printf 'error: invalid RPM package release: %s\n' "$RELEASE" >&2
    return 1
  fi
}

detect_wails_build_tags() {
  if [ -n "$WAILS_BUILD_TAGS" ]; then
    return
  fi
  if command -v pkg-config >/dev/null 2>&1; then
    if ! pkg-config --exists webkit2gtk-4.0 2>/dev/null && pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
      WAILS_BUILD_TAGS="webkit2_41"
    fi
  fi
}

VERSION="${VERSION:-$(version_from_source)}"
VERSION="${VERSION:-0.0.0}"
RPM_ARCH="$(rpm_arch_for_target "$TARGET")"
BIN_PATH="${BINARY_PATH:-$ROOT_DIR/build/bin/$BIN_NAME}"
RPM_TOPDIR="$RPM_WORK_DIR/topdir"
SPEC_PATH="$RPM_TOPDIR/SPECS/$PACKAGE_NAME.spec"
RPM_OUTPUT="$RPM_TOPDIR/RPMS/$RPM_ARCH/$PACKAGE_NAME-$VERSION-$RELEASE.$RPM_ARCH.rpm"
OUTPUT_PATH="${OUTPUT_PATH:-$DIST_DIR/$PACKAGE_NAME-$VERSION-$RELEASE.$RPM_ARCH.rpm}"

validate_package_metadata
require_command rpmbuild
detect_wails_build_tags

if [ "${SKIP_WAILS_BUILD:-0}" != "1" ]; then
  require_command "$WAILS"
  build_args=(build -clean -platform "$TARGET")
  if [ -n "$WAILS_BUILD_TAGS" ]; then
    build_args+=(-tags "$WAILS_BUILD_TAGS")
  fi
  "$WAILS" "${build_args[@]}"
fi

if [ ! -f "$BIN_PATH" ]; then
  printf 'error: built binary not found at %s\n' "$BIN_PATH" >&2
  printf 'hint: set BINARY_PATH=/path/to/power-mine or run without SKIP_WAILS_BUILD\n' >&2
  exit 1
fi

rm -rf "$RPM_TOPDIR"
install -d -m 0755 \
  "$RPM_TOPDIR/BUILD" \
  "$RPM_TOPDIR/BUILDROOT" \
  "$RPM_TOPDIR/RPMS" \
  "$RPM_TOPDIR/SOURCES" \
  "$RPM_TOPDIR/SPECS" \
  "$RPM_TOPDIR/SRPMS" \
  "$DIST_DIR"

install -m 0755 "$BIN_PATH" "$RPM_TOPDIR/SOURCES/$BIN_NAME"
install -m 0644 "$ROOT_DIR/build/appicon.png" "$RPM_TOPDIR/SOURCES/$APP_ID.png"

if [ -f "$ROOT_DIR/LICENSE" ]; then
  install -m 0644 "$ROOT_DIR/LICENSE" "$RPM_TOPDIR/SOURCES/LICENSE"
else
  : >"$RPM_TOPDIR/SOURCES/LICENSE"
fi

cat >"$RPM_TOPDIR/SOURCES/$APP_ID.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=$APP_NAME
Exec=$BIN_NAME
Icon=$APP_ID
Categories=Game;
Terminal=false
StartupWMClass=$BIN_NAME
EOF

cat >"$SPEC_PATH" <<EOF
Name:           $PACKAGE_NAME
Version:        $VERSION
Release:        $RELEASE%{?dist}
Summary:        Power Mine Minecraft launcher
License:        $LICENSE_ID
URL:            $HOMEPAGE
Source0:        $BIN_NAME
Source1:        $APP_ID.png
Source2:        LICENSE
Source3:        $APP_ID.desktop
Requires:       ca-certificates

%description
Power Mine is a desktop launcher for Minecraft Java Edition built with Go and Wails.

%prep

%build

%install
rm -rf %{buildroot}
install -d -m 0755 %{buildroot}/opt/$APP_ID
install -d -m 0755 %{buildroot}%{_bindir}
install -d -m 0755 %{buildroot}%{_datadir}/applications
install -d -m 0755 %{buildroot}%{_datadir}/icons/hicolor/256x256/apps
install -d -m 0755 %{buildroot}%{_licensedir}/%{name}
install -m 0755 %{SOURCE0} %{buildroot}/opt/$APP_ID/$BIN_NAME
ln -s ../../opt/$APP_ID/$BIN_NAME %{buildroot}%{_bindir}/$BIN_NAME
install -m 0644 %{SOURCE1} %{buildroot}%{_datadir}/icons/hicolor/256x256/apps/$APP_ID.png
install -m 0644 %{SOURCE2} %{buildroot}%{_licensedir}/%{name}/LICENSE
install -m 0644 %{SOURCE3} %{buildroot}%{_datadir}/applications/$APP_ID.desktop

%files
/opt/$APP_ID/$BIN_NAME
%{_bindir}/$BIN_NAME
%{_datadir}/applications/$APP_ID.desktop
%{_datadir}/icons/hicolor/256x256/apps/$APP_ID.png
%license %{_licensedir}/%{name}/LICENSE
EOF

rpmbuild \
  --define "_topdir $RPM_TOPDIR" \
  --define "_dbpath $RPM_TOPDIR/rpmdb" \
  --target "$RPM_ARCH" \
  -bb "$SPEC_PATH"

if [ ! -f "$RPM_OUTPUT" ]; then
  printf 'error: RPM output not found at %s\n' "$RPM_OUTPUT" >&2
  exit 1
fi

rm -f "$OUTPUT_PATH"
cp "$RPM_OUTPUT" "$OUTPUT_PATH"
printf 'RPM package created: %s\n' "$OUTPUT_PATH"
