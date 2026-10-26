# Install the cid CLI from GitHub Releases, verifying its SHA-256 checksum.
#
# Usage:
#   irm https://raw.githubusercontent.com/nicoarbelaez/claude-identity/main/scripts/install.ps1 | iex
#   $env:CID_VERSION = '<version>'; irm <url> | iex  # pin a version
#
# Environment:
#   CID_VERSION      Version to install (default: latest release).
#   CID_INSTALL_DIR  Install directory (default: %LOCALAPPDATA%\Programs\cid).
#   CID_NO_MODIFY_PATH=1  Do not add the install directory to the user PATH.
#   CID_VERIFY_ATTESTATION=0  Skip `gh attestation verify` even if gh exists.
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

$Repo = 'nicoarbelaez/claude-identity'
$Bin = 'cid'

# True when $Dir is an entry of the ';'-separated $PathList (case-insensitive, ignoring trailing '\').
function Test-PathContains([string]$PathList, [string]$Dir) {
    $want = $Dir.Trim().TrimEnd('\')
    foreach ($entry in ($PathList -split ';')) {
        if ($entry.Trim().TrimEnd('\') -ieq $want) { return $true }
    }
    return $false
}

switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { $Arch = 'amd64' }
    'ARM64' { $Arch = 'arm64' }
    default { throw "Unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}
# A 32-bit PowerShell on 64-bit Windows reports x86; use the real architecture.
if ($env:PROCESSOR_ARCHITEW6432 -eq 'AMD64') { $Arch = 'amd64' }
if ($env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { $Arch = 'arm64' }

$Version = $env:CID_VERSION
if (-not $Version) {
    $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
    $Version = $latest.tag_name
}
if (-not $Version) { throw 'Could not determine the latest release.' }
$Tag = 'v' + $Version.TrimStart('v')
$Version = $Tag.Substring(1)

$Archive = "${Bin}_${Version}_windows_${Arch}.zip"
$Base = "https://github.com/$Repo/releases/download/$Tag"
$Dest = $env:CID_INSTALL_DIR
if (-not $Dest) { $Dest = Join-Path $env:LOCALAPPDATA 'Programs\cid' }

$Work = Join-Path ([IO.Path]::GetTempPath()) ("cid-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $Work | Out-Null
try {
    Write-Host "Downloading $Archive ($Tag)..."
    Invoke-WebRequest -Uri "$Base/$Archive" -OutFile (Join-Path $Work $Archive) -UseBasicParsing
    Invoke-WebRequest -Uri "$Base/checksums.txt" -OutFile (Join-Path $Work 'checksums.txt') -UseBasicParsing

    $expected = $null
    foreach ($line in Get-Content (Join-Path $Work 'checksums.txt')) {
        $parts = $line -split '\s+'
        if ($parts.Length -ge 2 -and $parts[1] -eq $Archive) { $expected = $parts[0]; break }
    }
    if (-not $expected) { throw "No checksum entry for $Archive in checksums.txt." }
    $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $Work $Archive)).Hash.ToLowerInvariant()
    if ($expected.ToLowerInvariant() -ne $actual) {
        throw "Checksum mismatch for $Archive (expected $expected, got $actual); aborting."
    }
    Write-Host 'Checksum OK.'

    if ($env:CID_VERIFY_ATTESTATION -ne '0' -and (Get-Command gh -ErrorAction SilentlyContinue)) {
        Write-Host 'Verifying build provenance with gh...'
        & gh attestation verify (Join-Path $Work $Archive) --repo $Repo *> $null
        if ($LASTEXITCODE -ne 0) {
            throw "gh attestation verify failed for $Archive; aborting (set CID_VERIFY_ATTESTATION=0 to skip)."
        }
        Write-Host 'Attestation OK.'
    }

    Expand-Archive -Path (Join-Path $Work $Archive) -DestinationPath $Work -Force
    New-Item -ItemType Directory -Path $Dest -Force | Out-Null
    Copy-Item -Path (Join-Path $Work "$Bin.exe") -Destination (Join-Path $Dest "$Bin.exe") -Force
    Write-Host "Installed $Bin $Version to $(Join-Path $Dest "$Bin.exe")"

    if ($env:CID_NO_MODIFY_PATH -eq '1') {
        if (-not (Test-PathContains $env:Path $Dest)) {
            Write-Host "Add $Dest to your PATH to run '$Bin' from anywhere."
        }
    }
    else {
        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        if (-not (Test-PathContains $userPath $Dest)) {
            $newPath = if ($userPath) { $userPath.TrimEnd(';') + ';' + $Dest } else { $Dest }
            [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
            Write-Host "Added $Dest to your user PATH. Restart open terminals (and Claude Code) to pick it up."
        }
        if (-not (Test-PathContains $env:Path $Dest)) { $env:Path = $env:Path.TrimEnd(';') + ';' + $Dest }
    }
}
finally {
    Remove-Item -Recurse -Force -Path $Work -ErrorAction SilentlyContinue
}
