#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DATA_DIR="${POWER_MINE_DATA_DIR:-$HOME/.local/share/power-mine}"
FORGE_VERSION="${POWER_MINE_FORGE_1122_VERSION:-1.12.2-14.23.5.2860}"
FORGE_JAR="${POWER_MINE_FORGE_1122_JAR:-$DATA_DIR/minecraft/libraries/net/minecraftforge/forge/$FORGE_VERSION/forge-$FORGE_VERSION.jar}"
FORGE_INSTALLER_JAR="$DATA_DIR/minecraft/libraries/net/minecraftforge/forge/$FORGE_VERSION/forge-$FORGE_VERSION-installer.jar"
GSON_JAR="${POWER_MINE_GSON_1122_JAR:-}"
SRC_DIR="$ROOT_DIR/forge-1122-agent/src/main/java"
RES_DIR="$ROOT_DIR/forge-1122-agent/src/main/resources"
BUILD_DIR="$ROOT_DIR/forge-1122-agent/build"
CLASS_DIR="$BUILD_DIR/classes"
JAR_PATH="$BUILD_DIR/libs/power-mine-forge-1.12.2-agent-0.2.0.jar"
JAVAC_BIN="${JAVAC:-javac}"
JAR_BIN="${JAR:-jar}"

if [[ -z "$GSON_JAR" ]]; then
  GSON_JAR="$(find "$DATA_DIR/minecraft/libraries/com/google/code/gson/gson" -type f -name 'gson-*.jar' 2>/dev/null | sort -V | tail -1 || true)"
fi

if [[ ! -f "$FORGE_JAR" && -f "$FORGE_INSTALLER_JAR" ]]; then
  mkdir -p "$(dirname "$FORGE_JAR")"
  unzip -p "$FORGE_INSTALLER_JAR" "maven/net/minecraftforge/forge/$FORGE_VERSION/forge-$FORGE_VERSION.jar" > "$FORGE_JAR"
fi

if [[ ! -f "$FORGE_JAR" ]]; then
  echo "Forge jar not found: $FORGE_JAR" >&2
  echo "Install or repair a Forge 1.12.2 profile first, or set POWER_MINE_FORGE_1122_JAR." >&2
  exit 1
fi

if [[ ! -f "$GSON_JAR" ]]; then
  echo "Gson jar not found under $DATA_DIR/minecraft/libraries/com/google/code/gson/gson." >&2
  echo "Install or repair a Minecraft profile first, or set POWER_MINE_GSON_1122_JAR." >&2
  exit 1
fi

rm -rf "$CLASS_DIR"
mkdir -p "$CLASS_DIR" "$(dirname "$JAR_PATH")"

mapfile -t SOURCES < <(find "$SRC_DIR" -name '*.java' -print | sort)
if [[ ${#SOURCES[@]} -eq 0 ]]; then
  echo "No Forge 1.12.2 agent sources found in $SRC_DIR" >&2
  exit 1
fi

if "$JAVAC_BIN" --release 8 -version >/dev/null 2>&1; then
  "$JAVAC_BIN" --release 8 -cp "$FORGE_JAR:$GSON_JAR" -d "$CLASS_DIR" "${SOURCES[@]}"
else
  "$JAVAC_BIN" -source 1.8 -target 1.8 -cp "$FORGE_JAR:$GSON_JAR" -d "$CLASS_DIR" "${SOURCES[@]}"
fi

if [[ -d "$RES_DIR" ]]; then
  cp -R "$RES_DIR"/. "$CLASS_DIR"/
fi

"$JAR_BIN" cf "$JAR_PATH" -C "$CLASS_DIR" .
echo "Forge 1.12.2 agent created: $JAR_PATH"
