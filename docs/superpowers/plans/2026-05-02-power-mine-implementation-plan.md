# Power Mine Implementation Plan

Date: 2026-05-02

Design spec: `docs/superpowers/specs/2026-05-02-power-mine-launcher-design.md`

## Baseline Choices

- App framework: Wails v2.
- Backend: Go.
- Frontend: React + TypeScript + Vite.
- Initial platforms: macOS and Linux.
- MVP loaders: Vanilla and Fabric.
- Storage: JSON files for non-secret app data, system keyring for secrets.
- First implementation priority: a working vanilla launch path before Fabric polish.

## Milestone 0: Repository Setup

Goal: make the empty project buildable and testable.

Tasks:

- Initialize a Wails v2 app scaffold in this repository.
- Add `.gitignore` entries for Wails build output, frontend dependencies, logs, local app data, and `.superpowers/`.
- Add project docs for local development commands.
- Add root `Makefile` or `Taskfile.yml` with common commands:
  - `dev`
  - `build`
  - `test`
  - `test-go`
  - `test-frontend`
- Verify a clean `wails dev` launches the shell app.

Acceptance:

- `go test ./...` passes.
- Frontend package install/build passes.
- Wails dev server opens the starter application.

## Milestone 1: App Shell And UI Structure

Goal: create the Modrinth-like navigation shell without launcher logic.

Backend tasks:

- Add Wails app service registration.
- Add basic `AppService` with app info and health methods.
- Add `SettingsService` with default settings load/save.

Frontend tasks:

- Build app layout:
  - left navigation rail
  - top account/status area
  - main content area
- Add screens:
  - Home
  - Library
  - Profile detail
  - Create profile
  - Account
  - Logs
  - Settings
  - Browse placeholder
- Add loading, empty, and error states.

Acceptance:

- User can navigate all primary screens.
- Settings can be edited and persisted locally.
- No screen depends on real Minecraft APIs yet.

## Milestone 2: Domain Models And Storage

Goal: define stable internal models before network/install logic.

Backend tasks:

- Create packages for:
  - `internal/domain`
  - `internal/storage`
  - `internal/settings`
  - `internal/profiles`
  - `internal/platform`
- Define models:
  - `Account`
  - `Profile`
  - `Loader`
  - `Settings`
  - `InstallState`
  - `LaunchState`
  - typed app errors
- Implement platform app-data path resolution:
  - macOS `~/Library/Application Support/Power Mine`
  - Linux `$XDG_DATA_HOME/power-mine` or fallback
- Implement atomic JSON read/write helpers.
- Implement `ProfileService` CRUD and validation.

Frontend tasks:

- Wire Library and Create Profile screens to real profile APIs.
- Add profile validation feedback.

Acceptance:

- User can create, edit, delete, and select local profiles.
- Profiles persist after app restart.
- Unit tests cover path resolution, JSON storage, and profile validation.

## Milestone 3: Authentication

Goal: sign in with a Microsoft account and expose Minecraft profile state to the app.

Backend tasks:

- Add `internal/auth`.
- Implement Microsoft public-client auth flow suitable for desktop.
- Implement Xbox Live, XSTS, and Minecraft Services token exchange.
- Fetch Minecraft profile.
- Classify auth and ownership/profile failures.
- Store refresh/session secrets in system keyring.
- Store non-secret account metadata in local storage.
- Add token refresh path.

Frontend tasks:

- Implement Account screen sign-in/sign-out UI.
- Show current player name/UUID.
- Show actionable auth errors.

Acceptance:

- Valid Java Edition account signs in and survives app restart.
- Sign-out clears local account state and keyring secrets.
- Auth errors are visible without exposing tokens.

## Milestone 4: Minecraft Metadata And Downloader

Goal: resolve and download vanilla Minecraft files safely.

Backend tasks:

- Add `internal/minecraft/meta`.
- Parse version manifest and version metadata.
- Resolve client jar, libraries, assets, asset indexes, and natives for macOS/Linux.
- Implement download planner.
- Implement downloader with:
  - retries
  - progress events
  - `.part` files
  - atomic rename
  - hash/size validation
- Cache metadata and downloaded files under `minecraft/`.

Frontend tasks:

- Add install progress UI on profile detail.
- Add install failure details.

Acceptance:

- A vanilla profile can be installed without launching yet.
- Reinstall skips valid cached files.
- Fixture tests cover metadata parsing and download planning.

## Milestone 5: Java Validation And Vanilla Launch

Goal: launch a vanilla Minecraft profile.

Backend tasks:

- Add `internal/java`.
- Detect configured Java executable.
- Validate Java can run and report version.
- Add `internal/launch`.
- Build classpath, JVM args, natives path, and game args.
- Inject auth/session/profile fields.
- Start Java child process.
- Stream stdout/stderr to `LogService` and Wails events.
- Track process status and exit code.

Frontend tasks:

- Add Play/Stop state on profile detail.
- Add live log stream.
- Add launch result and exit code display.

Acceptance:

- A vanilla profile launches on macOS.
- A vanilla profile launches on Linux.
- Logs are streamed live and saved after exit.
- Launch command construction has unit coverage.

## Milestone 6: Fabric Loader

Goal: support Fabric profiles through a loader abstraction.

Backend tasks:

- Add `internal/loaders`.
- Define `LoaderInstaller`.
- Implement `FabricInstaller`.
- Query Fabric Meta API for loader and installer metadata.
- Resolve Fabric libraries, main class, and launch metadata.
- Merge Fabric launch data into the vanilla launch plan without duplicating core launch logic.
- Store selected Fabric loader version in profile config.

Frontend tasks:

- Add Fabric option to create/edit profile.
- Add Minecraft version and Fabric loader version selection.
- Show Fabric install state separately enough to diagnose failures.

Acceptance:

- User can create a Fabric profile.
- Fabric profile installs required files.
- Fabric profile launches on macOS and Linux.
- Vanilla launch path remains unchanged by Fabric-specific logic.

## Milestone 7: Logs, Errors, And UX Polish

Goal: make failures understandable enough for real use.

Backend tasks:

- Normalize typed errors across services.
- Add log retention policy.
- Add install/launch log files per run.
- Add network retry settings.
- Add cache cleanup helpers.

Frontend tasks:

- Add Logs screen with app/install/launch categories.
- Add expandable technical details for errors.
- Add settings validation.
- Add empty states and disabled states for unavailable actions.
- Keep Browse visibly present but inactive for MVP.

Acceptance:

- Failed auth, install, Java, and launch paths show useful messages.
- Logs can be opened from profile detail and Logs screen.
- Settings changes are validated before save.

## Milestone 8: Tests And Packaging

Goal: make the MVP reproducible outside the dev machine.

Backend tasks:

- Expand Go test coverage for domain, storage, metadata, downloader planning, auth error classification, and launch args.
- Add fake Java smoke test.
- Add fixture-based Fabric tests.

Frontend tasks:

- Add frontend smoke tests for primary screens.
- Add build verification.

Packaging tasks:

- Build macOS app bundle.
- Build Linux binary/package output supported by Wails v2.
- Document prerequisites for Java and system keyring.

Acceptance:

- `go test ./...` passes.
- Frontend tests/build pass.
- Wails production build succeeds.
- Manual acceptance from the design spec is completed on macOS and Linux.

## Risks And Mitigations

- Microsoft auth can be brittle: isolate it behind `AuthService`, keep token logging disabled, and test error classification with fixtures.
- Minecraft metadata changes over time: keep parsers strict where required but tolerant of unknown fields.
- Fabric metadata differs from vanilla metadata: merge through loader plans, not launch-time conditionals spread across the codebase.
- Java versions vary by Minecraft version: start with user-configured Java, then add managed Java later.
- Linux keyring availability varies by desktop environment: detect keyring failures and show a clear setup error.

## First Coding Slice

The first implementation slice should include:

- Wails v2 scaffold.
- React + TypeScript app shell.
- Settings persistence.
- Profile CRUD with local JSON storage.
- Basic tests for storage/path/profile validation.

This slice gives a working desktop app foundation without depending on Microsoft or Mojang services yet.
