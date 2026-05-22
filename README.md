# Power Mine

Power Mine is a Go + Wails desktop launcher for Minecraft Java Edition.

The current preview targets macOS and Linux with local profile management, offline player mode, Java runtime setup, Minecraft installation and launch, Modrinth browsing, local mod management, logs, and Modrinth `.mrpack` import/export.

## Releases

Preview builds are published on GitHub Releases.

- `PowerMine-0.1.0-macos-amd64.dmg` is the first macOS Intel installer image.
- Linux AppImage builds are published as `PowerMine-<version>-linux-x86_64.AppImage`.
- Local Linux packaging is available through `make appimage`; the file is written to `dist/`.

## Codex Tool

Power Mine includes a local Codex diagnostics tool for Minecraft mod development. It can inspect launcher profiles, read mod jar metadata, check loader and Minecraft version compatibility, and scan recent logs/crash reports for common mod failures.

CLI examples:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod /path/to/mod.jar --profile-id <profile-id>
```

The same script can run as an MCP server for Codex:

```bash
python3 tools/codex/power_mine_mcp.py mcp
```

On this development machine it is also installed as the local Codex plugin `power-mine@personal`.

## License

Power Mine is licensed under the GNU General Public License v3.0. See [LICENSE](LICENSE).

## Development

Requirements:

- Go
- Node.js and npm
- Wails v2 CLI

Useful commands:

- `./start-power-mine.sh` starts the launcher from macOS/Linux terminal.
- Double-click `Start Power Mine.command` on macOS to start the launcher.
- `make launch` runs the same local launcher script.
- `go test ./...` runs backend tests.
- `npm --prefix frontend run build` builds the frontend.
- `wails dev` starts the desktop app in development mode.
- `wails build` creates a production build.
- `make appimage` creates `dist/PowerMine-<version>-linux-<arch>.AppImage`.
- `python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile` runs headless mod diagnostics for Codex/local automation.

On Ubuntu 24.04 or another distro that provides WebKitGTK 4.1 instead of 4.0, install `libwebkit2gtk-4.1-dev`; `make appimage` will automatically pass the Wails build tag `webkit2_41` when that package is detected.

## Release Checklist

1. Update the version in `app.go`.
2. Run `go test ./...`.
3. Run `npm --prefix frontend run build`.
4. Run `wails build`.
5. Run `make appimage` on Linux.
6. Upload the generated file from `dist/` as a Linux release asset.

GitHub Actions can build the Linux AppImage without a local Go/Wails setup:

1. Open **Actions**.
2. Run **Linux AppImage** manually to get the `power-mine-linux-appimage` artifact.
3. Or publish a GitHub Release; the workflow attaches `dist/*.AppImage` to that release automatically.

The design and implementation plan live under `docs/superpowers/`.
