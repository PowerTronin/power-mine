# Power Mine

Power Mine is a Go + Wails desktop launcher for Minecraft Java Edition.

The current preview targets macOS and Linux with local profile management, offline player mode, Java runtime setup, Minecraft installation and launch, Modrinth browsing, local mod management, logs, and Modrinth `.mrpack` import/export.

## Releases

Preview builds are published on GitHub Releases.

- `PowerMine-0.1.0-macos-amd64.dmg` is the first macOS Intel installer image.
- Linux AppImage builds are published as `power-mine-<version>-linux-x86_64.appimage`.
- Local Linux packaging is available through `make appimage`; the file is written to `dist/`.

## Codex Tool

Power Mine includes a local Codex diagnostics tool for Minecraft mod development. It can inspect launcher profiles, read mod jar metadata, check loader/Minecraft compatibility, verify Fabric dependency metadata, validate models/textures/recipes, scaffold a minimal Fabric mod, and scan either recent logs or only logs from the latest launcher run.

The headless launcher path used by the Codex tool can create, install, repair, and launch Vanilla, Fabric, Quilt, Forge, and NeoForge profiles. Forge support includes legacy installers such as Forge `1.7.10-10.13.4.1614-1.7.10`, whose installer metadata is stored in `install_profile.json` instead of a separate `version.json`.

The Codex tool also has an experimental Fabric in-game agent for Minecraft `1.20.1`. The agent is a client-side mod that exposes a localhost API while a singleplayer world is loaded. Codex can use it to read player/world state, inspect inventory slots, check held-item and block render models, capture a screenshot, verify crafting grids through Minecraft's `RecipeManager`, select a hotbar slot, and place or break blocks in an integrated singleplayer world.

For repeated mod work, `agent-smoke-test` combines the static jar/profile checks with runtime agent checks in one JSON report. It can optionally launch a profile with Quick Play, wait until the agent reports a loaded world, put a mod item into the hotbar, check inventory, held item rendering, screenshot nonblank state, a concrete crafting recipe, and a block place/render/cleanup cycle.

CLI examples:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty create-profile "Codex 1.20.1" --install
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile --log-scope latest_run
python3 tools/codex/power_mine_mcp.py --pretty wait-profile-ready <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty scaffold-fabric-mod ./example-mod example_mod
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod /path/to/mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod-content /path/to/mod.jar
python3 tools/codex/power_mine_mcp.py --pretty import-mod /path/to/mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty build-agent
python3 tools/codex/power_mine_mcp.py --pretty install-agent --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty repair-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --quick-play-singleplayer "Codex World"
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --quick-play-singleplayer "Codex World"
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --jar-path /path/to/mod.jar --give-item example_mod:sample_item --recipe-items minecraft:emerald --expected-output example_mod:sample_item --block example_mod:sample_block
python3 tools/codex/power_mine_mcp.py --pretty agent-state
python3 tools/codex/power_mine_mcp.py --pretty agent-inventory
python3 tools/codex/power_mine_mcp.py --pretty agent-give-item example_mod:sample_item --count 1 --slot 0
python3 tools/codex/power_mine_mcp.py --pretty agent-held-item-render --hand main
python3 tools/codex/power_mine_mcp.py --pretty agent-screenshot
python3 tools/codex/power_mine_mcp.py --pretty agent-recipe-check "minecraft:oak_log" --width 1 --height 1 --expected-output minecraft:oak_planks
python3 tools/codex/power_mine_mcp.py --pretty agent-place-block 0 64 0 --block minecraft:stone
python3 tools/codex/power_mine_mcp.py --pretty agent-break-block 0 64 0
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
- `make appimage` creates `dist/power-mine-<version>-linux-<arch>.appimage`.
- `make agent` builds the Fabric in-game Codex agent at `agent/build/libs/power-mine-agent-0.1.0.jar`.
- `python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile` runs headless mod diagnostics for Codex/local automation.
- `python3 tools/codex/power_mine_mcp.py --pretty install-agent --profile-id <profile-id>` installs the built agent into a Fabric `1.20.1` profile.
- `python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --quick-play-singleplayer "World Name"` launches Minecraft and asks Quick Play to open an existing singleplayer world.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --quick-play-singleplayer "World Name"` runs the combined Codex runtime smoke test.
- `./build/bin/power-mine headless repair-profile <profile-id>` repairs a profile without opening the GUI.

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
3. Or publish a GitHub Release; the workflow attaches `dist/*.appimage` to that release automatically.

The design and implementation plan live under `docs/superpowers/`.
