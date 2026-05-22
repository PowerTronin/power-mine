# Power Mine Launcher Design

Date: 2026-05-02

## Goal

Build **Power Mine**, a desktop Minecraft Java Edition launcher written primarily in Go. The app should be similar in long-term direction to Modrinth App, but the first MVP focuses on a reliable full launcher core instead of a mod/catalog ecosystem.

The MVP supports macOS and Linux first. Windows is out of scope for the first implementation, but the architecture should avoid platform decisions that would make Windows impossible later.

## Product Scope

The first version is a Wails v2 desktop app:

- Go owns authentication, installation, local storage, process launching, file integrity, and logs.
- The frontend owns layout, state presentation, user input, install progress, and log viewing.
- The UI follows a Modrinth-like navigation model with primary areas for home/library, profiles, account, logs, settings, and a future browse area.

MVP capabilities:

- Microsoft account sign-in for Minecraft Java Edition.
- Minecraft profile validation and token refresh.
- Vanilla profile creation.
- Fabric profile creation.
- Minecraft version installation.
- Fabric loader installation.
- Java selection/configuration.
- Game launch with live logs.
- Basic app settings.

Explicitly out of scope for MVP:

- Modrinth catalog browsing.
- Installing individual mods from Modrinth.
- Importing `.mrpack`.
- Quilt, Forge, and NeoForge installers.
- Server list management.
- Skin/cape management.
- Launcher auto-update.
- Windows packaging.

## Technical Approach

Use **Wails v2** rather than Wails v3 for the MVP. Wails v2 is the stable baseline, while Wails v3 is still maturing. The project can revisit Wails v3 after the launcher core is stable.

The implementation should use a custom Go launcher core instead of delegating core launch behavior to an external launcher engine. This gives us control over storage layout, profile model, loader abstractions, integrity checks, logs, and future Modrinth integration.

Primary external services:

- Microsoft identity platform for OAuth sign-in.
- Xbox Live and XSTS token exchange.
- Minecraft Services for Minecraft access tokens, ownership/profile checks, and player profile data.
- Mojang/Piston metadata for Minecraft version manifests, libraries, assets, client jars, and natives.
- Fabric Meta API for Fabric loader metadata.

## Architecture

The app is split into frontend, Go application services, and infrastructure packages.

Frontend responsibilities:

- Render Modrinth-like navigation.
- Show account state.
- Show profile library and selected profile detail.
- Collect profile creation/editing settings.
- Display installation progress.
- Display launch state and live logs.
- Display settings and validation errors.

Go service responsibilities:

- `AuthService`: Microsoft OAuth flow, token exchange, token refresh, Minecraft profile fetch, ownership/profile validation.
- `ProfileService`: profile CRUD, validation, selected/default profile handling.
- `SettingsService`: app settings, Java path, memory defaults, data directory.
- `VersionService`: Minecraft version manifest retrieval, metadata parsing, cache lookup.
- `InstallService`: common install orchestration, download planning, hash/size validation, atomic writes.
- `FabricInstaller`: Fabric-specific metadata resolution and profile patching.
- `JavaService`: Java discovery, configured Java validation, future managed Java support.
- `LaunchService`: classpath construction, JVM/game argument construction, environment setup, process lifecycle.
- `LogService`: install and launch log capture, streaming, retention.

Loader installer abstraction:

```go
type LoaderInstaller interface {
    LoaderID() string
    Resolve(ctx context.Context, mcVersion string, loaderVersion string) (*LoaderPlan, error)
    Apply(ctx context.Context, profile *Profile, plan *LoaderPlan) error
}
```

Fabric is the only implemented loader in the MVP. Quilt, Forge, and NeoForge should be future implementations of the same boundary, not special cases embedded in launch code.

## Local Storage

Default app data directories:

- macOS: `~/Library/Application Support/Power Mine`
- Linux: `$XDG_DATA_HOME/power-mine`, falling back to `~/.local/share/power-mine`

Storage layout:

```text
power-mine/
  settings.json
  profiles.json
  minecraft/
    versions/
    libraries/
    assets/
    natives/
    downloads/
  instances/
    <profile-id>/
      minecraft/
      mods/
      saves/
      resourcepacks/
      options.txt
  logs/
    app.log
    installs/
    launches/
```

Secrets:

- Store Microsoft/Minecraft refresh/session secrets in the system keyring.
- Store only non-secret account metadata in local JSON, such as account id, player name, UUID, and avatar/skin reference if available.

## Core User Flows

### Sign In

The user clicks sign in. The app starts a Microsoft public-client OAuth flow suitable for desktop. The auth service exchanges the Microsoft token through Xbox Live, XSTS, and Minecraft Services. It then checks that a Minecraft Java profile exists. If the account is not usable for Java Edition, the UI shows an ownership/profile error.

### Create Profile

The user chooses:

- Profile name.
- Minecraft version.
- Loader type: `Vanilla` or `Fabric`.
- Fabric loader version when applicable, with latest stable as default.
- Game directory, defaulting to `instances/<profile-id>/minecraft`.
- Memory settings, defaulting from global settings.

The profile is saved only after validation succeeds.

### Install Profile

The install service reads the Minecraft version manifest, fetches the selected version metadata, resolves libraries/assets/natives, and downloads missing files. It verifies hash and size when metadata provides them. Downloads are written as `.part` files and atomically renamed when complete.

For Fabric profiles, the Fabric installer resolves loader metadata and adds required libraries, main class, and arguments to the launch plan.

### Launch Profile

The launch service validates account state, Java availability, installed files, and profile configuration. It builds JVM args, classpath, natives path, game args, and environment variables. It starts Java as a child process, streams stdout/stderr into UI logs, records exit code, and marks the profile state as stopped when the process exits.

### Settings

The settings screen supports:

- Java executable path.
- Default memory min/max.
- App data directory display.
- Network retry settings.
- Cache controls for metadata/downloads.

## Error Handling

Errors should be typed enough for the UI to show useful messages:

- `AuthError`: Microsoft/Xbox/Minecraft token flow failed.
- `OwnershipError`: account cannot access Java Edition or has no usable Minecraft profile.
- `NetworkError`: metadata or file download failed.
- `IntegrityError`: hash or size mismatch.
- `JavaError`: Java missing, incompatible, or not executable.
- `InstallError`: installation plan or write failed.
- `LaunchError`: argument construction or process start failed.

UI behavior:

- Show a short readable message.
- Offer expandable technical details.
- Preserve logs for failed installs and launches.
- Retry network operations where safe.
- Do not retry auth/ownership failures without user action.

## Testing Strategy

Go unit tests:

- Profile validation.
- Settings path resolution.
- Minecraft metadata parsing.
- Fabric metadata parsing.
- Download plan construction.
- JVM/classpath/game argument construction.
- Error classification.

Go integration tests:

- Use fixture JSON for Mojang/Piston and Fabric responses.
- Test install planning without network.
- Test launch command construction against known fixture profiles.
- Use a fake Java executable for process smoke tests.

Frontend tests:

- Smoke-test key screens in browser mode: account, library, profile detail, create profile, settings, logs.
- Validate basic loading/error/empty states.

Manual MVP acceptance:

- Sign in with a valid Microsoft account that has Java Edition access.
- Create and install a vanilla profile.
- Create and install a Fabric profile.
- Launch both profiles on macOS.
- Launch both profiles on Linux.
- Confirm logs stream during launch and remain available after exit.

## Implementation Phases

### Phase 1: App Skeleton

Create Wails v2 project, frontend shell, navigation, settings store, profile store, and basic service bindings.

### Phase 2: Authentication

Implement Microsoft/Minecraft auth, keyring storage, account state, refresh flow, and profile validation.

### Phase 3: Vanilla Install And Launch

Implement Mojang/Piston metadata parsing, downloader, asset/library layout, Java validation, launch command construction, and log streaming.

### Phase 4: Fabric

Implement Fabric metadata resolution and Fabric profile install/launch through the loader installer abstraction.

### Phase 5: Polish And Verification

Improve install progress, error details, settings validation, log viewer, packaging scripts for macOS/Linux, and smoke tests.

## Decisions Confirmed

- MVP target: full launcher, not just a mod manager or mock UI.
- Initial platforms: macOS and Linux.
- UI framework: Wails with web UI.
- Wails version: v2 for MVP stability.
- Loader support: Fabric in MVP; Quilt/Forge/NeoForge later via installer abstraction.
- Modrinth integration: out of scope for MVP.
- Layout direction: Modrinth-like navigation.
- Core implementation: custom Go launcher core.
