#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GRADLE_VERSION="${GRADLE_VERSION:-8.14.3}"
TOOLS_DIR="$ROOT_DIR/build/tools"
GRADLE_DIR="$TOOLS_DIR/gradle-$GRADLE_VERSION"
GRADLE_BIN="$GRADLE_DIR/bin/gradle"

if command -v gradle >/dev/null 2>&1; then
  GRADLE_CMD=(gradle)
else
  if [[ ! -x "$GRADLE_BIN" ]]; then
    mkdir -p "$TOOLS_DIR"
    ZIP_PATH="$TOOLS_DIR/gradle-$GRADLE_VERSION-bin.zip"
    if [[ ! -f "$ZIP_PATH" ]]; then
      curl -L --fail --output "$ZIP_PATH" "https://services.gradle.org/distributions/gradle-$GRADLE_VERSION-bin.zip"
    fi
    unzip -q "$ZIP_PATH" -d "$TOOLS_DIR"
  fi
  GRADLE_CMD=("$GRADLE_BIN")
fi

"${GRADLE_CMD[@]}" -p "$ROOT_DIR/agent" --no-daemon build
