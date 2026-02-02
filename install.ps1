#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version
)

$ErrorActionPreference = "Stop"

$Repo = "zigai/zgod"
$Binary = "zgod"

function Get-Arch {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    switch ($arch) {
        "X64" { return "amd64" }
        "Arm64" { return "arm64" }
        default { throw "Unsupported architecture: $arch" }
    }
}

function Get-LatestVersion {
    $response = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    return $response.tag_name
}

function Get-Checksum {
    param([string]$File)
    $hash = Get-FileHash -Path $File -Algorithm SHA256
    return $hash.Hash.ToLower()
}

function Main {
    $arch = Get-Arch

    if ($Version) {
        $ver = $Version
    } else {
        Write-Host "Fetching latest version..."
        $ver = Get-LatestVersion
    }

    if (-not $ver) {
        throw "Failed to determine version"
    }

    $versionNum = $ver -replace "^v", ""
    $archive = "${Binary}_${versionNum}_windows_${arch}.zip"
    $url = "https://github.com/$Repo/releases/download/$ver/$archive"
    $checksumUrl = "https://github.com/$Repo/releases/download/$ver/checksums.txt"

    $tmpDir = Join-Path $env:TEMP "zgod-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

    try {
        $archivePath = Join-Path $tmpDir $archive
        $checksumPath = Join-Path $tmpDir "checksums.txt"

        Write-Host "Downloading $archive..."
        Invoke-WebRequest -Uri $url -OutFile $archivePath -UseBasicParsing

        Write-Host "Downloading checksums..."
        Invoke-WebRequest -Uri $checksumUrl -OutFile $checksumPath -UseBasicParsing

        Write-Host "Verifying checksum..."
        $checksums = Get-Content $checksumPath
        $expectedLine = $checksums | Where-Object { $_.Contains($archive) }
        if (-not $expectedLine) {
            throw "Checksum not found for $archive"
        }
        $expected = ($expectedLine -split "\s+")[0].ToLower()
        $actual = Get-Checksum -File $archivePath

        if ($expected -ne $actual) {
            throw "Checksum mismatch! Expected: $expected, Actual: $actual"
        }
        Write-Host "Checksum verified."

        Write-Host "Extracting..."
        Expand-Archive -Path $archivePath -DestinationPath $tmpDir -Force

        $installDir = Join-Path $env:USERPROFILE "bin"
        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        }

        $binaryPath = Join-Path $tmpDir "$Binary.exe"
        $destPath = Join-Path $installDir "$Binary.exe"

        Write-Host "Installing to $installDir..."
        Move-Item -Path $binaryPath -Destination $destPath -Force

        $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
        if ($userPath -notlike "*$installDir*") {
            Write-Host "Adding $installDir to user PATH..."
            $newPath = "$userPath;$installDir"
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            Write-Host ""
            Write-Host "NOTE: Restart your terminal for PATH changes to take effect."
        }

        Write-Host ""
        Write-Host "Successfully installed $Binary $ver to $destPath"

    } finally {
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Main
