# Acthur Installer for Windows (PowerShell)
# Usage: irm https://install.acthur.dev/win | iex
#
# This script:
#   1. Detects architecture (x86_64)
#   2. Downloads the correct binary from GitHub Releases
#   3. Installs to %APPDATA%\acthur\bin
#   4. Adds to system PATH
#   5. Runs `acthur doctor` to verify

$ErrorActionPreference = 'Stop'

$ACTHUR_REPO  = "acthurhq/acthur"
$ACTHUR_DIR   = "$env:APPDATA\acthur"
$ACTHUR_BIN   = "$ACTHUR_DIR\bin"
$ACTHUR_EXE   = "$ACTHUR_BIN\acthur.exe"
$GITHUB_API   = "https://api.github.com/repos/$ACTHUR_REPO/releases/latest"

# ── Colours ───────────────────────────────────────────────────────────────────
function Write-Info    { param($msg) Write-Host "  ->  $msg" -ForegroundColor Cyan }
function Write-Success { param($msg) Write-Host "  v   $msg" -ForegroundColor Green }
function Write-Fail    { param($msg) Write-Host "  x   $msg" -ForegroundColor Red; throw $msg }
function Write-Banner  {
    Write-Host ""
    Write-Host "  *  Acthur Installer" -ForegroundColor White
    Write-Host "     https://acthur.dev" -ForegroundColor DarkGray
    Write-Host ""
}

# ── Architecture ──────────────────────────────────────────────────────────────
function Get-Arch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64"   { return "amd64" }
        "Arm64" { Write-Fail "ARM64 Windows is not yet supported. Download manually from https://github.com/$ACTHUR_REPO/releases" }
        default { Write-Fail "Unsupported architecture: $arch" }
    }
}

# ── Latest version ────────────────────────────────────────────────────────────
function Get-LatestVersion {
    try {
        $response = Invoke-RestMethod -Uri $GITHUB_API -Headers @{ "User-Agent" = "acthur-installer" }
        return $response.tag_name
    } catch {
        Write-Fail "Failed to fetch latest version: $_"
    }
}

# ── Download ──────────────────────────────────────────────────────────────────
function Install-Acthur {
    param($version, $arch)

    $archiveName = "acthur_$($version.TrimStart('v'))_windows_$arch.zip"
    $downloadUrl = "https://github.com/$ACTHUR_REPO/releases/download/$version/$archiveName"
    $checksumsName = "acthur_$($version.TrimStart('v'))_checksums.txt"
    $checksumsUrl = "https://github.com/$ACTHUR_REPO/releases/download/$version/$checksumsName"

    Write-Info "Downloading Acthur $version for windows/$arch..."

    $tmpDir  = [System.IO.Path]::GetTempPath() + [System.Guid]::NewGuid().ToString()
    $zipPath = "$tmpDir\$archiveName"
    $checksumsPath = "$tmpDir\$checksumsName"

    New-Item -ItemType Directory -Path $tmpDir | Out-Null

    try {
        $progressPreference = 'SilentlyContinue'
        Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing
        Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath -UseBasicParsing
    } catch {
        Write-Fail "Release asset download failed for $version`n  Error: $_"
    }

    $manifestEntry = Get-Content -Path $checksumsPath | ForEach-Object {
        $fields = $_ -split '\s+', 2
        if ($fields.Count -eq 2 -and $fields[1].TrimStart('*') -eq $archiveName) {
            $fields[0]
        }
    } | Select-Object -First 1

    if (-not $manifestEntry) {
        Write-Fail "Checksum manifest has no entry for $archiveName"
    }

    $actualSha256 = (Get-FileHash -Path $zipPath -Algorithm SHA256).Hash
    if ($actualSha256 -ne $manifestEntry) {
        Write-Fail "Checksum verification failed for $archiveName"
    }

    # Extract
    Write-Info "Extracting..."
    Expand-Archive -Path $zipPath -DestinationPath $tmpDir -Force

    # Install
    New-Item -ItemType Directory -Path $ACTHUR_BIN -Force | Out-Null
    Copy-Item "$tmpDir\acthur.exe" -Destination $ACTHUR_EXE -Force

    # Cleanup
    Remove-Item -Recurse -Force $tmpDir
}

# ── PATH ──────────────────────────────────────────────────────────────────────
function Add-ToPath {
    $currentPath = [System.Environment]::GetEnvironmentVariable("PATH", "User")

    if ($currentPath -like "*$ACTHUR_BIN*") {
        return # already in PATH
    }

    $newPath = "$ACTHUR_BIN;$currentPath"
    [System.Environment]::SetEnvironmentVariable("PATH", $newPath, "User")

    # Also update current session
    $env:PATH = "$ACTHUR_BIN;$env:PATH"

    Write-Info "Added $ACTHUR_BIN to PATH"
}

# ── Verify ────────────────────────────────────────────────────────────────────
function Verify-Installation {
    param($version)
    try {
        $output = & $ACTHUR_EXE version 2>&1 | Out-String
        $expectedVersion = $version.TrimStart('v')
        if ($output -notmatch "Version:\s+v?$([regex]::Escape($expectedVersion))") {
            Write-Fail "Installation verification failed: binary does not report requested version $version"
        }
        Write-Success "Acthur $version installed successfully"
    } catch {
        Write-Fail "Installation verification failed: $_"
    }
}

# ── Main ──────────────────────────────────────────────────────────────────────
Write-Banner

$arch    = Get-Arch
$version = Get-LatestVersion

Install-Acthur -version $version -arch $arch
Add-ToPath
Verify-Installation -version $version

Write-Host ""
Write-Host "  Next steps:" -ForegroundColor White
Write-Host ""
Write-Host "  Restart your terminal, then:" -ForegroundColor DarkGray
Write-Host "    acthur doctor           " -NoNewline; Write-Host "check your environment" -ForegroundColor DarkGray
Write-Host "    acthur new my-project   " -NoNewline; Write-Host "create a new project" -ForegroundColor DarkGray
Write-Host "    acthur --help           " -NoNewline; Write-Host "see all commands" -ForegroundColor DarkGray
Write-Host ""
Write-Host "  Docs: https://acthur.dev" -ForegroundColor DarkGray
Write-Host ""
