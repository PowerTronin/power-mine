# Detached Runtime Release Smoke Plan

Date: 2026-09-19

Related plan: `docs/superpowers/plans/2026-09-18-detached-runtime-windows-plan.md`

## Goal

Promote detached runtime windows from implemented feature to release-accepted feature by validating real packaged builds on the primary target platforms.

The smoke pass should prove that release artifacts preserve the native-window behavior, not only the development build:

- detached logs and server terminal open as separate OS-level app windows;
- reopening a detached tool focuses the existing helper window;
- SSE history/live updates, terminal input, and utility controls work in packaged builds;
- size and position are restored after close/reopen and app restart;
- closing utility windows or the main launcher does not leave visible or hidden orphan processes.

## Current Inputs

- Linux package commands: `make appimage`, `make deb`, `make rpm`.
- Windows package command: `make windows` on Windows with NSIS installed.
- Headless package smoke: `docs/release-smoke-tests.md`.
- Visual UI smoke baseline: `docs/launcher-ui-smoke-tests.md`.
- Ready Linux VM target: `debian-lab`.
- Windows VM targets: `windows10-lab`, `windows11-lab` after interactive OS setup is complete.

## Release Gate

Detached runtime windows are release-accepted when the following pass on Linux and Windows 10/11 release artifacts:

- Open Logs from the launcher and confirm a separate native window appears.
- Open a local server Terminal and confirm a separate native window appears.
- Reopen Logs/Terminal and confirm the existing window is focused instead of duplicated.
- Use filters, follow pause/resume, copy visible, and clear visible in both windows.
- Start a local server, send a terminal command, and confirm stdin/stdout/lifecycle entries appear in terminal and logs.
- Resize/move each window, close it, reopen it, and confirm placement is restored.
- Restart the launcher and confirm placement is still restored.
- Close utility windows and main launcher; confirm no `power-mine` helper processes remain.

## Milestone 1: Linux Artifact Build

Goal: create local Linux artifacts that match release packaging behavior.

Tasks:

- On the current Kali host, prefix local Go/Wails commands with `PATH="$HOME/.local/go/bin:$HOME/go/bin:$PATH"`.
- Run the normal verification suite before packaging:
  - `go test ./...`
  - `npm --prefix frontend run build`
  - `wails build -tags webkit2_41`
- Build Linux artifacts:
  - `SKIP_WAILS_BUILD=1 make deb`
  - `SKIP_WAILS_BUILD=1 make appimage`
  - `SKIP_WAILS_BUILD=1 make rpm`
- Record artifact names and hashes from `dist/`.

Acceptance:

- The `.deb`, AppImage, and `.rpm` files exist in `dist/`.
- The production binary still starts headless with an isolated `POWER_MINE_DATA_DIR`.

## Milestone 2: Debian GUI Smoke

Goal: validate the `.deb` package and detached windows in a real desktop VM.

Tasks:

- Copy the current `.deb` into `debian-lab`.
- Install it with `sudo apt install ./power-mine_<version>_amd64.deb`.
- Launch with an isolated data directory, for example `POWER_MINE_DATA_DIR=/tmp/power-mine-detached-smoke power-mine`.
- Run the visual flow from `docs/launcher-ui-smoke-tests.md`.
- Add detached-window checkpoints:
  - `logs-native-window-opened`
  - `logs-controls-verified`
  - `logs-placement-restored`
  - `terminal-native-window-opened`
  - `terminal-command-copied`
  - `terminal-placement-restored`
  - `no-helper-processes-after-close`

Acceptance:

- The `.deb` package passes headless and GUI smoke checks.
- Detached utility windows pass the release gate on Debian.

## Milestone 3: Windows 10/11 GUI Smoke

Goal: validate Windows release artifacts and OS-specific native-window behavior.

Tasks:

- Build or download Windows portable and installer artifacts.
- Run the existing elevated PowerShell installer smoke from `docs/release-smoke-tests.md` on Windows 10 and Windows 11.
- Launch the portable artifact with isolated `POWER_MINE_DATA_DIR`.
- Repeat the detached-window release gate on both Windows versions.
- Pay special attention to:
  - helper executable resolution from installed and portable locations;
  - focus existing window behavior;
  - clipboard writes from copy visible;
  - window placement with Windows scaling/DPI;
  - clean helper process shutdown.

Acceptance:

- Windows 10 portable and installer artifacts pass headless and GUI detached-window smoke.
- Windows 11 portable and installer artifacts pass headless and GUI detached-window smoke.

## Milestone 4: Hardening From Smoke Results

Goal: keep fixes narrow and driven by real package/VM failures.

Potential fixes:

- Add screen-aware placement clamping if a saved window opens off-screen.
- Add OS-specific placement guards if Windows reports transient close geometry.
- Add a parent-process watchdog if helper windows can outlive the main launcher in packaged builds.
- Move detached page HTML out of `app.go` only if further UI work makes string templates hard to maintain.

Acceptance:

- Every hardening change has a reproduction note from the smoke run.
- The final release checklist documents remaining warnings separately from blockers.

## Non-Goals

- Do not add more detached-window features before the release smoke pass.
- Do not replace the helper-process architecture during this validation pass.
- Do not make browser fallback a public release path.
- Do not block Windows 10/11 release acceptance on Windows 7 behavior.
