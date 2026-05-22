# Power Mine 0.1.0

First preview release of Power Mine, a Go + Wails Minecraft launcher.

License: GPL-3.0.

## Included

- Profile creation, install, repair, and launch.
- Offline player mode for offline servers.
- Java runtime checks and Java installation from the launcher.
- Vanilla, Fabric, Quilt, Forge, and NeoForge foundations.
- Modrinth browsing, dependency prompts, install/delete/update flows.
- Local mod manager with bulk enable/disable.
- Launcher and game logs.
- Modrinth `.mrpack` import/export.
- Headless Codex diagnostics tool for inspecting profiles, mod jars, loader/version compatibility, and recent crash/log output.

## Installers

- `PowerMine-0.1.0-macos-amd64.dmg` is the first macOS Intel build.
- `power-mine-0.1.0-linux-x86_64.appimage` is the first Linux AppImage build.

## Codex Tool

The repository includes `tools/codex/power_mine_mcp.py`, which works both as a CLI and an MCP stdio server for Codex.

Quick checks:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod /path/to/mod.jar --profile-id <profile-id>
```

This is an early preview build. Microsoft account authentication and signed/notarized installers are planned later.
