# Driftr installer for Windows: downloads a release from GitHub, verifies its
# checksum, installs driftr.exe and runs `driftr setup`, which writes the shims
# and puts them on your user PATH.
#
# Usage (PowerShell):
#   irm https://raw.githubusercontent.com/StackMade/driftr/main/install.ps1 | iex
#
# Environment variables:
#   DRIFTR_VERSION       pin a version (e.g. "1.6.0"); default: the latest release
#   DRIFTR_INSTALL_DIR   where driftr.exe goes; default: %USERPROFILE%\.driftr\bin
#   DRIFTR_UPDATE_API    release index base URL, for mirrors (as for self-update)
#   DRIFTR_UPDATE_MIRROR release download base URL, for mirrors (as for self-update)

# Everything runs inside a function: the script is usually piped into iex, and
# `exit` there would close the user's terminal.
function Install-Driftr {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue' # Windows PowerShell downloads crawl with the progress bar
    # Windows PowerShell 5.1 may still default to TLS 1.0.
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $repo = 'StackMade/driftr'
    $binDir = Join-Path $env:USERPROFILE '.driftr\bin'
    $installDir = if ($env:DRIFTR_INSTALL_DIR) { $env:DRIFTR_INSTALL_DIR } else { $binDir }
    $apiBase = if ($env:DRIFTR_UPDATE_API) { $env:DRIFTR_UPDATE_API.TrimEnd('/') } else { "https://api.github.com/repos/$repo" }
    $downloadBase = if ($env:DRIFTR_UPDATE_MIRROR) { $env:DRIFTR_UPDATE_MIRROR.TrimEnd('/') } else { "https://github.com/$repo/releases/download" }

    # A 32-bit PowerShell on 64-bit Windows reports x86 here and the real
    # architecture in PROCESSOR_ARCHITEW6432.
    $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($cpu) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { throw "unsupported architecture: $cpu" }
    }

    if ($env:DRIFTR_VERSION) {
        $version = $env:DRIFTR_VERSION -replace '^v', ''
    } else {
        $release = Invoke-RestMethod -UseBasicParsing "$apiBase/releases/latest"
        $version = $release.tag_name -replace '^v', ''
        if (-not $version) { throw 'could not determine the latest version (GitHub API rate limit?)' }
    }

    $archive = "driftr_${version}_windows_${arch}.zip"
    $baseUrl = "$downloadBase/v$version"
    Write-Host "Installing driftr v$version (windows/$arch)..."

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("driftr-install-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading $archive..."
        Invoke-WebRequest -UseBasicParsing "$baseUrl/$archive" -OutFile (Join-Path $tmp $archive)
        Invoke-WebRequest -UseBasicParsing "$baseUrl/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt')

        Write-Host 'Verifying checksum...'
        # Exact match on the file name field: a substring match could pick the wrong line.
        $expected = $null
        foreach ($line in Get-Content (Join-Path $tmp 'checksums.txt')) {
            $fields = -split $line
            if ($fields.Count -eq 2 -and $fields[1] -eq $archive) { $expected = $fields[0].ToLower(); break }
        }
        if (-not $expected) { throw "no checksum found for $archive in checksums.txt" }
        if ($expected -notmatch '^[0-9a-f]{64}$') { throw "malformed checksum for $archive in checksums.txt" }
        $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
        if ($actual -ne $expected) { throw "checksum mismatch for $archive (expected $expected, got $actual)" }

        Write-Host 'Extracting...'
        Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x') -Force

        New-Item -ItemType Directory -Force -Path $installDir | Out-Null
        $exe = Join-Path $installDir 'driftr.exe'
        if (Test-Path $exe) {
            # A running driftr.exe, a shim among them, cannot be overwritten
            # but can be renamed.
            Remove-Item "$exe.old" -Force -ErrorAction SilentlyContinue
            Move-Item $exe "$exe.old" -Force
        }
        Copy-Item (Join-Path $tmp 'x\driftr.exe') $exe
        Write-Host "Installed driftr to $exe"
    } finally {
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }

    # setup writes the shims and puts the shim directory on the user PATH.
    & $exe setup
    if ($LASTEXITCODE -ne 0) { throw "driftr setup failed with exit code $LASTEXITCODE" }

    if (-not [IO.Path]::GetFullPath($installDir).TrimEnd('\').Equals([IO.Path]::GetFullPath($binDir).TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase)) {
        Add-UserPath $installDir
    }

    Write-Host ''
    Write-Host "driftr v$version installed successfully!"
    Write-Host 'Open a new terminal, then run: driftr --version'
    Write-Host ''
    Write-Host 'To enable PowerShell completions, add this to your $PROFILE:'
    Write-Host '  driftr completion powershell | Out-String | Invoke-Expression'
}

# Add-UserPath puts dir at the front of the user Path, unless it is there. It
# writes the registry value directly, keeping its type: setting it through
# [Environment] would turn REG_EXPAND_SZ into REG_SZ and break entries such as
# %USERPROFILE%\bin.
function Add-UserPath([string]$dir) {
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
    try {
        $old = $key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $entries = @($old -split ';' | Where-Object { $_ })
        foreach ($entry in $entries) {
            if ([Environment]::ExpandEnvironmentVariables($entry).TrimEnd('\') -ieq $dir.TrimEnd('\')) { return }
        }
        $kind = if ($old) { $key.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
        $key.SetValue('Path', ((@($dir) + $entries) -join ';'), $kind)
    } finally {
        $key.Close()
    }
    # Clearing a variable that does not exist still broadcasts WM_SETTINGCHANGE,
    # so terminals opened from Explorer pick up the new Path.
    [Environment]::SetEnvironmentVariable('DRIFTR_INSTALL_PATH_REFRESH', $null, 'User')
    Write-Host "Added $dir to your user PATH"
}

Install-Driftr
