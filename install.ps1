# Claude Switcher by Nyxon - installer
#   Install:    irm https://raw.githubusercontent.com/nyxon-tech/claude-switcher/main/install.ps1 | iex
#   Uninstall:  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/nyxon-tech/claude-switcher/main/install.ps1))) -Uninstall
# Installs for the current user only: no admin rights, nothing outside your user profile.
param(
    [switch]$Uninstall,
    [switch]$NoShortcut,
    [string]$Ref = 'main'
)
$ErrorActionPreference = 'Stop'
$repo = 'nyxon-tech/claude-switcher'
$dir = "$env:LOCALAPPDATA\Programs\claude-switcher"
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
$local = if ($PSScriptRoot) { Join-Path $PSScriptRoot 'claude-switcher.ps1' } else { '' }
if ($local -and (Test-Path $local)) {
    Copy-Item $local "$dir\claude-switcher.ps1" -Force
}
else {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -UseBasicParsing "https://raw.githubusercontent.com/$repo/$Ref/claude-switcher.ps1" -OutFile "$dir\claude-switcher.ps1"
}

# A .cmd shim so `claude-switcher` works from cmd, PowerShell and Windows Terminal without touching execution policy
@(
    '@echo off',
    'where pwsh >nul 2>nul',
    'if %errorlevel%==0 (pwsh -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0claude-switcher.ps1" %*) else (powershell -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%~dp0claude-switcher.ps1" %*)'
) | Set-Content "$dir\claude-switcher.cmd" -Encoding ASCII

Set-UserPath { param($parts) if ($parts -contains $dir) { $parts } else { @($parts) + $dir } }
if (($env:Path -split ';') -notcontains $dir) { $env:Path += ";$dir" }

if (-not $NoShortcut) {
    $shell = New-Object -ComObject WScript.Shell
    $link = $shell.CreateShortcut($shortcut)
    $link.TargetPath = "$dir\claude-switcher.cmd"
    $link.WorkingDirectory = $env:USERPROFILE
    $link.Description = 'Switch Claude Desktop accounts and move chats between them'
    $link.Save()
}

$version = & "$dir\claude-switcher.cmd" version
Write-Host ''
Write-Host "  Claude Switcher $version installed" -ForegroundColor Green
Write-Host '  Open a new terminal and run:  claude-switcher' -ForegroundColor White
if (-not $NoShortcut) { Write-Host '  Or open "Claude Switcher" from the Start menu.' -ForegroundColor DarkGray }
Write-Host ''
