# Detached Runtime Windows Plan

Date: 2026-09-18

Related area: local servers, game logs, desktop shell polish.

## Goal

Make runtime tools feel like launcher-native utilities instead of browser tabs:

- local server terminal opens as a separate app window;
- game logs open as a separate app window;
- both windows stay synchronized with launcher events;
- browser-based HTTP views remain a development fallback, not the primary UX.

## Current Status

As of `b3dfac4`, Wails v2.15 does not expose a suitable public multi-window API for this app, so the launcher uses app-owned in-window utilities for logs and local server terminals. Native separate OS-level windows remain a future implementation path; browser HTTP views are legacy/fallback only.

## Product Decisions

- Server terminal and game logs are distinct windows, not tabs inside the main Library view.
- The main launcher owns process lifecycle, state, and event history.
- Detached windows are views over existing launcher state and should not duplicate server/profile control logic.
- Terminal input is limited to the server command channel; game logs are read-only at first.
- If a detached window is closed, the server/game process continues unless the user explicitly stops it.

## Milestone 1: Native Window Host

Goal: replace browser-tab launch with native Wails windows.

Tasks:

- Add a small window manager in the Go backend for detached runtime windows.
- Create window types for `server-terminal` and `game-logs`.
- Route each window to a frontend entry/view with a stable target id.
- Keep one window per server/profile by default and focus the existing one when reopened.
- Preserve the current browser HTTP implementation as a fallback while native windows mature.

Acceptance:

- Clicking Terminal for a local server opens a separate app window.
- Clicking detached logs opens a separate app window.
- Reopening focuses the existing matching window instead of spawning duplicates.

## Milestone 2: Event Synchronization

Goal: keep detached windows live without polling-heavy UI hacks.

Tasks:

- Reuse existing server/game event history as the initial snapshot.
- Stream new runtime events into the matching detached window.
- Keep terminal input routed through the backend server command API.
- Prevent log stream events from overwriting lifecycle state in the main UI.

Acceptance:

- Server terminal receives stdout, stderr, stdin echo, start, stop, and failure events.
- Game logs receive launch, stdout, stderr, stop, and failure events.
- Closing and reopening a detached window restores recent history.

## Milestone 3: Window UX

Goal: make detached tools useful during real play/testing.

Tasks:

- Add copy, clear visible output, and auto-scroll controls.
- Add status chips for Running, Starting, Stopped, and Failed.
- Add lightweight filters for stdout, stderr, lifecycle, and commands.
- Remember size and position per detached window type.

Acceptance:

- Logs stay readable on desktop and small screens.
- Auto-scroll can be paused without losing incoming events.
- Window placement is restored on next open.

## Custom App Frame Workstream

Steam-style chrome is feasible in Wails, but it should be a separate shell-polish task.

Approach:

- Enable Wails frameless mode with `options.App{Frameless: true}`.
- Build a custom React title bar with app navigation, account status, and window controls.
- Use Wails drag regions through the configured CSS drag property/value.
- Recreate minimize, maximize, close, and double-click behavior per platform.
- Validate Linux window-manager behavior before making frameless the default.

Risks:

- Frameless windows can vary across Linux desktop environments.
- Native menus, shadows, resize borders, and accessibility need manual QA.
- Detached utility windows may need simpler chrome than the main launcher.

Acceptance:

- Main window can run frameless without losing resize/move/window-control behavior.
- The custom title bar matches the launcher visual language.
- Linux and Windows builds pass smoke checks before release.
