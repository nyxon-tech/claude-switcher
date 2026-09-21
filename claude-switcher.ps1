<#
.SYNOPSIS
    Claude Desktop Profile Switcher - Switch between multiple Claude accounts
.DESCRIPTION
    Swaps all Electron session files (auth tokens, cookies, local storage, etc.)
    to switch between different Claude accounts without re-logging in.
    Keeps vm_bundles (12GB+ Cowork VM) shared across profiles.
.NOTES
    Version: 1.1.0
    Requires: Windows 10/11, Claude Desktop 1.1.x+ (Microsoft Store or standalone installer)
    Admin may be needed for Cowork VM sessiondata repair (diskpart)
#>

param(
    [Parameter(Position=0)] [string]$Action = "list",
    [Parameter(Position=1)] [string]$Name = "",
    [string]$ClaudeDir = "",
    [string]$InstanceDir = "",
    [switch]$NoRestart
)

# Microsoft Store (MSIX) installs keep app data in the package's LocalCache, not %APPDATA%
function Find-ClaudeDir {
    $store = Get-ChildItem "$env:LOCALAPPDATA\Packages" -Directory -Filter 'Claude_*' -ErrorAction SilentlyContinue |
        Where-Object { Test-Path "$($_.FullName)\LocalCache\Roaming\Claude" } | Select-Object -First 1
    if ($store) { return "$($store.FullName)\LocalCache\Roaming\Claude" }
    return "$env:APPDATA\Claude"
}

# === Config ===
$claudeDir = if ($ClaudeDir) { $ClaudeDir } else { Find-ClaudeDir }
$instanceDir = if ($InstanceDir) { $InstanceDir } else { "$env:USERPROFILE\.claude-instances" }
$currentFile = "$instanceDir\_current_profile"

# Session files to swap per profile (everything auth/session related, ~5MB total)
# These store OAuth tokens, cookies, and browser session state
$sessionFiles = @(
    "config.json",       # OAuth token
    "Preferences",       # Electron preferences
    "DIPS",              # Bounce tracking DB
    "DIPS-wal",
    "SharedStorage",     # Shared storage DB
    "SharedStorage-wal",
    "ant-did",           # Anthropic device ID
    "buddy-tokens.json", # Per-account tokens (Claude Desktop 2.x)
    "plan-usage-history.json"
)
$sessionDirs = @(
    "Local Storage",     # localStorage (auth state)
    "Session Storage",   # sessionStorage
    "Network",           # Cookies, HSTS, etc.
    "IndexedDB",         # IndexedDB databases
    "WebStorage"         # Web storage
)

# Files that are SHARED across profiles (never swapped):
#   vm_bundles/       - 12GB+ Cowork VM (file-locked by Hyper-V)
#   claude_desktop_config.json - MCP server config
#   Local State       - DPAPI encryption key
#   claude-code-sessions/ - already kept per account by Claude Desktop itself
#   Cache/, Code Cache/, GPUCache/ - runtime caches

# === Helpers ===
function Write-OK   { param($m) Write-Host "  [OK] $m" -ForegroundColor Green }
function Write-Err  { param($m) Write-Host "  [ERR] $m" -ForegroundColor Red }
function Write-Warn { param($m) Write-Host "  [!] $m" -ForegroundColor Yellow }
function Write-Info { param($m) Write-Host "  [i] $m" -ForegroundColor DarkCyan }

function Get-CurrentProfile {
    if (Test-Path $currentFile) { return (Get-Content $currentFile -Raw).Trim() }
    return $null
}

function Get-Profiles {
    $profiles = @()
    if (Test-Path $instanceDir) {
        Get-ChildItem -Path $instanceDir -Directory | Where-Object {
            $_.Name -notmatch '^_' -and (Test-Path "$($_.FullName)\config.json")
        } | ForEach-Object { $profiles += $_.Name }
    }
    return $profiles
}

# The account Claude Desktop is signed into right now (an id, not a secret)
function Get-LiveAccount {
    try { return (Get-Content "$claudeDir\config.json" -Raw | ConvertFrom-Json).lastKnownAccountUuid } catch { return $null }
}

function Get-ProfileAccount {
    param([string]$profileName)
    $file = "$instanceDir\$profileName\_account"
    if (Test-Path $file) { return (Get-Content $file -Raw).Trim() }
    return $null
}

# Refuse to save over a profile when Desktop was signed into another account by hand
function Test-CurrentMatches {
    param([string]$profileName)
    $recorded = Get-ProfileAccount $profileName
    $live = Get-LiveAccount
    if ($recorded -and $live -and $recorded -ne $live) {
        Write-Err "Claude Desktop is signed into a different account than profile '$profileName'."
        Write-Err "Nothing was changed. Save this login under its own name first: create <name>"
        return $false
    }
    return $true
}

function Save-Session {
    param([string]$profileName)
    $dest = "$instanceDir\$profileName"
    if (!(Test-Path $dest)) { New-Item -ItemType Directory -Path $dest -Force | Out-Null }

    foreach ($f in $sessionFiles) {
        $src = "$claudeDir\$f"
        if (Test-Path $src) { Copy-Item $src "$dest\$f" -Force }
        else { Remove-Item "$dest\$f" -Force -ErrorAction SilentlyContinue }
    }
    foreach ($d in $sessionDirs) {
        $src = "$claudeDir\$d"
        $dDest = "$dest\$d"
        if (Test-Path $dDest) { Remove-Item $dDest -Recurse -Force }
        if (Test-Path $src) { Copy-Item $src $dDest -Recurse -Force }
    }
    $account = Get-LiveAccount
    if ($account) { Set-Content -Path "$dest\_account" -Value $account -NoNewline }
}

# Remove every session file so nothing from the previous account lingers (stale -wal files included)
function Clear-Session {
    foreach ($f in $sessionFiles) { Remove-Item "$claudeDir\$f" -Force -ErrorAction SilentlyContinue }
    foreach ($d in $sessionDirs) { Remove-Item "$claudeDir\$d" -Recurse -Force -ErrorAction SilentlyContinue }
}

function Load-Session {
    param([string]$profileName)
    $src = "$instanceDir\$profileName"
    Clear-Session
    foreach ($f in $sessionFiles) {
        $fSrc = "$src\$f"
        if (Test-Path $fSrc) { Copy-Item $fSrc "$claudeDir\$f" -Force }
    }
    foreach ($d in $sessionDirs) {
        $dSrc = "$src\$d"
        if (Test-Path $dSrc) { Copy-Item $dSrc "$claudeDir\$d" -Recurse -Force }
    }
}

# Only the Desktop app itself; a Claude Code CLI in a terminal is also named claude.exe
function Get-DesktopProcesses {
    Get-Process -Name "Claude" -ErrorAction SilentlyContinue | Where-Object {
        -not $_.Path -or $_.Path -like '*\WindowsApps\Claude_*' -or $_.Path -like '*\AnthropicClaude\*'
    }
}

function Stop-ClaudeGracefully {
    Write-Host ""
    if ($claudeDir -ne (Find-ClaudeDir)) {
        Write-Info "Not the live Claude Desktop folder, skipping the running check"
        return $true
    }
    if (-not (Get-DesktopProcesses)) {
        Write-OK "Claude Desktop is not running"
        return $true
    }

    Write-Warn "Please close Claude Desktop manually:"
    Write-Warn "  RIGHT-CLICK system tray icon -> Quit"
    Write-Info "Waiting for Claude to exit... (no timeout)"
    Write-Host ""

    # vmwp is any Hyper-V VM (WSL2, Docker too); only wait for it when Cowork's VM exists
    $coworkVM = Test-Path "$claudeDir\vm_bundles\claudevm.bundle"
    $elapsed = 0
    while ($true) {
        Start-Sleep -Seconds 1
        $elapsed++

        $cAlive = [bool](Get-DesktopProcesses)
        $vAlive = $coworkVM -and [bool](Get-Process -Name "vmwp" -ErrorAction SilentlyContinue)

        if (-not $cAlive -and -not $vAlive) {
            Write-OK "Claude exited cleanly (${elapsed}s)"
            Start-Sleep -Seconds 2
            return $true
        }

        if ($elapsed % 30 -eq 0) {
            $status = ""
            if ($cAlive) { $status += "Claude " }
            if ($vAlive) { $status += "vmwp " }
            Write-Info "${elapsed}s - still waiting for: $status"
        }
    }
}

function Repair-CoworkVM {
    # Cowork runs in a Hyper-V VM that needs sessiondata.vhdx
    # If this file is missing, VM fails with "HCS operation failed" error
    # Fix: recreate empty VHDX with diskpart (requires admin)
    $sdPath = "$claudeDir\vm_bundles\claudevm.bundle\sessiondata.vhdx"

    # Only check if vm_bundles exists (Cowork may not be installed)
    if (-not (Test-Path "$claudeDir\vm_bundles\claudevm.bundle")) { return }

    if (-not (Test-Path $sdPath)) {
        Write-Host ""
        Write-Host "  !! sessiondata.vhdx MISSING! Auto-rebuilding..." -ForegroundColor Red -BackgroundColor Yellow
        Write-Host ""

        $dpScript = [System.IO.Path]::GetTempFileName()
        @"
create vdisk file="$sdPath" maximum=1024 type=expandable
exit
"@ | Set-Content $dpScript -Encoding ASCII
        Start-Process diskpart -ArgumentList "/s `"$dpScript`"" -Verb RunAs -Wait
        Remove-Item $dpScript -Force -ErrorAction SilentlyContinue

        if (Test-Path $sdPath) {
            Write-OK "sessiondata.vhdx rebuilt"
        } else {
            Write-Err "Failed to rebuild - run as admin: diskpart"
            Write-Err "  create vdisk file=`"$sdPath`" maximum=1024 type=expandable"
        }
    }
}

function Start-Claude {
    if ($NoRestart) { Write-Info "Start Claude Desktop yourself when ready"; return }
    # Try Microsoft Store version first, then standalone
    $storeApp = Get-AppxPackage -Name "Claude" -ErrorAction SilentlyContinue
    if ($storeApp) {
        $familyName = $storeApp.PackageFamilyName
        Start-Process "explorer.exe" "shell:AppsFolder\${familyName}!Claude"
    } else {
        $exePath = "$env:LOCALAPPDATA\AnthropicClaude\claude.exe"
        if (Test-Path $exePath) {
            Start-Process $exePath
        } else {
            Write-Err "Claude Desktop not found. Please install it first."
            return
        }
    }
    Start-Sleep -Seconds 2
    Write-OK "Claude Desktop launched!"
}

function Switch-Profile {
    param([string]$target)

    $current = Get-CurrentProfile
    $targetDir = "$instanceDir\$target"

    if (!(Test-Path "$targetDir\config.json")) {
        Write-Err "Profile '$target' not found."
        Write-Info "Available: $(( Get-Profiles ) -join ', ')"
        return
    }

    if ($current -eq $target) {
        Write-Warn "Already on profile '$target'"
        return
    }

    if ($current -and -not (Test-CurrentMatches $current)) { return }

    Write-Host "  ========================================"
    Write-Host "  Switching: $current -> $target" -ForegroundColor Cyan
    Write-Host "  ========================================"

    # Step 1: Close Claude
    $closed = Stop-ClaudeGracefully
    if (-not $closed) { return }
    Stop-Process -Name "chrome-native-host" -Force -ErrorAction SilentlyContinue

    # Step 2: Save current session
    if ($current) {
        Save-Session $current
        Write-OK "Saved session to '$current'"
    }

    # Step 3: Load target session
    Load-Session $target
    Write-OK "Loaded session from '$target'"

    # Step 4: Ensure Cowork VM sessiondata exists
    Repair-CoworkVM

    # Step 5: Update marker
    Set-Content -Path $currentFile -Value $target -NoNewline
    Write-OK "Profile set to '$target'"

    # Step 6: Launch
    Write-Host ""
    Write-Info "Starting Claude Desktop..."
    Start-Claude
    Write-Host ""
    Write-Host "  Done! Now on profile: $target" -ForegroundColor Green
    Write-Host ""
}

function New-Profile {
    param([string]$name)

    $profileDir = "$instanceDir\$name"
    if (Test-Path "$profileDir\config.json") {
        Write-Warn "Profile '$name' already exists. Overwriting..."
    }

    if (!(Test-Path "$claudeDir\config.json")) {
        Write-Err "No config.json found. Please login to Claude Desktop first."
        return
    }

    # Must close Claude to copy locked files (Cookies etc)
    $closed = Stop-ClaudeGracefully
    if (-not $closed) { return }
    Stop-Process -Name "chrome-native-host" -Force -ErrorAction SilentlyContinue

    Save-Session $name
    Set-Content -Path $currentFile -Value $name -NoNewline
    Write-OK "Created profile '$name' from current login"
    Write-OK "Active profile set to '$name'"
}

# Add another account without logging out, since logging out can invalidate the saved login
function New-EmptyProfile {
    param([string]$name)

    if (Test-Path "$instanceDir\$name\config.json") {
        Write-Err "Profile '$name' already exists. Use: switch $name"
        return
    }
    $current = Get-CurrentProfile
    if (-not $current) {
        Write-Err "Save the account you are signed into first: create <name>"
        return
    }
    if (-not (Test-CurrentMatches $current)) { return }

    $closed = Stop-ClaudeGracefully
    if (-not $closed) { return }
    Stop-Process -Name "chrome-native-host" -Force -ErrorAction SilentlyContinue

    Save-Session $current
    Write-OK "Saved session to '$current'"
    Clear-Session
    Set-Content -Path $currentFile -Value $name -NoNewline
    Write-OK "Cleared the login, profile '$name' is saved on your next switch"
    Write-Info "Sign in to the new account when Claude Desktop opens"
    Start-Claude
}

# === Main ===
Write-Host ""
Write-Host "  Claude Profile Switcher v1.1.0" -ForegroundColor White
Write-Host "  github.com/NeezerGu/claude-profile-switcher" -ForegroundColor DarkGray

if (!(Test-Path $instanceDir)) { New-Item -ItemType Directory -Path $instanceDir -Force | Out-Null }

switch ($Action.ToLower()) {
    "list" {
        $current = Get-CurrentProfile
        $profiles = Get-Profiles
        Write-Info "Claude data: $claudeDir"
        if ($profiles.Count -eq 0) {
            Write-Info "No profiles yet. Run: .\claude-switcher.ps1 create <name>"
        } else {
            Write-Host "  Profiles:"
            foreach ($p in $profiles) {
                if ($p -eq $current) {
                    Write-Host "    - $p <-- active" -ForegroundColor Green
                } else {
                    Write-Host "    - $p" -ForegroundColor White
                }
            }
        }
    }
    "current" {
        $c = Get-CurrentProfile
        if ($c) { Write-Info "Current profile: $c" }
        else { Write-Info "No profile set" }
    }
    "switch" {
        if (!$Name) { Write-Err "Usage: .\claude-switcher.ps1 switch <profile>"; return }
        Switch-Profile $Name
    }
    "create" {
        if (!$Name) { Write-Err "Usage: .\claude-switcher.ps1 create <name>"; return }
        New-Profile $Name
    }
    "new" {
        if (!$Name) { Write-Err "Usage: .\claude-switcher.ps1 new <name>"; return }
        New-EmptyProfile $Name
    }
    "repair" {
        Write-Info "Checking Cowork VM..."
        Repair-CoworkVM
        Write-OK "Check complete"
    }
    default {
        Write-Host "  Usage: .\claude-switcher.ps1 <command> [name]"
        Write-Host ""
        Write-Host "  Commands:"
        Write-Host "    create <name>   - Save current login as a profile"
        Write-Host "    new <name>      - Save current login, then open Claude signed out for another account"
        Write-Host "    switch <name>   - Switch to a profile"
        Write-Host "    list            - List all profiles"
        Write-Host "    current         - Show active profile"
        Write-Host "    repair          - Fix Cowork VM if broken"
        Write-Host ""
        Write-Host "  Quick Setup (never use Log out in Claude, it can invalidate a saved login):"
        Write-Host "    1. Login to Account A in Claude Desktop"
        Write-Host "    2. .\claude-switcher.ps1 create work"
        Write-Host "    3. .\claude-switcher.ps1 new personal   (sign in to Account B)"
        Write-Host "    4. .\claude-switcher.ps1 switch work"
    }
}
Write-Host ""
