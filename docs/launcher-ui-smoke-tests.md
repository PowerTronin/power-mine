# Launcher UI Smoke Tests

These checks are intended for Codex sessions that have access to the `boxes` MCP tools. They validate the real Wails desktop UI in a VM, not only the headless launcher commands.

The first target is `debian-lab`. Windows VMs can reuse the same scenario after package transfer and launch commands are adjusted.

## Tooling

- `boxes_boxes_list`, `boxes_boxes_start`, `boxes_boxes_display`: find and open the VM.
- `boxes_boxes_screenshot`: capture the current desktop/app state.
- `boxes_boxes_mouse`, `boxes_boxes_keyboard`, `boxes_boxes_clipboard`: drive the launcher UI.
- `boxes_boxes_drag_drop`: optional artifact transfer when SPICE file transfer is available.
- Shell/SSH/VM scripts: build, copy, install, and launch artifacts; `boxes` handles the visual part.

## Scope

The visual smoke should prove that:

- the Wails window opens and paints the launcher shell;
- Home, Library, Create, Logs, and Settings are reachable;
- profile/server cards stay compact and do not overflow the Library list;
- local server create/install/start/stop/restart actions update the visible state;
- app-owned logs and terminal windows open inside the launcher without spawning an external browser;
- failures show an error banner or log entry instead of hanging silently.

## Debian Lab Preparation

Host-side build:

```bash
PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH" npm --prefix frontend run build
PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH" wails build -tags webkit2_41
SKIP_WAILS_BUILD=1 make deb
```

Guest-side install options:

- Preferred release-package check: copy `dist/power-mine_<version>_amd64.deb` into `debian-lab`, then run `sudo apt install ./power-mine_<version>_amd64.deb`.
- Faster dev check: copy `build/bin/power-mine` into `debian-lab` and run it directly.
- Always launch with an isolated data directory, for example `POWER_MINE_DATA_DIR=/tmp/power-mine-ui-smoke power-mine`.

Do not reuse `/tmp/power-mine-ui-check` for release smoke runs; stale Java processes or `session.lock` files can hide restart regressions.

## Codex MCP Scenario

This is the baseline script for a Codex operator using `boxes` tools.

1. Start or resume `debian-lab`.
2. Capture the desktop before launching the app.
3. Launch Power Mine from a terminal or application menu with `POWER_MINE_DATA_DIR=/tmp/power-mine-ui-smoke`.
4. Capture the first launcher window; pass if the left rail and Home content are visible.
5. Click Library; pass if client/server installation rows are visible or the empty state is readable.
6. Click Create; create a vanilla client profile with a unique smoke name; pass if Library selects the new profile.
7. Return to Create; create a local vanilla server with a unique smoke name and EULA accepted.
8. Wait for install progress to complete; pass if the server row shows compact text such as `Installed` and no stale progressbar remains.
9. Click Start; pass if the row/detail changes to `Installed / Running` or `Starting` without Java progress noise.
10. Click Stop; wait for stopped state; pass if Start becomes enabled again.
11. Click Start again; pass if restart works and the row returns to `Installed / Running`.
12. Open Terminal for that server; pass if the app-owned terminal opens, shows recent server events, and accepts commands.
13. Open Logs, then Open app window; pass if the app-owned logs window includes the profile/server actions.
14. Capture final screenshots and summarize pass/fail with the screenshot checkpoints.

## Screenshot Checkpoints

Use these checkpoint names in reports so later sessions are comparable:

- `desktop-before-launch`
- `home-opened`
- `library-empty-or-list`
- `profile-created`
- `server-installing`
- `server-installed-no-progress`
- `server-running`
- `server-stopped-start-enabled`
- `server-restarted`
- `server-terminal-opened`
- `logs-app-window-opened`

## Pass/Fail Rules

- Fail if the launcher window is blank, clipped, or inaccessible after launch.
- Fail if navigation clicks stop working on any primary screen.
- Fail if a completed server install still shows an active progressbar in Library/Home/detail.
- Fail if stdout/stderr log events make a stopped server look running again.
- Fail if Stop succeeds but Start stays disabled after the server process exits.
- Fail if Terminal or Open app window launches an external browser in the normal app-owned UI path.
- Warn, not fail, if the app-owned terminal/logs are in-window overlays instead of separate OS-level windows.

## Hardening Follow-ups

- Add mocked frontend tests for the same flows so CI catches obvious regressions before VM checks.
- Add stable `data-ui` attributes or visible checkpoint labels where practical; `boxes` is visual/pixel-driven, not DOM-driven.
- Persist VM screenshots into release artifacts when a scriptable MCP runner is available.
- Extend the scenario to Windows 10/11 after the Debian flow is stable.
