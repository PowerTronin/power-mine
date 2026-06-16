# Power Mine Codex Diagnostics

`power_mine_mcp.py` is a standard-library Python diagnostics tool for Codex and local automation.

It can:

- list Power Mine profiles;
- create launcher profiles through the headless launcher;
- inspect Fabric, Quilt, Forge, and NeoForge mod jars;
- compare mod metadata with a launcher profile;
- verify installed Fabric `depends`/`breaks` metadata, including nested Fabric API modules;
- statically validate item/block models, textures, blockstates, and crafting recipe JSON in mod jars;
- scan recent logs, crash reports, JVM error logs, or only files from the latest launcher run;
- wait for launched-profile ready/crash signals from logs;
- scaffold minimal buildable Fabric `1.20.1` and Forge `1.7.10` mod projects;
- build a local mod project and find the newest jar artifact;
- import, enable, disable, and delete profile mod jars;
- call the launcher headless commands to install, repair, or launch a profile;
- create and operate Vanilla, Fabric, Quilt, Forge, and NeoForge launcher profiles, including legacy Forge `1.7.10` installers;
- build and install the experimental in-game bridge agents for Fabric `1.20.1` and Forge `1.7.10`;
- ask a running agent to open or create a local singleplayer test world when that agent supports it;
- query the running agent for player/world state and inventory;
- release captured mouse input and disable pause-on-lost-focus during automation sessions;
- put an item stack into a hotbar/inventory slot before render checks;
- wait for a loaded world to advance game ticks;
- use/right-click held items and block positions through the running agent;
- run structured item/block interaction checks with optional expectations for used result, screen open, stack changes, block changes, or any observed effect;
- ask a capable running client whether held items and placed blocks have baked render models or screenshot visual-probe evidence;
- capture a framebuffer screenshot to a PNG file for visual inspection when the agent supports it;
- ask the running Minecraft client whether a crafting grid really matches a recipe when the agent supports it;
- ask the running agent to perform a recipe craft by consuming player inventory inputs and inserting the output stack;
- place or break blocks in a loaded integrated singleplayer world through the agent;
- run one combined static/runtime smoke test that returns a structured pass/warn/fail report;
- run a complete mod dev loop that builds a project, imports the jar, installs the matching agent, launches the profile, opens a world, performs runtime checks, and stops the launched process by default;
- run as an MCP stdio server for Codex plugin use.

Examples:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty create-profile "Codex 1.20.1" --install
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile --log-scope latest_run
python3 tools/codex/power_mine_mcp.py --pretty wait-profile-ready <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty install-java 8
python3 tools/codex/power_mine_mcp.py --pretty scaffold-fabric-mod ./example-mod example_mod
python3 tools/codex/power_mine_mcp.py --pretty scaffold-forge-1.7.10-mod ./example-forge-mod example_forge_mod
python3 tools/codex/power_mine_mcp.py --pretty build-mod-project ./example-forge-mod
python3 tools/codex/power_mine_mcp.py --pretty mod-dev-loop ./example-forge-mod --profile-id <profile-id> --world-name "Codex Dev World"
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod ~/Downloads/example-mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod-content ~/Downloads/example-mod.jar
python3 tools/codex/power_mine_mcp.py --pretty import-mod ~/Downloads/example-mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty set-mod-enabled example-mod.jar --profile-id <profile-id> --no-enabled
python3 tools/codex/power_mine_mcp.py --pretty build-agent
python3 tools/codex/power_mine_mcp.py --pretty build-agent --target forge-1.7.10
python3 tools/codex/power_mine_mcp.py --pretty install-agent --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty repair-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --quick-play-singleplayer "Codex World"
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id> --keep-pause-on-lost-focus
python3 tools/codex/power_mine_mcp.py --pretty agent-world-open "Codex World" --create
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --world-name "Codex World" --create-world
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --launch --quick-play-singleplayer "Codex World"
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --jar-path ~/Downloads/example-mod.jar --give-item example_mod:sample_item --recipe-items minecraft:emerald --expected-output example_mod:sample_item --block example_mod:sample_block
python3 tools/codex/power_mine_mcp.py --pretty agent-health
python3 tools/codex/power_mine_mcp.py --pretty agent-capabilities
python3 tools/codex/power_mine_mcp.py --pretty agent-state
python3 tools/codex/power_mine_mcp.py --pretty agent-inventory
python3 tools/codex/power_mine_mcp.py --pretty agent-release-input
python3 tools/codex/power_mine_mcp.py --pretty agent-give-item example_mod:sample_item --count 1 --slot 0
python3 tools/codex/power_mine_mcp.py --pretty agent-wait-ticks 20
python3 tools/codex/power_mine_mcp.py --pretty agent-use-item --slot 0
python3 tools/codex/power_mine_mcp.py --pretty agent-use-block 0 64 0 --side up
python3 tools/codex/power_mine_mcp.py --pretty agent-interaction-check item --item example_mod:sample_item --require-effect
python3 tools/codex/power_mine_mcp.py --pretty agent-held-item-render --hand main
python3 tools/codex/power_mine_mcp.py --pretty agent-block-render --x 0 --y 64 --z 0
python3 tools/codex/power_mine_mcp.py --pretty agent-screenshot
python3 tools/codex/power_mine_mcp.py --pretty agent-recipe-check "minecraft:oak_log" --width 1 --height 1 --expected-output minecraft:oak_planks
python3 tools/codex/power_mine_mcp.py --pretty agent-recipe-check "minecraft:log" --width 1 --height 1 --expected-output minecraft:planks
python3 tools/codex/power_mine_mcp.py --pretty agent-craft-recipe "minecraft:oak_log" --width 1 --height 1 --expected-output minecraft:oak_planks
python3 tools/codex/power_mine_mcp.py --pretty agent-smoke-test <profile-id> --recipe-items minecraft:oak_log --expected-output minecraft:oak_planks --craft-recipe
python3 tools/codex/power_mine_mcp.py --pretty agent-world-snapshot --radius 2
python3 tools/codex/power_mine_mcp.py --pretty agent-place-block 0 64 0 --block minecraft:stone
python3 tools/codex/power_mine_mcp.py --pretty agent-break-block 0 64 0
python3 tools/codex/power_mine_mcp.py mcp
```

The default data directory follows the launcher:

- Linux: `$XDG_DATA_HOME/power-mine` or `~/.local/share/power-mine`
- macOS: `~/Library/Application Support/Power Mine`

Set `POWER_MINE_DATA_DIR` or pass `--data-dir` to inspect another launcher data directory.

Headless profile install, repair, and launch use the Power Mine binary. The tool looks for it in this order:

1. `POWER_MINE_BINARY`
2. `build/bin/power-mine`
3. `go run -tags desktop,production[,webkit2_41] .` from `POWER_MINE_REPO`
4. `dist/power-mine-0.1.0-linux-x86_64.appimage`

Set `POWER_MINE_GO_TAGS` to override the fallback Go build tags.

## In-Game Agent

Launcher profile automation is broader than runtime automation: headless install/repair/launch supports Forge and NeoForge, including legacy Forge `1.7.10`. Runtime automation uses one bridge protocol with separate agent implementations.

The Fabric agent is currently the most complete implementation for Minecraft `1.20.1` Fabric profiles. Build it with:

```bash
make agent
```

The Forge `1.7.10` agent is a legacy implementation for old modpacks. It supports health/capabilities, state, inventory, give item, hotbar select, world open/create, world tick waiting, item/block right-click, world snapshot, screenshot capture, screenshot-based held-item and block visual probes, legacy crafting checks, runtime inventory craft, place block, and break block. Forge `1.7.10` recipes do not have stable IDs, so recipe checks report the matched recipe class. Held-item and block endpoints return visual probe metrics and PNG paths; baked-model introspection remains unsupported because the legacy renderer is not equivalent to modern baked models. Build it with:

```bash
make agent-forge
```

Then install the matching agent into a supported profile:

```bash
python3 tools/codex/power_mine_mcp.py --pretty install-agent --profile-id <profile-id>
```

After launching the profile and loading a singleplayer world, the agent listens on `127.0.0.1:39276`.
Set `POWER_MINE_AGENT_PORT` or JVM property `power.mine.agent.port` to change the port. Set
`POWER_MINE_AGENT_TOKEN` or JVM property `power.mine.agent.token` to require `Authorization: Bearer <token>`.

Block placement and breaking are deliberately limited to integrated singleplayer worlds. The launcher can pass
`--quick-play-singleplayer` for an existing world. A running agent can also handle `agent-world-open`: Fabric
`1.20.1` opens existing local worlds, while Forge `1.7.10` can create a fresh local test world from the title screen.
Headless launches force `pauseOnLostFocus:false` in `options.txt` unless `--keep-pause-on-lost-focus` is passed.
Use `agent-release-input` after entering a world to ask the agent to release captured mouse input.
Use `agent-wait-ticks`, `agent-use-item`, `agent-use-block`, `agent-interaction-check`, and `agent-craft-recipe` when a mod needs runtime behavior checks beyond static assets, recipes, and block placement. `agent-interaction-check` wraps the raw right-click call with before/after state, inventory, block snapshots, tick waiting, and optional expectations. `agent-craft-recipe` uses the recipe system and player inventory directly; it does not automate mouse clicks in the crafting GUI.

## Local Codex Plugin

The local Codex plugin installed on this machine is `power-mine@personal`.

Source plugin path:

```text
~/plugins/power-mine
```

Installed cache path:

```text
~/.codex/plugins/cache/personal/power-mine/<installed-version>
```

After changing the MCP script or plugin metadata, validate and reinstall:

```bash
python3 ~/.codex/skills/.system/plugin-creator/scripts/validate_plugin.py ~/plugins/power-mine
codex plugin add power-mine@personal
```

Start a new Codex thread after reinstalling so the new MCP tools are loaded.
