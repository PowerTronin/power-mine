# Release Smoke Tests

Use these checks after a package artifact has been built locally or downloaded from GitHub Actions.

## Linux

- AppImage: run `./power-mine-<version>-linux-x86_64.appimage headless create-profile --name Smoke --loader vanilla --minecraft-version 1.20.1` with `POWER_MINE_DATA_DIR` pointed at a temporary directory.
- Debian: install `power-mine_<version>_amd64.deb`, verify `/opt/power-mine/power-mine`, `/usr/bin/power-mine`, the desktop file, and the icon, then run the same headless `create-profile` smoke and uninstall.
- RPM: install `power-mine-<version>-1.x86_64.rpm`, verify `/opt/power-mine/power-mine`, `/usr/bin/power-mine`, the desktop file, and the icon, then run the same headless `create-profile` smoke and uninstall.

## Windows 10/11

Run these in an elevated PowerShell session for the installer smoke:

```powershell
$Version = "0.1.0"
$ArtifactDir = "C:\Users\user\Downloads"
$Installer = Join-Path $ArtifactDir "power-mine-$Version-windows-amd64-installer.exe"
$Portable = Join-Path $ArtifactDir "power-mine-$Version-windows-amd64.exe"
$DataDir = Join-Path $env:TEMP "power-mine-smoke"

Get-FileHash -Algorithm SHA256 $Installer
Get-FileHash -Algorithm SHA256 $Portable

Remove-Item -Recurse -Force $DataDir -ErrorAction SilentlyContinue
$env:POWER_MINE_DATA_DIR = $DataDir
& $Portable headless create-profile --name SmokePortable --loader vanilla --minecraft-version 1.20.1
if ($LASTEXITCODE -ne 0) { throw "Portable smoke failed" }

& $Installer /S
if ($LASTEXITCODE -ne 0) { throw "Installer failed with exit code $LASTEXITCODE" }

$InstalledExe = Join-Path $env:ProgramFiles "Power Mine\Power Mine\power-mine.exe"
if (-not (Test-Path $InstalledExe)) { throw "Installed binary missing: $InstalledExe" }
if (-not (Test-Path (Join-Path $env:ProgramData "Microsoft\Windows\Start Menu\Programs\Power Mine.lnk"))) { throw "Start menu shortcut missing" }
if (-not (Test-Path (Join-Path $env:PUBLIC "Desktop\Power Mine.lnk"))) { throw "Desktop shortcut missing" }

& $InstalledExe headless create-profile --name SmokeInstalled --loader vanilla --minecraft-version 1.20.1
if ($LASTEXITCODE -ne 0) { throw "Installed headless smoke failed" }

& (Join-Path (Split-Path $InstalledExe) "uninstall.exe") /S
if ($LASTEXITCODE -ne 0) { throw "Uninstall failed with exit code $LASTEXITCODE" }
if (Test-Path $InstalledExe) { throw "Installed binary remained after uninstall" }

Remove-Item -Recurse -Force $DataDir -ErrorAction SilentlyContinue
```

## Windows 7 Best Effort

The primary Wails NSIS installer intentionally checks for Windows 10 or newer. On Windows 7, treat a silent installer exit code `64` as the expected result for the primary installer. Any Windows 7 support should be validated separately with a future legacy/portable build and must not block Windows 10/11 releases.
