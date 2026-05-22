#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$script_dir"

dev_url="http://localhost:34115/"
frontend_url="http://localhost:5173/"

if command -v curl >/dev/null 2>&1; then
  if curl -fsS -o /dev/null "$dev_url"; then
    echo "Power Mine is already running."
    echo "Dev backend: $dev_url"
    echo "Frontend:    $frontend_url"
    exit 0
  fi
fi

if [[ -n "${WAILS_BIN:-}" ]]; then
  wails_bin="$WAILS_BIN"
elif [[ -x "/Users/zovchick/go/bin/wails" ]]; then
  wails_bin="/Users/zovchick/go/bin/wails"
else
  wails_bin="$(command -v wails || true)"
fi

if [[ -z "$wails_bin" ]]; then
  echo "Wails CLI was not found."
  echo "Install Wails v2 or set WAILS_BIN=/path/to/wails before running this launcher."
  exit 1
fi

echo "Starting Power Mine..."
echo "Project: $script_dir"
echo "Wails:   $wails_bin"
echo

exec "$wails_bin" dev
