# Power Mine

Power Mine is a Go + Wails desktop launcher for Minecraft Java Edition.

The current preview targets macOS and Linux with local profile management, offline player mode, Java runtime setup, Minecraft installation and launch, Modrinth browsing, local mod management, logs, and Modrinth `.mrpack` import/export.

## Releases

Preview builds are published on GitHub Releases.

- `PowerMine-0.1.0-macos-amd64.dmg` is the first macOS Intel installer image.
- Linux AppImage builds are published as `power-mine-<version>-linux-x86_64.appimage`.
- Local Linux packaging is available through `make appimage`; the file is written to `dist/`.

## Codex Tool

Power Mine includes a local Codex diagnostics tool for Minecraft mod development. It can inspect launcher profiles, read mod jar metadata, check loader/Minecraft compatibility, verify Fabric dependency metadata, validate models/textures/recipes, scaffold minimal Fabric `1.20.1` and Forge `1.7.10` mods, build local mod projects, and scan either recent logs or only logs from the latest launcher run.

The headless launcher path used by the Codex tool can create, install, repair, and launch Vanilla, Fabric, Quilt, Forge, and NeoForge profiles. Forge support includes legacy installers such as Forge `1.7.10-10.13.4.1614-1.7.10`, whose installer metadata is stored in `install_profile.json` instead of a separate `version.json`.

The Codex tool also has an experimental in-game bridge protocol exposed by client-side agent mods. The Fabric `1.20.1` agent currently has the broadest runtime coverage: Codex can open an existing local world, read player/world state, inspect inventory slots, wait for world ticks, use/right-click held items and blocks, run structured item/block interaction checks, check held-item and block baked render models, rotate the camera/head and capture screenshots, verify crafting grids through Minecraft's `RecipeManager`, perform a runtime inventory craft by consuming inputs and inserting the output, select a hotbar slot, and place or break blocks in an integrated singleplayer world. Separate Forge `1.7.10` and Forge `1.12.2` agents are available for legacy packs; they support the same bridge metadata endpoint plus state, inventory, give-item, hotbar select, world open/create, world tick waiting, item/block right-click, structured interaction checks, world snapshot, camera rotation, screenshot capture, screenshot-based held-item and block visual probes, runtime recipe checks, runtime inventory craft, place block, and break block. Forge held-item and block checks are visual screenshot probes, not modern baked-model introspection.

For repeated mod work, `mod-dev-loop` builds a local mod project, finds the newest jar, diagnoses it, imports it into a profile, installs the matching runtime agent, launches Minecraft, opens or creates a test world, runs the runtime smoke test, and stops the launched process unless `--keep-running` is passed. `agent-smoke-test` is the lower-level runtime report: it can optionally launch a profile with Quick Play or ask the running agent to open/create a world, wait until the agent reports a loaded world, verify that world ticks advance, release captured input, put a mod item into the hotbar, check inventory, held item rendering, screenshot nonblank state, a concrete crafting recipe, optional real inventory craft, item/block right-click behavior, and a block place/render/cleanup cycle. Runtime craft uses the recipe system and player inventory directly; it does not automate mouse clicks in the crafting GUI.

Headless launches for Codex automation force `pauseOnLostFocus:false` in the profile's `options.txt` before starting Minecraft, so singleplayer worlds keep ticking when Codex or the user switches to another window. Pass `--keep-pause-on-lost-focus` to preserve Minecraft's existing option. A running agent can also handle `agent-release-input` to release captured mouse input and keep pause-on-lost-focus disabled.

CLI examples:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty create-profile "Codex 1.20.1" --install
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile --log-scope latest_run
python3 tools/codex/power_mine_mcp.py --pretty wait-profile-ready <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty scaffold-fabric-mod ./example-mod example_mod
python3 tools/codex/power_mine_mcp.py --pretty scaffold-forge-1.7.10-mod ./example-forge-mod example_forge_mod
python3 tools/codex/power_mine_mcp.py --pretty build-mod-project ./example-forge-mod
python3 tools/codex/power_mine_mcp.py --pretty mod-dev-loop ./example-forge-mod --profile-id <profile-id> --world-name "Codex Dev World"
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod /path/to/mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod-content /path/to/mod.jar
python3 tools/codex/power_mine_mcp.py --pretty import-mod /path/to/mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty build-agent
python3 tools/codex/power_mine_mcp.py --pretty build-agent --target forge-1.7.10
python3 tools/codex/power_mine_mcp.py --pretty build-agent --target forge-1.12.2
python3 tools/codex/power_mine_mcp.py --pretty install-agent --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty repair-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --quick-play-singleplayer "Codex World"
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --keep-pause-on-lost-focus
python3 tools/codex/power_mine_mcp.py --pretty agent-world-open "Codex World" --create
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --world-name "Codex World" --create-world
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --quick-play-singleplayer "Codex World"
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --jar-path /path/to/mod.jar --give-item example_mod:sample_item --recipe-items minecraft:emerald --expected-output example_mod:sample_item --block example_mod:sample_block
python3 tools/codex/power_mine_mcp.py --pretty agent-capabilities
python3 tools/codex/power_mine_mcp.py --pretty agent-state
python3 tools/codex/power_mine_mcp.py --pretty agent-inventory
python3 tools/codex/power_mine_mcp.py --pretty agent-release-input
python3 tools/codex/power_mine_mcp.py --pretty agent-give-item example_mod:sample_item --count 1 --slot 0
python3 tools/codex/power_mine_mcp.py --pretty agent-wait-ticks 20
python3 tools/codex/power_mine_mcp.py --pretty agent-use-item --slot 0
python3 tools/codex/power_mine_mcp.py --pretty agent-use-block 0 64 0 --side up
python3 tools/codex/power_mine_mcp.py --pretty agent-interaction-check block --x 0 --y 64 --z 0 --expect-used
python3 tools/codex/power_mine_mcp.py --pretty agent-held-item-render --hand main
python3 tools/codex/power_mine_mcp.py --pretty agent-block-render --x 0 --y 64 --z 0
python3 tools/codex/power_mine_mcp.py --pretty agent-screenshot
python3 tools/codex/power_mine_mcp.py --pretty agent-camera-look --x 0 --y 64 --z 0 --screenshot
python3 tools/codex/power_mine_mcp.py --pretty agent-recipe-check "minecraft:oak_log" --width 1 --height 1 --expected-output minecraft:oak_planks
python3 tools/codex/power_mine_mcp.py --pretty agent-recipe-check "minecraft:log" --width 1 --height 1 --expected-output minecraft:planks
python3 tools/codex/power_mine_mcp.py --pretty agent-craft-recipe "minecraft:oak_log" --width 1 --height 1 --expected-output minecraft:oak_planks
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --recipe-items minecraft:oak_log --expected-output minecraft:oak_planks --craft-recipe
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
- `make agent` builds the Fabric `1.20.1` in-game Codex agent at `agent/build/libs/power-mine-agent-0.1.0.jar`.
- `make agent-forge` builds the Forge `1.7.10` in-game Codex agent at `forge-agent/build/libs/power-mine-forge-1.7.10-agent-0.1.0.jar`.
- `make agent-forge-1122` builds the Forge `1.12.2` in-game Codex agent at `forge-1122-agent/build/libs/power-mine-forge-1.12.2-agent-0.1.0.jar`.
- `make agent-all` builds all runtime agents.
- `python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile` runs headless mod diagnostics for Codex/local automation.
- `python3 tools/codex/power_mine_mcp.py --pretty install-java 8` installs the managed Java 8 runtime used by legacy Minecraft and Forge profiles.
- `python3 tools/codex/power_mine_mcp.py --pretty install-agent --profile-id <profile-id>` installs the matching built agent into a supported Fabric `1.20.1`, Forge `1.7.10`, or Forge `1.12.2` profile.
- `python3 tools/codex/power_mine_mcp.py --pretty scaffold-forge-1.7.10-mod ./example-forge-mod example_forge_mod` creates a minimal Forge `1.7.10` mod that builds against an installed Power Mine Forge profile without requiring ForgeGradle.
- `python3 tools/codex/power_mine_mcp.py --pretty mod-dev-loop ./example-forge-mod --profile-id <profile-id> --world-name "World Name"` builds, imports, launches, opens a world, checks sample item/recipe/craft/block runtime behavior, and stops the launched Minecraft process.
- `python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --quick-play-singleplayer "World Name"` launches Minecraft, forces `pauseOnLostFocus:false`, and asks Quick Play to open an existing singleplayer world.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-world-open "World Name" --create` asks the running agent to open/create a singleplayer world. Forge `1.7.10` and Forge `1.12.2` can create; Fabric `1.20.1` currently opens existing worlds only.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-release-input` asks the running agent to release captured mouse input and disable pause-on-lost-focus at runtime.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-wait-ticks 20` verifies that the loaded world advances game ticks.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-camera-look --yaw 180 --pitch 20 --screenshot` rotates the player camera/head and captures a screenshot after the frame updates.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-use-block 0 64 0 --side up` right-clicks a block through the running agent.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-craft-recipe "minecraft:log" --width 1 --height 1 --expected-output minecraft:planks` asks the running agent to craft through the runtime recipe system and player inventory.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-interaction-check item --item example_mod:sample_item --require-effect` gives/selects an item, right-clicks it, waits ticks, and reports observed screen, stack, and effect changes.
- `python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --world-name "World Name" --create-world` runs the combined Codex runtime smoke test and asks the agent to enter the world after launch.
- `./build/bin/power-mine headless repair-profile <profile-id>` repairs a profile without opening the GUI.

Forge and NeoForge profile installation automatically prepares the required managed Java runtime when it is missing. Legacy Minecraft versions such as `1.7.10` require Java 8, so the first install may download Temurin 8 before running the Forge installer.

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
