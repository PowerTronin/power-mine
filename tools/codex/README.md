# Power Mine Codex Diagnostics

`power_mine_mcp.py` is a standard-library Python diagnostics tool for Codex and local automation.

It can:

- list Power Mine profiles;
- inspect Fabric, Quilt, Forge, and NeoForge mod jars;
- compare mod metadata with a launcher profile;
- scan recent logs, crash reports, and JVM error logs for common mod failures;
- run as an MCP stdio server for Codex plugin use.

Examples:

```bash
python3 tools/codex/power_mine_mcp.py --pretty list-profiles
python3 tools/codex/power_mine_mcp.py --pretty diagnose-profile
python3 tools/codex/power_mine_mcp.py --pretty diagnose-mod ~/Downloads/example-mod.jar --profile-id <profile-id>
python3 tools/codex/power_mine_mcp.py mcp
```

The default data directory follows the launcher:

- Linux: `$XDG_DATA_HOME/power-mine` or `~/.local/share/power-mine`
- macOS: `~/Library/Application Support/Power Mine`

Set `POWER_MINE_DATA_DIR` or pass `--data-dir` to inspect another launcher data directory.

## Local Codex Plugin

The local Codex plugin installed on this machine is `power-mine@personal`.

Source plugin path:

```text
/home/amd-btw/plugins/power-mine
```

Installed cache path:

```text
/home/amd-btw/.codex/plugins/cache/personal/power-mine/0.1.0
```

After changing the MCP script or plugin metadata, validate and reinstall:

```bash
python3 /home/amd-btw/.codex/skills/.system/plugin-creator/scripts/validate_plugin.py /home/amd-btw/plugins/power-mine
codex plugin add power-mine@personal
```

Start a new Codex thread after reinstalling so the new MCP tools are loaded.
