# Power Mine Codex Diagnostics

`power_mine_mcp.py` is a standard-library Python diagnostics tool for Codex and local automation.

It can:

- list Power Mine profiles;
- inspect Fabric, Quilt, Forge, and NeoForge mod jars;
- compare mod metadata with a launcher profile;
- scan recent logs, crash reports, and JVM error logs for common mod failures;
- import, enable, disable, and delete profile mod jars;
- call the launcher headless commands to install, repair, or launch a profile;
- run as an MCP stdio server for Codex plugin use.

Examples:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod ~/Downloads/example-mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty import-mod ~/Downloads/example-mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty set-mod-enabled example-mod.jar --profile-id <profile-id> --no-enabled
python3 tools/codex/power_mine_mcp.py --pretty repair-profile <profile-id>
python3 tools/codex/power_mine_mcp.py --pretty launch-profile <profile-id>
python3 tools/codex/power_mine_mcp.py mcp
```

The default data directory follows the launcher:

- Linux: `$XDG_DATA_HOME/power-mine` or `~/.local/share/power-mine`
- macOS: `~/Library/Application Support/Power Mine`

Set `POWER_MINE_DATA_DIR` or pass `--data-dir` to inspect another launcher data directory.

Headless profile install, repair, and launch use the Power Mine binary. The tool looks for it in this order:

1. `POWER_MINE_BINARY`
2. `build/bin/power-mine`
3. `dist/power-mine-0.1.0-linux-x86_64.appimage`
4. `go run .` from `POWER_MINE_REPO`

## Local Codex Plugin

The local Codex plugin installed on this machine is `power-mine@personal`.

Source plugin path:

```text
/home/amd-btw/plugins/power-mine
```

Installed cache path:

```text
/home/amd-btw/.codex/plugins/cache/personal/power-mine/<installed-version>
```

After changing the MCP script or plugin metadata, validate and reinstall:

```bash
python3 /home/amd-btw/.codex/skills/.system/plugin-creator/scripts/validate_plugin.py /home/amd-btw/plugins/power-mine
codex plugin add power-mine@personal
```

Start a new Codex thread after reinstalling so the new MCP tools are loaded.
