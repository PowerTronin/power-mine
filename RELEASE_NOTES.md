# Power Mine 0.2.0

Packaging-focused preview release of Power Mine, a Go + Wails Minecraft launcher.

License: GPL-3.0.

## Included

- Linux release packages now include AppImage, `.deb`, and `.rpm` artifacts.
- Windows amd64 release packages now include a portable `.exe` and NSIS installer.
- GitHub Actions can build and attach Linux and Windows packages to a published release.
- Release smoke tests cover Debian, Fedora/RPM, Windows 10, and Windows 11 package flows.
- Windows 7 remains best-effort and is not a blocker for Windows 10/11 releases.
- Frontend dependencies were updated and markdown rendering now loads as a separate chunk.
- Legacy Forge `1.7.10` install handling and Codex/headless launcher tooling were improved.

## Installers

- `power-mine-0.2.0-linux-x86_64.appimage`
- `power-mine_0.2.0_amd64.deb`
- `power-mine-0.2.0-1.x86_64.rpm`
- `power-mine-0.2.0-windows-amd64.exe`
- `power-mine-0.2.0-windows-amd64-installer.exe`

## Smoke Status

- Debian install, headless profile creation, and uninstall: passed.
- Fedora/RPM install, headless profile creation, and uninstall: passed.
- Windows 10 portable/installer smoke: passed.
- Windows 11 portable/installer smoke: passed.

This is still a preview build. Microsoft account authentication and signed/notarized installers are planned later.
