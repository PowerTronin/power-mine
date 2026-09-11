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
DEB_WORK_DIR="${DEB_WORK_DIR:-$ROOT_DIR/build/deb}"
WAILS_BUILD_TAGS="${WAILS_BUILD_TAGS:-}"
MAINTAINER="${MAINTAINER:-Power Mine <power-mine@example.invalid>}"
HOMEPAGE="${HOMEPAGE:-https://github.com/PowerTronin/power-mine}"

version_from_source() {
  sed -nE 's/.*Version:[[:space:]]*"([^"]+)".*/\1/p' app.go | head -n 1
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command not found: %s\n' "$1" >&2
    return 1
  fi
}

deb_arch_for_target() {
  local os="${1%%/*}"
  local arch="${1##*/}"

  if [ "$os" != "linux" ]; then
    printf 'error: .deb builds require a linux target, got: %s\n' "$1" >&2
    return 1
  fi

  case "$arch" in
    amd64) printf 'amd64' ;;
    arm64) printf 'arm64' ;;
    386) printf 'i386' ;;
    *)
      printf 'error: unsupported Debian target architecture: %s\n' "$arch" >&2
      return 1
      ;;
  esac
}

validate_package_metadata() {
  if [[ ! "$PACKAGE_NAME" =~ ^[a-z0-9][a-z0-9+.-]+$ ]]; then
    printf 'error: invalid Debian package name: %s\n' "$PACKAGE_NAME" >&2
    return 1
  fi
  if [[ ! "$VERSION" =~ ^[0-9][A-Za-z0-9.+:~-]*$ ]]; then
    printf 'error: invalid Debian package version: %s\n' "$VERSION" >&2
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

runtime_depends() {
  local depends
  case " $WAILS_BUILD_TAGS " in
    *webkit2_41*)
      depends="libgtk-3-0, libwebkit2gtk-4.1-0, libjavascriptcoregtk-4.1-0, ca-certificates"
      ;;
    *)
      depends="libgtk-3-0, libwebkit2gtk-4.0-37, libjavascriptcoregtk-4.0-18, ca-certificates"
      ;;
  esac

  if [ -n "${EXTRA_DEPENDS:-}" ]; then
    depends="$depends, $EXTRA_DEPENDS"
  fi
  printf '%s' "$depends"
}

VERSION="${VERSION:-$(version_from_source)}"
VERSION="${VERSION:-0.0.0}"
DEB_ARCH="$(deb_arch_for_target "$TARGET")"
BIN_PATH="${BINARY_PATH:-$ROOT_DIR/build/bin/$BIN_NAME}"
OUTPUT_PATH="${OUTPUT_PATH:-$DIST_DIR/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}.deb}"
DEB_ROOT="$DEB_WORK_DIR/${PACKAGE_NAME}_${VERSION}_${DEB_ARCH}"

validate_package_metadata
require_command dpkg-deb
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

rm -rf "$DEB_ROOT"
install -d -m 0755 \
  "$DEB_ROOT/DEBIAN" \
  "$DEB_ROOT/opt/$APP_ID" \
  "$DEB_ROOT/usr/bin" \
  "$DEB_ROOT/usr/share/applications" \
  "$DEB_ROOT/usr/share/doc/$PACKAGE_NAME" \
  "$DEB_ROOT/usr/share/icons/hicolor/256x256/apps" \
  "$DIST_DIR"

install -m 0755 "$BIN_PATH" "$DEB_ROOT/opt/$APP_ID/$BIN_NAME"
ln -s "/opt/$APP_ID/$BIN_NAME" "$DEB_ROOT/usr/bin/$BIN_NAME"
install -m 0644 "$ROOT_DIR/build/appicon.png" "$DEB_ROOT/usr/share/icons/hicolor/256x256/apps/$APP_ID.png"

if [ -f "$ROOT_DIR/LICENSE" ]; then
  install -m 0644 "$ROOT_DIR/LICENSE" "$DEB_ROOT/usr/share/doc/$PACKAGE_NAME/copyright"
fi

cat >"$DEB_ROOT/usr/share/applications/$APP_ID.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=$APP_NAME
Exec=$BIN_NAME
Icon=$APP_ID
Categories=Game;
Terminal=false
StartupWMClass=$BIN_NAME
EOF
chmod 0644 "$DEB_ROOT/usr/share/applications/$APP_ID.desktop"

cat >"$DEB_ROOT/DEBIAN/control" <<EOF
Package: $PACKAGE_NAME
Version: $VERSION
Section: games
Priority: optional
Architecture: $DEB_ARCH
Maintainer: $MAINTAINER
Depends: $(runtime_depends)
Homepage: $HOMEPAGE
Description: Power Mine Minecraft launcher
 Power Mine is a desktop launcher for Minecraft Java Edition built with Go and Wails.
EOF

chmod 0644 "$DEB_ROOT/DEBIAN/control"
rm -f "$OUTPUT_PATH"
dpkg-deb --build --root-owner-group "$DEB_ROOT" "$OUTPUT_PATH"

printf 'Debian package created: %s\n' "$OUTPUT_PATH"
