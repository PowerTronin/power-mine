#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

APP_ID="${APP_ID:-power-mine}"
APP_NAME="${APP_NAME:-Power Mine}"
BIN_NAME="${BIN_NAME:-power-mine}"
TARGET="${TARGET:-linux/amd64}"
WAILS="${WAILS:-wails}"
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist}"
APPIMAGE_WORK_DIR="${APPIMAGE_WORK_DIR:-$ROOT_DIR/build/appimage}"
TOOLS_DIR="${TOOLS_DIR:-$ROOT_DIR/build/tools}"
WAILS_BUILD_TAGS="${WAILS_BUILD_TAGS:-}"

version_from_source() {
  sed -nE 's/.*Version:[[:space:]]*"([^"]+)".*/\1/p' app.go | head -n 1
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'error: required command not found: %s\n' "$1" >&2
    return 1
  fi
}

download_file() {
  local url="$1"
  local output="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$output"
    return
  fi

  printf 'error: curl or wget is required to download appimagetool\n' >&2
  return 1
}

appimage_arch_for_target() {
  local arch="${1##*/}"
  case "$arch" in
    amd64) printf 'x86_64' ;;
    arm64) printf 'aarch64' ;;
    386) printf 'i386' ;;
    *)
      printf 'error: unsupported AppImage target architecture: %s\n' "$arch" >&2
      return 1
      ;;
  esac
}

find_appimagetool() {
  local appimage_arch="$1"

  if [ -n "${APPIMAGETOOL:-}" ]; then
    printf '%s' "$APPIMAGETOOL"
    return
  fi
  if command -v appimagetool >/dev/null 2>&1; then
    command -v appimagetool
    return
  fi

  mkdir -p "$TOOLS_DIR"
  local tool_path="$TOOLS_DIR/appimagetool-$appimage_arch.AppImage"
  if [ ! -x "$tool_path" ]; then
    local url="${APPIMAGETOOL_URL:-https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-$appimage_arch.AppImage}"
    printf 'Downloading appimagetool from %s\n' "$url" >&2
    download_file "$url" "$tool_path"
    chmod +x "$tool_path"
  fi
  printf '%s' "$tool_path"
}

VERSION="${VERSION:-$(version_from_source)}"
VERSION="${VERSION:-0.0.0}"
APPIMAGE_ARCH="$(appimage_arch_for_target "$TARGET")"
APPDIR="$APPIMAGE_WORK_DIR/$APP_ID.AppDir"
BIN_PATH="${BINARY_PATH:-$ROOT_DIR/build/bin/$BIN_NAME}"
OUTPUT_PATH="${OUTPUT_PATH:-$DIST_DIR/PowerMine-$VERSION-linux-$APPIMAGE_ARCH.AppImage}"

if [ "${SKIP_WAILS_BUILD:-0}" != "1" ]; then
  require_command "$WAILS"
  if [ -z "$WAILS_BUILD_TAGS" ] && command -v pkg-config >/dev/null 2>&1; then
    if ! pkg-config --exists webkit2gtk-4.0 2>/dev/null && pkg-config --exists webkit2gtk-4.1 2>/dev/null; then
      WAILS_BUILD_TAGS="webkit2_41"
    fi
  fi
  build_args=(build --clean --target "$TARGET")
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

rm -rf "$APPDIR"
mkdir -p \
  "$APPDIR/usr/bin" \
  "$APPDIR/usr/share/applications" \
  "$APPDIR/usr/share/icons/hicolor/256x256/apps" \
  "$DIST_DIR"

cp "$BIN_PATH" "$APPDIR/usr/bin/$BIN_NAME"
chmod +x "$APPDIR/usr/bin/$BIN_NAME"

cp "$ROOT_DIR/build/appicon.png" "$APPDIR/$APP_ID.png"
cp "$ROOT_DIR/build/appicon.png" "$APPDIR/usr/share/icons/hicolor/256x256/apps/$APP_ID.png"

cat >"$APPDIR/AppRun" <<EOF
#!/bin/sh
HERE="\$(dirname "\$(readlink -f "\$0")")"
export PATH="\$HERE/usr/bin:\$PATH"
exec "\$HERE/usr/bin/$BIN_NAME" "\$@"
EOF
chmod +x "$APPDIR/AppRun"

cat >"$APPDIR/$APP_ID.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=$APP_NAME
Exec=$BIN_NAME
Icon=$APP_ID
Categories=Game;
Terminal=false
StartupWMClass=$BIN_NAME
EOF
cp "$APPDIR/$APP_ID.desktop" "$APPDIR/usr/share/applications/$APP_ID.desktop"

APPIMAGETOOL_PATH="$(find_appimagetool "$APPIMAGE_ARCH")"
rm -f "$OUTPUT_PATH"

export ARCH="$APPIMAGE_ARCH"
export APPIMAGE_EXTRACT_AND_RUN="${APPIMAGE_EXTRACT_AND_RUN:-1}"
"$APPIMAGETOOL_PATH" "$APPDIR" "$OUTPUT_PATH"
chmod +x "$OUTPUT_PATH"

printf 'AppImage created: %s\n' "$OUTPUT_PATH"
