# Power Mine

Power Mine is a Go + Wails desktop launcher for Minecraft Java Edition.

The current preview targets macOS and Linux with local profile management, offline player mode, Java runtime setup, Minecraft installation and launch, Modrinth browsing, local mod management, logs, and Modrinth `.mrpack` import/export.

## Releases

Preview builds are published on GitHub Releases.

- `PowerMine-0.1.0-macos-amd64.dmg` is the first macOS Intel installer image.
- Linux packaging is planned after the first release workflow is stable.

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

## Release Checklist

1. Update the version in `app.go`.
2. Run `go test ./...`.
3. Run `npm --prefix frontend run build`.
4. Run `wails build`.
5. Package `build/bin/power-mine.app` into a release asset.

The design and implementation plan live under `docs/superpowers/`.
