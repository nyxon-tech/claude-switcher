# Claude Switcher by Nyxon - installer for Windows
#   Install:    irm https://raw.githubusercontent.com/nyxon-tech/claude-switcher/main/install.ps1 | iex
#   Uninstall:  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/nyxon-tech/claude-switcher/main/install.ps1))) -Uninstall
# Installs for the current user only: no admin rights, nothing outside your user profile.
# The download is checked against the release's checksums.txt before anything is installed.
# This file must stay ASCII: Windows PowerShell 5.1 reads scripts without a BOM in the ANSI code page.
param(
    [switch]$Uninstall,
    [switch]$NoShortcut,
    [string]$Version = 'latest',
    [string]$Binary = ''
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$repo = 'nyxon-tech/claude-switcher'
$dir = "$env:LOCALAPPDATA\Programs\claude-switcher"
$exe = "$dir\claude-switcher.exe"
$shortcut = "$([Environment]::GetFolderPath('Programs'))\Claude Switcher.lnk"

function Set-UserPath([scriptblock]$Change) {
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    $parts = @($current -split ';' | Where-Object { $_ })
    $updated = (& $Change $parts) -join ';'
    if ($updated -ne $current) { [Environment]::SetEnvironmentVariable('Path', $updated, 'User') }
}

if ($Uninstall) {
    if (Test-Path $dir) { Remove-Item $dir -Recurse -Force }
    if (Test-Path $shortcut) { Remove-Item $shortcut -Force }
    Set-UserPath { param($parts) @($parts | Where-Object { $_ -ne $dir }) }
    Write-Host '  Claude Switcher removed.' -ForegroundColor Green
    Write-Host "  Your saved logins are still in $env:USERPROFILE\.claude-instances. Delete that folder to forget them." -ForegroundColor DarkGray
    return
}

New-Item -ItemType Directory -Path $dir -Force | Out-Null
# v2 was a PowerShell script with a .cmd shim; the exe replaces both
Remove-Item "$dir\claude-switcher.ps1", "$dir\claude-switcher.cmd" -Force -ErrorAction SilentlyContinue

if ($Binary) {
    Copy-Item $Binary $exe -Force
}
else {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') { 'arm64' } else { 'amd64' }
    $zip = "claude-switcher_windows_$arch.zip"
    $base = if ($Version -eq 'latest') { "https://github.com/$repo/releases/latest/download" } else { "https://github.com/$repo/releases/download/$Version" }
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Invoke-WebRequest -UseBasicParsing "$base/$zip" -OutFile "$tmp\$zip"
        Invoke-WebRequest -UseBasicParsing "$base/checksums.txt" -OutFile "$tmp\checksums.txt"
        $line = Select-String -Path "$tmp\checksums.txt" -Pattern ('\s' + [regex]::Escape($zip) + '$') | Select-Object -First 1
        if (-not $line) { throw "$zip is not listed in checksums.txt" }
        $want = ($line.Line -split '\s+')[0]
        $got = (Get-FileHash "$tmp\$zip" -Algorithm SHA256).Hash
        if ($got -ne $want) { throw "Checksum mismatch for $zip. Nothing was installed." }
        Expand-Archive "$tmp\$zip" -DestinationPath "$tmp\x" -Force
        Copy-Item "$tmp\x\claude-switcher.exe" $exe -Force
    }
    finally { Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue }
}
Unblock-File $exe

Set-UserPath { param($parts) if ($parts -contains $dir) { $parts } else { @($parts) + $dir } }
if (($env:Path -split ';') -notcontains $dir) { $env:Path += ";$dir" }

if (-not $NoShortcut) {
    $shell = New-Object -ComObject WScript.Shell
    $link = $shell.CreateShortcut($shortcut)
    $link.TargetPath = $exe
    $link.WorkingDirectory = $env:USERPROFILE
    $link.Description = 'Switch Claude Desktop accounts and manage your Claude Code chats'
    $link.Save()
}

$installed = & $exe version --short
Write-Host ''
Write-Host "  Claude Switcher $installed installed" -ForegroundColor Green
Write-Host '  Open a new terminal and run:  claude-switcher' -ForegroundColor White
if (-not $NoShortcut) { Write-Host '  Or open "Claude Switcher" from the Start menu.' -ForegroundColor DarkGray }
Write-Host ''
