[CmdletBinding()]
param(
    [string]$Target = $env:TARGET,
    [string]$WebView2Strategy = $env:WEBVIEW2_STRATEGY,
    [string]$DistDir = $env:DIST_DIR,
    [string]$Wails = $env:WAILS
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if ([System.Environment]::OSVersion.Platform -ne [System.PlatformID]::Win32NT) {
    throw "Windows package builds must run on Windows so Wails can invoke the native NSIS toolchain."
}

if ([string]::IsNullOrWhiteSpace($Target)) {
    $Target = "windows/amd64"
}
if ([string]::IsNullOrWhiteSpace($WebView2Strategy)) {
    $WebView2Strategy = "download"
}
if ([string]::IsNullOrWhiteSpace($Wails)) {
    $Wails = "wails"
}

$RootDir = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $RootDir

function Get-JsonString {
    param(
        [Parameter(Mandatory = $true)]$Object,
        [Parameter(Mandatory = $true)][string]$Name
    )

    $property = $Object.PSObject.Properties[$Name]
    if ($null -eq $property -or $null -eq $property.Value) {
        return ""
    }
    return [string]$property.Value
}

function Get-AppVersion {
    $appGo = Join-Path $RootDir "app.go"
    $content = Get-Content -LiteralPath $appGo -Raw
    if ($content -match 'Version:\s*"([^"]+)"') {
        return $Matches[1]
    }
    return "0.0.0"
}

function Require-Command {
    param([Parameter(Mandatory = $true)][string]$Name)

    if ($null -eq (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Required command not found: $Name"
    }
}

function Resolve-RepoPath {
    param([Parameter(Mandatory = $true)][string]$Path)

    if ([System.IO.Path]::IsPathRooted($Path)) {
        return $Path
    }
    return (Join-Path $RootDir $Path)
}

function Write-Sha256File {
    param([Parameter(Mandatory = $true)][string]$Path)

    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLowerInvariant()
    $fileName = Split-Path -Leaf $Path
    Set-Content -LiteralPath "$Path.sha256" -Encoding ASCII -Value "$hash  $fileName"
}

$targetParts = $Target -split "/", 2
if ($targetParts.Count -ne 2 -or $targetParts[0] -ne "windows") {
    throw "Windows package target must look like windows/<arch>, got: $Target"
}

$Arch = $targetParts[1]
if ($Arch -notin @("amd64", "arm64")) {
    throw "Unsupported Windows target architecture: $Arch"
}

$configPath = Join-Path $RootDir "wails.json"
$config = Get-Content -LiteralPath $configPath -Raw | ConvertFrom-Json
$projectName = Get-JsonString $config "name"
if ([string]::IsNullOrWhiteSpace($projectName)) {
    $projectName = "power-mine"
}

$outputFileName = Get-JsonString $config "outputfilename"
if ([string]::IsNullOrWhiteSpace($outputFileName)) {
    $outputFileName = $projectName
}
$binName = [System.IO.Path]::GetFileNameWithoutExtension($outputFileName)
if (-not [string]::IsNullOrWhiteSpace($env:BIN_NAME)) {
    $binName = $env:BIN_NAME
}

$appId = $projectName
if (-not [string]::IsNullOrWhiteSpace($env:APP_ID)) {
    $appId = $env:APP_ID
}

$sourceVersion = Get-AppVersion
$version = $sourceVersion
if (-not [string]::IsNullOrWhiteSpace($env:VERSION)) {
    $version = $env:VERSION
}
if ($version -ne $sourceVersion) {
    throw "VERSION ($version) must match app.go AppInfo version ($sourceVersion)."
}

$infoProperty = $config.PSObject.Properties["info"]
if ($null -eq $infoProperty) {
    throw "wails.json must define info.productVersion for Windows installer metadata."
}
$wailsProductVersion = Get-JsonString $infoProperty.Value "productVersion"
if ($wailsProductVersion -ne $version) {
    throw "wails.json info.productVersion ($wailsProductVersion) must match app.go AppInfo version ($version)."
}

if ([string]::IsNullOrWhiteSpace($DistDir)) {
    $DistDir = Join-Path $RootDir "dist"
} else {
    $DistDir = Resolve-RepoPath $DistDir
}

if ($env:SKIP_WAILS_BUILD -ne "1") {
    Require-Command $Wails
    $buildArgs = @("build", "-clean", "-platform", $Target, "-nsis", "-webview2", $WebView2Strategy, "-trimpath")
    & $Wails @buildArgs
    if ($LASTEXITCODE -ne 0) {
        throw "wails build failed with exit code $LASTEXITCODE"
    }
} else {
    Write-Host "Skipping Wails build because SKIP_WAILS_BUILD=1"
}

$buildBinDir = Join-Path $RootDir "build/bin"
$binaryPath = Join-Path $buildBinDir "$binName.exe"
if (-not [string]::IsNullOrWhiteSpace($env:BINARY_PATH)) {
    $binaryPath = Resolve-RepoPath $env:BINARY_PATH
}

$installerPath = Join-Path $buildBinDir "$projectName-$Arch-installer.exe"
if (-not [string]::IsNullOrWhiteSpace($env:INSTALLER_PATH)) {
    $installerPath = Resolve-RepoPath $env:INSTALLER_PATH
}

if (-not (Test-Path -LiteralPath $binaryPath -PathType Leaf)) {
    throw "Built binary not found at $binaryPath"
}
if (-not (Test-Path -LiteralPath $installerPath -PathType Leaf)) {
    throw "NSIS installer not found at $installerPath"
}

New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

$portableOutput = Join-Path $DistDir "$appId-$version-windows-$Arch.exe"
$installerOutput = Join-Path $DistDir "$appId-$version-windows-$Arch-installer.exe"

Copy-Item -LiteralPath $binaryPath -Destination $portableOutput -Force
Copy-Item -LiteralPath $installerPath -Destination $installerOutput -Force

Write-Sha256File $portableOutput
Write-Sha256File $installerOutput

Write-Host "Windows binary created: $portableOutput"
Write-Host "Windows installer created: $installerOutput"
