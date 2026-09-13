# Changelog

## 0.2.0 - 2026-09-14

Packaging-focused preview release.

- Added Linux `.deb` and `.rpm` packages alongside AppImage builds.
- Added Windows amd64 portable and NSIS installer builds in GitHub Actions.
- Added release smoke-test coverage for Debian, Fedora/RPM, Windows 10, and Windows 11.
- Documented Windows 7 as best-effort and non-blocking for current releases.
- Updated frontend dependencies and split markdown rendering into a lazy chunk.
- Improved legacy Forge `1.7.10` installation and Codex/headless launcher tooling.

## 0.1.0 - 2026-05-22

Initial Power Mine preview release.

- Minecraft profile library with install, repair, launch, and per-profile settings.
- Offline account mode for offline servers.
- Java runtime checks and launcher-managed Java installation.
- Fabric, Quilt, Forge, NeoForge, and Vanilla profile support.
- Modrinth browse, install, dependency handling, update checks, and deletion flows.
- Local mod management with enable, disable, delete, bulk selection, and import.
- Game and launcher logs view.
- Modrinth `.mrpack` import and export.
