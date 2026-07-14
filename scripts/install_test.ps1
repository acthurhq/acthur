$ErrorActionPreference = 'Stop'

$Root = Split-Path -Parent $PSScriptRoot
$Installer = Join-Path $PSScriptRoot 'install.ps1'
$Fixture = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())

function Fail([string]$Message) {
    throw "FAIL: $Message"
}

try {
    New-Item -ItemType Directory -Path $Fixture | Out-Null

    $fixtureBinary = Join-Path $Fixture 'acthur.exe'
    & go build -o $fixtureBinary -ldflags '-X main.Version=v1.2.3' ./cmd/acthur
    if ($LASTEXITCODE -ne 0) { Fail 'could not build the released binary fixture' }

    $archiveName = 'acthur_1.2.3_windows_amd64.zip'
    $archive = Join-Path $Fixture $archiveName
    Compress-Archive -Path $fixtureBinary -DestinationPath $archive
    $archiveHash = (Get-FileHash -Path $archive -Algorithm SHA256).Hash.ToLowerInvariant()

    $wrapper = Join-Path $Fixture 'invoke-installer.ps1'
    @'
param([string]$Installer, [string]$AppDataRoot, [string]$Archive, [string]$Checksums)
$ErrorActionPreference = 'Stop'
$env:APPDATA = $AppDataRoot
$env:PATH = "$AppDataRoot\acthur\bin;$env:PATH"
$env:TEST_ARCHIVE = $Archive
$env:TEST_CHECKSUMS = $Checksums

function Invoke-RestMethod {
    param($Uri, $Headers)
    [pscustomobject]@{ tag_name = 'v1.2.3' }
}

function Invoke-WebRequest {
    param($Uri, $OutFile, [switch]$UseBasicParsing)
    if ($Uri -like '*.zip') {
        Copy-Item $env:TEST_ARCHIVE $OutFile
    } elseif ($Uri -like '*checksums.txt') {
        Copy-Item $env:TEST_CHECKSUMS $OutFile
    } else {
        throw "unexpected release URL: $Uri"
    }
}

. $Installer
'@ | Set-Content -Path $wrapper

    function Invoke-InstallerCase([string]$Name, [string]$Manifest, [bool]$ShouldPass) {
        $caseRoot = Join-Path $Fixture $Name
        $appData = Join-Path $caseRoot 'appdata'
        $checksums = Join-Path $caseRoot 'checksums.txt'
        New-Item -ItemType Directory -Path $caseRoot | Out-Null
        Set-Content -Path $checksums -Value $Manifest

        $stdout = Join-Path $caseRoot 'stdout.txt'
        $stderr = Join-Path $caseRoot 'stderr.txt'
        $child = Start-Process -FilePath (Get-Process -Id $PID).Path `
            -ArgumentList @('-NoProfile', '-File', $wrapper, $Installer, $appData, $archive, $checksums) `
            -Wait -PassThru -NoNewWindow -RedirectStandardOutput $stdout -RedirectStandardError $stderr
        $passed = $child.ExitCode -eq 0
        if ($passed -ne $ShouldPass) {
            Get-Content $stdout, $stderr | Write-Error
            Fail "$Name returned exit code $($child.ExitCode)"
        }
        return Join-Path $appData 'acthur\bin\acthur.exe'
    }

    $installed = Invoke-InstallerCase 'checksum-mismatch' (('0' * 64) + "  $archiveName") $false
    if (Test-Path $installed) { Fail 'checksum mismatch reached installation' }

    $installed = Invoke-InstallerCase 'missing-entry' (('0' * 64) + '  another.zip') $false
    if (Test-Path $installed) { Fail 'missing checksum entry reached installation' }

    $installed = Invoke-InstallerCase 'valid-release' "$archiveHash  $archiveName" $true
    if (-not (Test-Path $installed)) { Fail 'verified release was not installed' }
    $versionOutput = & $installed version 2>&1 | Out-String
    if ($versionOutput -notmatch 'Version:\s+v?1\.2\.3') {
        Fail 'installed binary did not report requested release version'
    }

    Write-Host 'PASS: install.ps1 verifies release integrity before installation'
} finally {
    Remove-Item -Recurse -Force $Fixture -ErrorAction SilentlyContinue
}
