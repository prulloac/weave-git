#requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = $env:WEAVE_GIT_VERSION,
    [string]$BinDir = $env:WEAVE_GIT_BIN_DIR,
    [switch]$NoPath
)

$ErrorActionPreference = "Stop"
$Repo = "prulloac/weave-git"
$Binary = "weave-git"

if (-not $Version) { $Version = "latest" }
if (-not $BinDir) {
    $BinDir = Join-Path $env:LOCALAPPDATA "Programs\weave\bin"
}
$SkipPath = $NoPath.IsPresent
if ($env:WEAVE_GIT_NO_MODIFY_PATH -eq "1") { $SkipPath = $true }

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if ($Version -eq "latest") {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ "User-Agent" = "weave-git-installer" }
    $Tag = $release.tag_name
} else {
    $Tag = $Version
    if (-not $Tag.StartsWith("v")) { $Tag = "v$Tag" }
}
$VersionNumber = $Tag.TrimStart("v")

switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { $Arch = "amd64" }
    "ARM64" { $Arch = "arm64" }
    default { throw "Unsupported architecture: $($env:PROCESSOR_ARCHITECTURE)" }
}

$Archive = "${Binary}_${VersionNumber}_windows_${Arch}.zip"
$Checksums = "${Binary}_${VersionNumber}_checksums.txt"
$Base = "https://github.com/$Repo/releases/download/$Tag"
$Temp = Join-Path ([System.IO.Path]::GetTempPath()) ("weave-git-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $Temp -Force | Out-Null

try {
    $ArchivePath = Join-Path $Temp $Archive
    $ChecksumsPath = Join-Path $Temp $Checksums
    Invoke-WebRequest -Uri "$Base/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -Uri "$Base/$Checksums" -OutFile $ChecksumsPath

    $Line = Get-Content $ChecksumsPath | Where-Object { ($_ -split '\s+')[1] -eq $Archive } | Select-Object -First 1
    if (-not $Line) { throw "Checksum not found for $Archive" }
    $Expected = ($Line -split '\s+')[0].ToLower()
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLower()
    if ($Expected -ne $Actual) { throw "Checksum verification failed for $Archive" }

    $Extract = Join-Path $Temp "extract"
    Expand-Archive -Path $ArchivePath -DestinationPath $Extract -Force
    $Source = Join-Path $Extract "$Binary.exe"
    if (-not (Test-Path $Source)) { throw "Archive does not contain $Binary.exe" }

    New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
    $Target = Join-Path $BinDir "$Binary.exe"
    Copy-Item -Path $Source -Destination $Target -Force

    if (-not $SkipPath) {
        $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if (-not $UserPath) { $UserPath = "" }
        $Parts = @($UserPath -split ";" | Where-Object { $_ })
        if ($Parts -notcontains $BinDir) {
            $NewPath = if ($UserPath) { "$BinDir;$UserPath" } else { $BinDir }
            [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        }
        $env:Path = "$BinDir;$env:Path"
        Write-Host "Added $BinDir to the user PATH. Restart your terminal to use it everywhere."
    } else {
        Write-Host "Add $BinDir to your PATH manually if needed."
    }

    Write-Host "Installed $Binary $Tag to $Target"
    & $Target --version
} finally {
    Remove-Item -Path $Temp -Recurse -Force -ErrorAction SilentlyContinue
}
