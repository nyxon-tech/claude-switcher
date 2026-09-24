#Requires -Version 5.1
<#
.SYNOPSIS
    Claude Switcher by Nyxon - switch Claude Desktop accounts on Windows without signing in again,
    and move your Claude Code chats between them.
.DESCRIPTION
    Run it with no arguments for the interactive menu. Every menu action is also a command,
    see `claude-switcher help`.

    Built on NeezerGu/claude-profile-switcher (MIT). Unofficial, not affiliated with Anthropic.
.LINK
    https://github.com/nyxon-tech/claude-switcher
#>
[CmdletBinding()]
param(
    [Parameter(Position = 0)] [string]$Command = '',
    [Parameter(Position = 1)] [string]$Name = '',
    [Parameter(Position = 2)] [string]$NewName = '',
    [string]$From = '',
    [string]$To = '',
    [string[]]$Chat = @(),
    [switch]$All,
    [switch]$Yes,
    [switch]$NoRestart,
    [string]$ClaudeDir = '',
    [string]$InstanceDir = '',
    [string]$ProjectsDir = ''
)

$ErrorActionPreference = 'Stop'
$Script:Version = '2.0.1'
$Script:RepoUrl = 'https://github.com/nyxon-tech/claude-switcher'

# ------------------------------------------------------------------ discovery

# Microsoft Store (MSIX) installs keep app data in the package's LocalCache, not %APPDATA%
function Find-ClaudeDir {
    $store = Get-ChildItem "$env:LOCALAPPDATA\Packages" -Directory -Filter 'Claude_*' -ErrorAction SilentlyContinue |
        Where-Object { Test-Path "$($_.FullName)\LocalCache\Roaming\Claude" } | Select-Object -First 1
    if ($store) { return "$($store.FullName)\LocalCache\Roaming\Claude" }
    return "$env:APPDATA\Claude"
}

$Script:LiveDir = Find-ClaudeDir
$Script:Data = if ($ClaudeDir) { $ClaudeDir } else { $Script:LiveDir }
$Script:Vault = if ($InstanceDir) { $InstanceDir } else { "$env:USERPROFILE\.claude-instances" }
$Script:Projects = if ($ProjectsDir) { $ProjectsDir } else { "$env:USERPROFILE\.claude\projects" }
$Script:Sessions = "$($Script:Data)\claude-code-sessions"
$Script:CurrentFile = "$($Script:Vault)\_current_profile"
$Script:JournalDir = "$($Script:Vault)\_journal"
$Script:IsLive = ($Script:Data -eq $Script:LiveDir)
$Script:Interactive = -not ([Console]::IsInputRedirected -or [Console]::IsOutputRedirected)
$Script:Utf8 = New-Object System.Text.UTF8Encoding $false
$Script:TxIndex = $null

# Everything that makes Desktop "signed in" as one account. Swapped per profile, about 5 MB.
$Script:SessionFiles = @(
    'config.json',              # OAuth token and app settings
    'Preferences',
    'DIPS', 'DIPS-wal',
    'SharedStorage', 'SharedStorage-wal',
    'ant-did',                  # device id
    'buddy-tokens.json',        # per-account tokens, Desktop 2.x
    'plan-usage-history.json'
)
$Script:SessionDirs = @('Local Storage', 'Session Storage', 'Network', 'IndexedDB', 'WebStorage')
# Never swapped: vm_bundles (Cowork VM), claude_desktop_config.json (MCP servers), Local State (DPAPI key),
# claude-code-sessions (Desktop already keeps it per account), caches.

$Script:G = @{
    Ptr  = [string][char]0x203A
    On   = [string][char]0x25CF
    Off  = [string][char]0x25CB
    Dot  = [string][char]0x00B7
    Dash = [string][char]0x2014
}

# ------------------------------------------------------------------ output

function Say([string]$Text, [string]$Color = 'Gray') { Write-Host "  $Text" -ForegroundColor $Color }
function Ok([string]$Text) { Write-Host "  $($G.On) $Text" -ForegroundColor Green }
function Warn([string]$Text) { Write-Host "  ! $Text" -ForegroundColor Yellow }
function Fail([string]$Text) { Write-Host "  x $Text" -ForegroundColor Red }
function Info([string]$Text) { Write-Host "  $Text" -ForegroundColor DarkGray }

function Short([string]$Text) { if ($Text.Length -gt 8) { $Text.Substring(0, 8) } else { $Text } }

function Format-Ago([int64]$Ms) {
    if ($Ms -le 0) { return '' }
    $span = (Get-Date) - [DateTimeOffset]::FromUnixTimeMilliseconds($Ms).LocalDateTime
    if ($span.TotalMinutes -lt 60) { return "$([int][Math]::Max(1, $span.TotalMinutes))m ago" }
    if ($span.TotalHours -lt 24) { return "$([int]$span.TotalHours)h ago" }
    if ($span.TotalDays -lt 60) { return "$([int]$span.TotalDays)d ago" }
    return [DateTimeOffset]::FromUnixTimeMilliseconds($Ms).LocalDateTime.ToString('yyyy-MM-dd')
}

# Titles are user text: no line breaks, no astral characters that break terminal column math
function Format-Title($Text) {
    $s = (([string]$Text) -replace '[\r\n\t]+', ' ' -replace '\p{Cs}', '').Trim()
    if ($s) { $s } else { '(untitled)' }
}

# ------------------------------------------------------------------ files

function Read-Json([string]$Path) {
    try { return ([IO.File]::ReadAllText($Path, $Script:Utf8) | ConvertFrom-Json) } catch { return $null }
}

# Desktop parses its records with JSON.parse, which rejects a byte order mark
function Write-Text([string]$Path, [string]$Text) { [IO.File]::WriteAllText($Path, $Text, $Script:Utf8) }

function Read-Line([string]$Path) {
    if (Test-Path -LiteralPath $Path) { return ([IO.File]::ReadAllText($Path, $Script:Utf8)).Trim() }
    return ''
}

# ------------------------------------------------------------------ profiles

function Get-LiveAccount {
    $config = Read-Json "$($Script:Data)\config.json"
    if ($config -and $config.lastKnownAccountUuid) { return [string]$config.lastKnownAccountUuid }
    return ''
}

function Get-CurrentProfile { Read-Line $Script:CurrentFile }

function Get-Profiles {
    if (-not (Test-Path $Script:Vault)) { return @() }
    @(Get-ChildItem $Script:Vault -Directory | Where-Object {
        $_.Name -notmatch '^_' -and (Test-Path "$($_.FullName)\config.json")
    } | ForEach-Object {
        [pscustomobject]@{
            Name    = $_.Name
            Account = Read-Line "$($_.FullName)\_account"
            Dir     = $_.FullName
            Saved   = (Get-Item "$($_.FullName)\config.json").LastWriteTime
        }
    })
}

function Find-Profile([string]$ProfileName) {
    @(Get-Profiles | Where-Object { $_.Name -eq $ProfileName }) | Select-Object -First 1
}

function Test-ProfileName([string]$ProfileName) {
    if ($ProfileName -match '^[\p{L}\p{N}](?:[\p{L}\p{N} ._-]{0,38}[\p{L}\p{N}_-])?$') { return $true }
    Fail "Use letters, digits, spaces, dot, dash or underscore for a profile name (up to 40), not '$ProfileName'."
    return $false
}

# A profile is only saved over when Desktop is still signed into that profile's account
function Test-CurrentMatches([string]$ProfileName) {
    $recorded = Read-Line "$($Script:Vault)\$ProfileName\_account"
    $live = Get-LiveAccount
    if ($recorded -and $live -and $recorded -ne $live) {
        Fail "Claude Desktop is signed into a different account than profile '$ProfileName'."
        Fail 'Nothing was changed. Save this login under its own name first: claude-switcher save <name>'
        return $false
    }
    return $true
}

function Save-Session([string]$ProfileName) {
    $dest = "$($Script:Vault)\$ProfileName"
    New-Item -ItemType Directory -Path $dest -Force | Out-Null
    foreach ($f in $Script:SessionFiles) {
        $src = "$($Script:Data)\$f"
        if (Test-Path -LiteralPath $src) { Copy-Item -LiteralPath $src "$dest\$f" -Force }
        else { Remove-Item -LiteralPath "$dest\$f" -Force -ErrorAction SilentlyContinue }
    }
    foreach ($d in $Script:SessionDirs) {
        if (Test-Path -LiteralPath "$dest\$d") { Remove-Item -LiteralPath "$dest\$d" -Recurse -Force }
        if (Test-Path -LiteralPath "$($Script:Data)\$d") { Copy-Item -LiteralPath "$($Script:Data)\$d" "$dest\$d" -Recurse -Force }
    }
    $account = Get-LiveAccount
    if ($account) { Write-Text "$dest\_account" $account }
}

# Remove every session file so nothing from the previous account lingers (stale -wal files included)
function Clear-Session {
    foreach ($f in $Script:SessionFiles) { Remove-Item -LiteralPath "$($Script:Data)\$f" -Force -ErrorAction SilentlyContinue }
    foreach ($d in $Script:SessionDirs) { Remove-Item -LiteralPath "$($Script:Data)\$d" -Recurse -Force -ErrorAction SilentlyContinue }
}

function Restore-Session([string]$ProfileName) {
    $src = "$($Script:Vault)\$ProfileName"
    Clear-Session
    foreach ($f in $Script:SessionFiles) {
        if (Test-Path -LiteralPath "$src\$f") { Copy-Item -LiteralPath "$src\$f" "$($Script:Data)\$f" -Force }
    }
    foreach ($d in $Script:SessionDirs) {
        if (Test-Path -LiteralPath "$src\$d") { Copy-Item -LiteralPath "$src\$d" "$($Script:Data)\$d" -Recurse -Force }
    }
}

function Set-CurrentProfile([string]$ProfileName) {
    New-Item -ItemType Directory -Path $Script:Vault -Force | Out-Null
    Write-Text $Script:CurrentFile $ProfileName
}

# ------------------------------------------------------------------ Claude Desktop process

# Only the Desktop app itself; a Claude Code CLI in a terminal is also named claude.exe
function Get-DesktopProcesses {
    @(Get-Process -Name 'Claude' -ErrorAction SilentlyContinue | Where-Object {
        -not $_.Path -or $_.Path -like '*\WindowsApps\Claude_*' -or $_.Path -like '*\AnthropicClaude\*'
    })
}

function Test-ClaudeRunning { $Script:IsLive -and (Get-DesktopProcesses).Count -gt 0 }

function Wait-ClaudeClosed {
    if (-not $Script:IsLive -or (Get-DesktopProcesses).Count -eq 0) { return $true }
    Write-Host ''
    Warn 'Claude Desktop is open. Quit it from the tray: right-click the Claude icon next to the clock, then Quit.'
    if ($Script:Interactive) { Info 'Waiting for it to close. Press Esc to cancel.' } else { Info 'Waiting for it to close.' }
    # vmwp belongs to every Hyper-V VM (WSL2, Docker too); only wait for it when Cowork's VM exists
    $coworkVM = Test-Path "$($Script:Data)\vm_bundles\claudevm.bundle"
    while ($true) {
        Start-Sleep -Milliseconds 500
        if ($Script:Interactive -and [Console]::KeyAvailable -and [Console]::ReadKey($true).Key -eq 'Escape') {
            Warn 'Cancelled, nothing was changed.'
            return $false
        }
        $busy = (Get-DesktopProcesses).Count -gt 0 -or ($coworkVM -and (Get-Process -Name 'vmwp' -ErrorAction SilentlyContinue))
        if (-not $busy) {
            Start-Sleep -Seconds 2
            Ok 'Claude Desktop is closed'
            return $true
        }
    }
}

function Start-Claude {
    if ($NoRestart -or -not $Script:IsLive) { return }
    $store = Get-AppxPackage -Name 'Claude' -ErrorAction SilentlyContinue
    if ($store) { Start-Process 'explorer.exe' "shell:AppsFolder\$($store.PackageFamilyName)!Claude" }
    elseif (Test-Path "$env:LOCALAPPDATA\AnthropicClaude\claude.exe") { Start-Process "$env:LOCALAPPDATA\AnthropicClaude\claude.exe" }
    else { Warn 'Could not find Claude Desktop to reopen it. Start it yourself.'; return }
    Ok 'Reopening Claude Desktop'
}

# Cowork's Hyper-V VM fails with "HCS operation failed" when sessiondata.vhdx is missing
function Repair-CoworkVM {
    $bundle = "$($Script:Data)\vm_bundles\claudevm.bundle"
    $disk = "$bundle\sessiondata.vhdx"
    if (-not (Test-Path $bundle) -or (Test-Path $disk)) { return }
    Warn 'Cowork sessiondata.vhdx is missing, rebuilding it (asks for admin)'
    $script = [IO.Path]::GetTempFileName()
    "create vdisk file=`"$disk`" maximum=1024 type=expandable`nexit" | Set-Content $script -Encoding ASCII
    Start-Process diskpart -ArgumentList "/s `"$script`"" -Verb RunAs -Wait
    Remove-Item $script -Force -ErrorAction SilentlyContinue
    if (Test-Path $disk) { Ok 'sessiondata.vhdx rebuilt' } else { Fail "Rebuild failed. As admin, run diskpart: create vdisk file=`"$disk`" maximum=1024 type=expandable" }
}

function Confirm-Action([string]$Question) {
    if ($Yes) { return $true }
    if (-not $Script:Interactive) { Fail "$Question Pass -Yes to confirm when not running in a terminal."; return $false }
    Write-Host "  $Question " -ForegroundColor White -NoNewline
    Write-Host '[Y/n] ' -ForegroundColor DarkGray -NoNewline
    $key = [Console]::ReadKey($true)
    $agreed = $key.Key -eq 'Enter' -or $key.KeyChar -eq 'y' -or $key.KeyChar -eq 'Y'
    if ($agreed) { Write-Host 'yes' -ForegroundColor Green } else { Write-Host 'no' -ForegroundColor DarkGray }
    return $agreed
}

# ------------------------------------------------------------------ profile actions

function Invoke-Save([string]$ProfileName, [switch]$StayClosed) {
    if (-not (Test-ProfileName $ProfileName)) { return }
    if (-not (Test-Path "$($Script:Data)\config.json")) { Fail 'Claude Desktop is not signed in. Sign in first, then save.'; return }
    $live = Get-LiveAccount
    $existing = Find-Profile $ProfileName
    if ($existing -and $existing.Account -and $live -and $existing.Account -ne $live) {
        if (-not (Confirm-Action "Profile '$ProfileName' belongs to another account. Replace it with the one signed in now?")) { return }
    }
    if (-not (Wait-ClaudeClosed)) { return }
    Stop-Process -Name 'chrome-native-host' -Force -ErrorAction SilentlyContinue
    Save-Session $ProfileName
    Set-CurrentProfile $ProfileName
    Ok "Saved the signed-in account as '$ProfileName'"
    if (-not $StayClosed) { Start-Claude }
}

# Add another account without logging out, because logging out can invalidate a saved login
function Invoke-New([string]$ProfileName) {
    if (-not (Test-ProfileName $ProfileName)) { return }
    if (Find-Profile $ProfileName) { Fail "Profile '$ProfileName' already exists. Switch to it: claude-switcher switch $ProfileName"; return }
    $current = Get-CurrentProfile
    if (-not $current -or -not (Find-Profile $current)) {
        Fail 'Save the account you are signed into first: claude-switcher save <name>'
        return
    }
    if (-not (Test-CurrentMatches $current)) { return }
    if (-not (Wait-ClaudeClosed)) { return }
    Stop-Process -Name 'chrome-native-host' -Force -ErrorAction SilentlyContinue
    Save-Session $current
    Ok "Kept '$current' safe"
    Clear-Session
    Set-CurrentProfile $ProfileName
    Ok "Claude Desktop will open signed out. Sign in to the account for '$ProfileName'."
    Info "It is saved as '$ProfileName' the next time you switch."
    Start-Claude
}

function Invoke-Switch([string]$Target) {
    $profile = Find-Profile $Target
    if (-not $profile) {
        Fail "No profile named '$Target'."
        $names = @(Get-Profiles | ForEach-Object { $_.Name })
        if ($names) { Info "Profiles: $($names -join ', ')" }
        return
    }
    $current = Get-CurrentProfile
    if ($current -eq $Target) { Warn "Already on '$Target'."; return }
    if ($current -and -not (Test-CurrentMatches $current)) { return }
    Say "Switching $(if ($current) { $current } else { '(unsaved)' }) -> $Target" 'Cyan'
    if (-not (Wait-ClaudeClosed)) { return }
    Stop-Process -Name 'chrome-native-host' -Force -ErrorAction SilentlyContinue
    if ($current) { Save-Session $current; Ok "Saved '$current'" }
    Restore-Session $Target
    Repair-CoworkVM
    Set-CurrentProfile $Target
    Ok "Signed in as '$Target'"
    Start-Claude
}

function Invoke-Rename([string]$Old, [string]$New) {
    if (-not (Find-Profile $Old)) { Fail "No profile named '$Old'."; return }
    if (-not (Test-ProfileName $New)) { return }
    if (Find-Profile $New) { Fail "A profile named '$New' already exists."; return }
    Rename-Item -LiteralPath "$($Script:Vault)\$Old" $New
    if ((Get-CurrentProfile) -eq $Old) { Set-CurrentProfile $New }
    Ok "Renamed '$Old' to '$New'"
}

function Invoke-Remove([string]$ProfileName) {
    $profile = Find-Profile $ProfileName
    if (-not $profile) { Fail "No profile named '$ProfileName'."; return }
    if ((Get-CurrentProfile) -eq $ProfileName) { Fail "'$ProfileName' is the account in use. Switch to another profile first."; return }
    if (-not (Confirm-Action "Remove the saved login '$ProfileName'? Its chats stay in Claude Desktop.")) { return }
    Remove-Item -LiteralPath $profile.Dir -Recurse -Force
    Ok "Removed '$ProfileName'"
}

# ------------------------------------------------------------------ chats

function Get-TranscriptIndex {
    if ($null -ne $Script:TxIndex) { return $Script:TxIndex }
    $Script:TxIndex = @{}
    if (Test-Path $Script:Projects) {
        foreach ($file in [IO.Directory]::EnumerateFiles($Script:Projects, '*.jsonl', [IO.SearchOption]::AllDirectories)) {
            $id = [IO.Path]::GetFileNameWithoutExtension($file)
            if ($id -match '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$' -and -not $Script:TxIndex.ContainsKey($id)) {
                $Script:TxIndex[$id] = $file
            }
        }
    }
    return $Script:TxIndex
}

# One chat list per account and organization, the way Desktop stores them
function Get-Spaces {
    $out = @()
    if (-not (Test-Path $Script:Sessions)) { return $out }
    $profiles = @(Get-Profiles)
    $live = Get-LiveAccount
    foreach ($account in @(Get-ChildItem $Script:Sessions -Directory -Force)) {
        $orgs = @(Get-ChildItem $account.FullName -Directory -Force)
        foreach ($org in $orgs) {
            $owners = @($profiles | Where-Object { $_.Account -eq $account.Name } | ForEach-Object { $_.Name })
            $label = if ($owners.Count) { $owners -join ', ' } else { "account $(Short $account.Name)" }
            if ($orgs.Count -gt 1) { $label += " $($G.Dot) org $(Short $org.Name)" }
            $out += [pscustomobject]@{
                Account  = $account.Name
                Org      = $org.Name
                Dir      = $org.FullName
                Label    = $label
                Chats    = @(Get-ChildItem $org.FullName -Filter 'local_*.json' -File -ErrorAction SilentlyContinue).Count
                SignedIn = ($account.Name -eq $live)
                Linked   = [bool](($account.Attributes -bor $org.Attributes) -band [IO.FileAttributes]::ReparsePoint)
            }
        }
    }
    return $out
}

function Get-Chats([string]$Dir) {
    $index = Get-TranscriptIndex
    $chats = foreach ($file in @(Get-ChildItem $Dir -Filter 'local_*.json' -File -ErrorAction SilentlyContinue)) {
        $card = Read-Json $file.FullName
        if (-not $card) { continue }
        $last = 0
        if ($card.lastActivityAt) { $last = [int64]$card.lastActivityAt } elseif ($card.createdAt) { $last = [int64]$card.createdAt }
        $cwd = [string]$card.cwd
        [pscustomobject]@{
            Id         = $file.BaseName
            File       = $file.FullName
            Session    = [string]$card.cliSessionId
            Title      = Format-Title $card.title
            Project    = if ($cwd) { Split-Path $cwd -Leaf } else { '' }
            Last       = $last
            Archived   = [bool]$card.isArchived
            HasHistory = (-not $card.cliSessionId) -or $index.ContainsKey([string]$card.cliSessionId)
        }
    }
    @($chats | Sort-Object Last -Descending)
}

function Resolve-Space([string]$Ref, [object[]]$Spaces) {
    if (-not $Ref) { return $null }
    if ($Ref -eq 'signed-in') { $hits = @($Spaces | Where-Object { $_.SignedIn }) }
    else {
        $profile = Find-Profile $Ref
        if ($profile -and $profile.Account) { $hits = @($Spaces | Where-Object { $_.Account -eq $profile.Account }) }
        else {
            $account, $org = $Ref -split '/', 2
            $hits = @($Spaces | Where-Object { $_.Account.StartsWith($account, 'OrdinalIgnoreCase') -and (-not $org -or $_.Org.StartsWith($org, 'OrdinalIgnoreCase')) })
        }
    }
    if ($hits.Count -eq 1) { return $hits[0] }
    if ($hits.Count -gt 1) { Fail "'$Ref' matches several chat lists, add the org: account/org"; return $null }
    Fail "No account matches '$Ref'. See: claude-switcher accounts"
    return $null
}

function Resolve-Chats([string[]]$Refs, [object[]]$Chats) {
    $picked = @()
    foreach ($ref in @($Refs | ForEach-Object { $_ -split ',' } | Where-Object { $_ })) {
        $hits = @($Chats | Where-Object { $_.Id.StartsWith($ref, 'OrdinalIgnoreCase') -or $_.Id.StartsWith("local_$ref", 'OrdinalIgnoreCase') -or ($_.Session -and $_.Session.StartsWith($ref, 'OrdinalIgnoreCase')) })
        if ($hits.Count -ne 1) { Fail "'$ref' matches $($hits.Count) chats, use a longer id."; return $null }
        $picked += $hits[0]
    }
    return $picked
}

# ------------------------------------------------------------------ journal (undo)

function New-Journal([string]$Action) {
    $dir = "$($Script:JournalDir)\$(Get-Date -Format 'yyyyMMdd-HHmmss-fff')"
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    [pscustomobject]@{ Action = $Action; Dir = $dir; Entries = New-Object System.Collections.ArrayList }
}

function Add-JournalCreated($Journal, [string]$Path) {
    [void]$Journal.Entries.Add([pscustomobject]@{ Op = 'created'; Path = $Path; Backup = '' })
}

function Add-JournalRemoved($Journal, [string]$Path) {
    $backup = "$($Journal.Dir)\$($Journal.Entries.Count)-$([IO.Path]::GetFileName($Path))"
    Copy-Item -LiteralPath $Path $backup -Force
    [void]$Journal.Entries.Add([pscustomobject]@{ Op = 'removed'; Path = $Path; Backup = $backup })
}

function Save-Journal($Journal, [string]$Summary) {
    $doc = [ordered]@{ action = $Journal.Action; at = (Get-Date).ToString('o'); summary = $Summary; entries = @($Journal.Entries) }
    Write-Text "$($Journal.Dir)\journal.json" ($doc | ConvertTo-Json -Depth 5)
}

function Get-LastJournal {
    if (-not (Test-Path $Script:JournalDir)) { return $null }
    $dir = Get-ChildItem $Script:JournalDir -Directory | Where-Object {
        (Test-Path "$($_.FullName)\journal.json") -and -not (Test-Path "$($_.FullName)\undone")
    } | Sort-Object Name -Descending | Select-Object -First 1
    if (-not $dir) { return $null }
    $journal = Read-Json "$($dir.FullName)\journal.json"
    $journal | Add-Member -NotePropertyName Dir -NotePropertyValue $dir.FullName -Force
    return $journal
}

function Invoke-Undo {
    $journal = Get-LastJournal
    if (-not $journal) { Info 'Nothing to undo.'; return }
    Say "Last change: $($journal.summary)" 'White'
    if (-not (Confirm-Action 'Undo it?')) { return }
    if (-not (Wait-ClaudeClosed)) { return }
    $entries = @($journal.entries)
    [array]::Reverse($entries)
    foreach ($entry in $entries) {
        if ($entry.Op -eq 'created') { Remove-Item -LiteralPath $entry.Path -Force -ErrorAction SilentlyContinue }
        else {
            New-Item -ItemType Directory -Path (Split-Path $entry.Path) -Force | Out-Null
            Copy-Item -LiteralPath $entry.Backup $entry.Path -Force
        }
    }
    Write-Text "$($journal.Dir)\undone" (Get-Date).ToString('o')
    Ok "Undone: $($journal.summary)"
    Start-Claude
}

# ------------------------------------------------------------------ chat actions

function Invoke-Transfer($Source, $Target, [object[]]$Chats, [bool]$MoveThem) {
    if (-not $Chats -or $Chats.Count -eq 0) { Info 'No chats selected.'; return }
    if ($Source.Dir -eq $Target.Dir) { Fail 'Pick two different accounts.'; return }
    $verb = if ($MoveThem) { 'Moved' } else { 'Copied' }
    if (-not (Confirm-Action "$(if ($MoveThem) { 'Move' } else { 'Copy' }) $($Chats.Count) chat(s) from $($Source.Label) to $($Target.Label)?")) { return }
    if (-not (Wait-ClaudeClosed)) { return }
    $journal = New-Journal $(if ($MoveThem) { 'move' } else { 'copy' })
    $written = 0; $present = 0
    foreach ($chat in $Chats) {
        $dest = "$($Target.Dir)\$($chat.Id).json"
        if (Test-Path -LiteralPath $dest) { $present++ }
        else {
            Copy-Item -LiteralPath $chat.File $dest
            Add-JournalCreated $journal $dest
            $written++
        }
        if ($MoveThem) {
            Add-JournalRemoved $journal $chat.File
            Remove-Item -LiteralPath $chat.File -Force
        }
    }
    $summary = "$verb $($Chats.Count) chat(s) from $($Source.Label) to $($Target.Label)"
    Save-Journal $journal $summary
    Ok $summary
    if ($present) { Info "$present were already in $($Target.Label) and were left as they are." }
    Info 'Undo with: claude-switcher undo'
    Start-Claude
}

# Copy every chat the target does not have yet, newest copy wins when several accounts hold it
function Invoke-Merge($Target, [object[]]$Spaces) {
    $have = @{}
    foreach ($f in @(Get-ChildItem $Target.Dir -Filter 'local_*.json' -File -ErrorAction SilentlyContinue)) { $have[$f.Name] = $true }
    $newest = @{}
    foreach ($space in @($Spaces | Where-Object { $_.Dir -ne $Target.Dir })) {
        foreach ($f in @(Get-ChildItem $space.Dir -Filter 'local_*.json' -File -ErrorAction SilentlyContinue)) {
            if ($have.ContainsKey($f.Name)) { continue }
            if (-not $newest.ContainsKey($f.Name) -or $f.LastWriteTime -gt $newest[$f.Name].LastWriteTime) { $newest[$f.Name] = $f }
        }
    }
    if ($newest.Count -eq 0) { Ok "$($Target.Label) already has every chat."; return }
    if (-not (Confirm-Action "Copy $($newest.Count) chat(s) into $($Target.Label)?")) { return }
    if (-not (Wait-ClaudeClosed)) { return }
    $journal = New-Journal 'merge'
    foreach ($f in $newest.Values) {
        $dest = "$($Target.Dir)\$($f.Name)"
        Copy-Item -LiteralPath $f.FullName $dest
        Add-JournalCreated $journal $dest
    }
    $summary = "Copied $($newest.Count) chat(s) into $($Target.Label)"
    Save-Journal $journal $summary
    Ok $summary
    Info 'Undo with: claude-switcher undo'
    Start-Claude
}

# The value of the first (or last) "key":"..." in a transcript. Ordinal search plus a walk to the closing quote:
# a regex match, even anchored with \G, scans the rest of a 50 MB string and takes seconds.
function Find-Value([string]$Text, [string]$Key, [switch]$Last) {
    $marker = '"' + $Key + '":"'
    $at = if ($Last) { $Text.LastIndexOf($marker, [StringComparison]::Ordinal) } else { $Text.IndexOf($marker, [StringComparison]::Ordinal) }
    if ($at -lt 0) { return $null }
    $open = $at + $marker.Length - 1
    $close = $open
    while ($true) {
        $close = $Text.IndexOf([char]'"', $close + 1)
        if ($close -lt 0) { return $null }
        $escapes = 0
        for ($k = $close - 1; $k -gt $open -and $Text[$k] -eq [char]'\'; $k--) { $escapes++ }
        if ($escapes % 2 -eq 0) { break }
    }
    try { return [regex]::Unescape($Text.Substring($open + 1, $close - $open - 1)) } catch { return $null }
}

# Reads what a sidebar record needs from a transcript. Files reach tens of megabytes, so no line-by-line parsing
# except the first few user turns when there is no custom title.
function Read-Transcript([string]$Path, [string]$Session, [string]$Text = '') {
    $text = if ($Text) { $Text } else { [IO.File]::ReadAllText($Path, $Script:Utf8) }
    $firstStamp = Find-Value $text 'timestamp'
    $cwd = Find-Value $text 'cwd'
    if (-not $firstStamp -or -not $cwd) { return $null }
    $lastStamp = Find-Value $text 'timestamp' -Last
    $title = Find-Value $text 'customTitle' -Last
    $model = Find-Value $text 'model' -Last
    if ($model -notlike 'claude-*') { $model = $null }
    if (-not $title) {
        $seen = 0
        foreach ($line in [IO.File]::ReadLines($Path)) {
            if (++$seen -gt 400) { break }
            if (-not $line.Contains('"type":"user"')) { continue }
            try {
                $entry = $line | ConvertFrom-Json
                $content = $entry.message.content
                $asked = if ($content -is [string]) { $content } else { (@($content) | Where-Object { $_.type -eq 'text' } | ForEach-Object { $_.text }) -join ' ' }
                $asked = ([string]$asked).Trim()
                if ($asked -and -not $entry.isMeta -and -not $asked.StartsWith('<')) { $title = $asked; break }
            } catch {}
        }
    }
    if (-not $title) { $title = 'Recovered chat' }
    $title = Format-Title $title
    $key = Get-ChatKey $title $cwd
    if ($title.Length -gt 80) { $title = $title.Substring(0, 79) + [char]0x2026 }
    $culture = [Globalization.CultureInfo]::InvariantCulture
    [pscustomobject]@{
        Session = $Session; Path = $Path; Title = $title; Key = $key; Cwd = $cwd; Project = Split-Path $cwd -Leaf; Model = $model
        First   = [DateTimeOffset]::Parse($firstStamp, $culture).ToUnixTimeMilliseconds()
        Last    = [DateTimeOffset]::Parse($lastStamp, $culture).ToUnixTimeMilliseconds()
    }
}

# Desktop titles every transcript of a chat with the chat's title, so title and project identify the chat
function Get-ChatKey($Title, $Cwd) { ("$(Format-Title $Title)|$Cwd").ToLowerInvariant() }

# The message a transcript's first message answers: empty for a fresh start, otherwise a message
# in the earlier transcript this one continues
function Get-FirstParent([string]$Text) {
    $at = $Text.IndexOf('"parentUuid":', [StringComparison]::Ordinal)
    if ($at -lt 0 -or $Text.Length -lt $at + 51 -or $Text[$at + 13] -ne '"') { return '' }
    return $Text.Substring($at + 14, 36)
}

function Get-HeadParent([string]$Path) {
    $seen = 0
    foreach ($line in [IO.File]::ReadLines($Path)) {
        if ($line.Contains('"parentUuid":')) { return Get-FirstParent $line }
        if (++$seen -gt 200) { break }
    }
    return ''
}

# Chats whose history is on disk but that no account lists. One Desktop chat spans several transcripts
# (a /clear, a restart or a resume starts a new one under the same title) and its record points only at
# the newest, so a transcript counts as lost only when no record carries its title in its project, no
# other transcript continues it, and the chat was not deleted in the app. Each lost chat is offered once,
# at its newest transcript.
function Get-Orphans {
    $index = Get-TranscriptIndex
    $carded = @{}; $titled = @{}; $deleted = @{}
    foreach ($space in Get-Spaces) {
        foreach ($f in @(Get-ChildItem $space.Dir -File -ErrorAction SilentlyContinue)) {
            if ($f.Name.StartsWith('deleted_')) { $deleted[$f.Name.Substring(8)] = $true; continue }
            if (-not ($f.Name.StartsWith('local_') -and $f.Name.EndsWith('.json'))) { continue }
            $card = Read-Json $f.FullName
            if (-not $card) { continue }
            if ($card.cliSessionId) { $carded[[string]$card.cliSessionId] = $true }
            if ($card.title) { $titled[(Get-ChatKey $card.title $card.cwd)] = $true }
        }
    }
    $candidates = @($index.Keys | Where-Object { -not $carded.ContainsKey($_) -and -not $deleted.ContainsKey($_) })

    # every transcript's first message names the message it continues from, if any
    $parent = @{}
    foreach ($session in @($candidates + @($carded.Keys | Where-Object { $index.ContainsKey($_) }))) {
        $p = Get-HeadParent $index[$session]
        if ($p) { $parent[$session] = $p }
    }
    $wanted = @($parent.Values | Sort-Object -Unique)
    $holds = if ($wanted.Count) { [regex]('"uuid":"(' + ($wanted -join '|') + ')"') } else { $null }

    $older = @{}
    $found = foreach ($session in $candidates) {
        $text = [IO.File]::ReadAllText($index[$session], $Script:Utf8)
        if ($holds) {
            foreach ($m in $holds.Matches($text)) {
                if ($parent[$session] -ne $m.Groups[1].Value) { $older[$session] = $true; break }
            }
        }
        Read-Transcript $index[$session] $session $text
    }
    $found = @($found | Where-Object { $_ })
    $lost = @($found | Where-Object { -not $older.ContainsKey($_.Session) -and -not $titled.ContainsKey($_.Key) })
    $chats = @(foreach ($group in @($lost | Group-Object Key)) { @($group.Group | Sort-Object Last -Descending)[0] })
    $Script:HiddenParts = $found.Count - $chats.Count
    @($chats | Sort-Object Last -Descending)
}

function New-Card($Transcript) {
    $card = [ordered]@{
        sessionId                = "local_$([guid]::NewGuid())"
        cliSessionId             = $Transcript.Session
        cwd                      = $Transcript.Cwd
        originCwd                = $Transcript.Cwd
        lastFocusedAt            = $Transcript.Last
        createdAt                = $Transcript.First
        lastActivityAt           = $Transcript.Last
        isArchived               = $false
        title                    = $Transcript.Title
        titleSource              = 'auto'
        permissionMode           = 'default'
        remoteMcpServersConfig   = @()
        alwaysAllowedReasons     = @()
        sessionPermissionUpdates = @()
        spawnSeed                = @{}
    }
    if ($Transcript.Model) { $card['model'] = $Transcript.Model }
    return $card
}

function Invoke-Rescue($Target, [object[]]$Transcripts) {
    if (-not $Transcripts -or $Transcripts.Count -eq 0) { Info 'No chats selected.'; return }
    if (-not (Confirm-Action "Recover $($Transcripts.Count) chat(s) into $($Target.Label)?")) { return }
    if (-not (Wait-ClaudeClosed)) { return }
    $journal = New-Journal 'rescue'
    foreach ($t in $Transcripts) {
        $card = New-Card $t
        $dest = "$($Target.Dir)\$($card.sessionId).json"
        Write-Text $dest ($card | ConvertTo-Json -Depth 5 -Compress)
        Add-JournalCreated $journal $dest
    }
    $summary = "Recovered $($Transcripts.Count) chat(s) into $($Target.Label)"
    Save-Journal $journal $summary
    Ok $summary
    Info 'Undo with: claude-switcher undo'
    Start-Claude
}

# ------------------------------------------------------------------ doctor

function Invoke-Doctor {
    $issues = 0
    Say "Claude Switcher $($Script:Version)" 'White'
    Write-Host ''
    $kind = if ($Script:Data -like '*\Packages\Claude_*') { 'Microsoft Store install' } else { 'Standalone install' }
    if (Test-Path "$($Script:Data)\config.json") { Ok "$kind found" } else { Warn "$kind folder has no login yet: $($Script:Data)"; $issues++ }
    Info $Script:Data
    if ($Script:IsLive) {
        if (Test-ClaudeRunning) { Info 'Claude Desktop is running' } else { Info 'Claude Desktop is closed' }
    }

    $live = Get-LiveAccount
    $profiles = @(Get-Profiles)
    $current = Get-CurrentProfile
    $owner = @($profiles | Where-Object { $_.Account -and $_.Account -eq $live })
    if ($owner) { Ok "Signed in as profile '$($owner[0].Name)'" }
    elseif ($live) { Warn "The signed-in account ($(Short $live)) is not saved yet. Save it: claude-switcher save <name>"; $issues++ }
    if ($current -and $owner -and $owner[0].Name -ne $current) {
        Warn "The active profile says '$current' but Desktop is signed in as '$($owner[0].Name)'. Switching will refuse until this matches."
        $issues++
    }
    Info "$($profiles.Count) saved profile(s)$(if ($profiles) { ': ' + (($profiles | ForEach-Object { $_.Name }) -join ', ') })"

    Write-Host ''
    $index = Get-TranscriptIndex
    foreach ($space in Get-Spaces) {
        $chats = @(Get-Chats $space.Dir)
        $ghosts = @($chats | Where-Object { -not $_.HasHistory }).Count
        $line = "$($space.Label): $($chats.Count) chat(s)"
        if ($space.SignedIn) { $line += ' (signed in)' }
        if ($space.Linked) {
            Fail "$line are behind a junction or symlink. Desktop reads through it but never writes, so new chats vanish."
            Info '  Replace the link with a real folder holding a copy of the chats.'
            $issues++
        }
        else { Ok $line }
        if ($ghosts) { Info "  $ghosts of them have no history left on disk and open empty." }
    }
    $orphans = @($index.Keys).Count - @(Get-Spaces | ForEach-Object { Get-Chats $_.Dir } | Where-Object { $_.HasHistory -and $_.Session } | ForEach-Object { $_.Session } | Sort-Object -Unique).Count
    if ($orphans -gt 0) { Warn "$orphans chat history file(s) are missing from every sidebar. Recover them: claude-switcher rescue" }

    $settings = Read-Json "$env:USERPROFILE\.claude\settings.json"
    if (-not $settings -or -not $settings.cleanupPeriodDays) {
        Write-Host ''
        Warn 'Claude Code deletes chat history it has not touched for 30 days.'
        Info '  To keep it, add "cleanupPeriodDays": 3650 to %USERPROFILE%\.claude\settings.json'
    }
    Write-Host ''
    if ($issues) { Warn "$issues thing(s) need attention" } else { Ok 'Everything looks right' }
}

# ------------------------------------------------------------------ interactive menu

# Builds one screen of the picker as lines of (text, color) segments. Kept apart from drawing
# so the README frames are rendered by the same code the menu runs.
function Format-PickerFrame($State) {
    $items = $State.Items
    $view = @(for ($i = 0; $i -lt $items.Count; $i++) {
        $f = $State.Filter
        if (-not $f -or ([string]$items[$i].Label).IndexOf($f, [StringComparison]::OrdinalIgnoreCase) -ge 0 -or ([string]$items[$i].Detail).IndexOf($f, [StringComparison]::OrdinalIgnoreCase) -ge 0) { $i }
    })
    if ($State.Cursor -ge $view.Count) { $State.Cursor = [Math]::Max(0, $view.Count - 1) }
    $rows = $State.Height - 8
    if ($State.Cursor -lt $State.Top) { $State.Top = $State.Cursor }
    if ($State.Cursor -ge $State.Top + $rows) { $State.Top = $State.Cursor - $rows + 1 }
    $width = $State.Width
    $chosen = $State.Chosen

    $lines = New-Object System.Collections.ArrayList
    [void]$lines.Add(@(@('  Claude Switcher ', 'White'), @('by Nyxon', 'DarkCyan')))
    $status = if ($State.Status) { $State.Status } else { Get-StatusLine }
    [void]$lines.Add(@(, @("  $status", 'DarkGray')))
    [void]$lines.Add(@())
    $heading = "  $($State.Title)"
    if ($State.Multi) { $heading += "   $($chosen.Count) selected" }
    [void]$lines.Add(@(, @($heading, 'Cyan')))
    if ($State.Note) { [void]$lines.Add(@(, @("  $($State.Note)", 'DarkGray'))) } else { [void]$lines.Add(@()) }
    for ($r = $State.Top; $r -lt [Math]::Min($view.Count, $State.Top + $rows); $r++) {
        $item = $items[$view[$r]]
        $here = $r -eq $State.Cursor
        $picked = $chosen.ContainsKey($view[$r])
        $pointer = if ($here) { "  $($G.Ptr) " } else { '    ' }
        $mark = ''
        if ($State.Multi) { $mark = if ($picked) { "$($G.On) " } else { "$($G.Off) " } }
        $labelColor = if ($here) { 'White' } elseif ($item.Color) { $item.Color } else { 'Gray' }
        $room = $width - $pointer.Length - $mark.Length - 2
        $label = [string]$item.Label
        $detail = [string]$item.Detail
        $fit = [Math]::Max(10, $room - $detail.Length - 3)
        if ($detail -and $label.Length -gt $fit) { $label = $label.Substring(0, $fit) }
        $gap = [Math]::Max(2, $room - $label.Length - $detail.Length)
        [void]$lines.Add(@(
            @($pointer, 'Cyan'),
            @($mark, $(if ($picked) { 'Green' } else { 'DarkGray' })),
            @($label, $labelColor),
            @(((' ' * $gap) + $detail), 'DarkGray')
        ))
    }
    if ($view.Count -eq 0) { [void]$lines.Add(@(, @('    nothing matches', 'DarkGray'))) }
    while ($lines.Count -lt $State.Height - 2) { [void]$lines.Add(@()) }
    $keys = if ($State.Typing) { "  search: $($State.Filter)_   Enter keep  Esc clear" }
        elseif ($State.Multi) { '  Space pick  Ctrl+A all  type to search  Enter confirm  Esc back' }
        else { '  Arrows move  type to search  Enter choose  Esc back' }
    if ($State.Filter -and -not $State.Typing) { $keys = "  filter: $($State.Filter)   " + $keys.Trim() }
    if ($keys.Length -gt $width) { $keys = $keys.Substring(0, $width) }
    [void]$lines.Add(@(, @($keys, 'DarkGray')))
    [pscustomobject]@{ Lines = $lines; View = $view; Rows = $rows }
}

# Draws only the rows that changed since the last frame
function Write-Frame($Lines, $Shown, [int]$Origin, [int]$Width, [int]$Height) {
    for ($row = 0; $row -lt $Height; $row++) {
        $segs = if ($row -lt $Lines.Count) { $Lines[$row] } else { @() }
        $drawn = @($segs | ForEach-Object { "$($_[1]):$($_[0])" }) -join '|'
        if ($Shown.ContainsKey($row) -and $Shown[$row] -eq $drawn) { continue }
        $Shown[$row] = $drawn
        [Console]::SetCursorPosition(0, $Origin + $row)
        [Console]::Write(' ' * $Width)
        [Console]::SetCursorPosition(0, $Origin + $row)
        $used = 0
        foreach ($seg in $segs) {
            $text = [string]$seg[0]
            if ($used + $text.Length -gt $Width) { $text = $text.Substring(0, [Math]::Max(0, $Width - $used)) }
            if ($text) { [Console]::ForegroundColor = [ConsoleColor]$seg[1]; [Console]::Write($text) }
            $used += $text.Length
        }
        [Console]::ResetColor()
    }
}

# Applies one key to the picker. Returns @{ Value = ... } when the picker is done.
function Step-Picker($State, $Frame, [ConsoleKeyInfo]$Key) {
    $view = $Frame.View
    $chosen = $State.Chosen
    $char = $Key.KeyChar
    $ctrl = ($Key.Modifiers -band [ConsoleModifiers]::Control) -ne 0
    if ($State.Typing) {
        switch ($Key.Key) {
            'Enter' { $State.Typing = $false; return $null }
            'Escape' { $State.Typing = $false; $State.Filter = ''; return $null }
            'Backspace' {
                if ($State.Filter) { $State.Filter = $State.Filter.Substring(0, $State.Filter.Length - 1) }
                if (-not $State.Filter) { $State.Typing = $false }
                return $null
            }
            'UpArrow' { $State.Typing = $false }
            'DownArrow' { $State.Typing = $false }
            default {
                if (-not $ctrl -and -not [char]::IsControl($char)) { $State.Filter += $char; $State.Cursor = 0; $State.Top = 0 }
                return $null
            }
        }
    }
    switch ($Key.Key) {
        'UpArrow' { if ($State.Cursor -gt 0) { $State.Cursor-- }; return $null }
        'DownArrow' { if ($State.Cursor -lt $view.Count - 1) { $State.Cursor++ }; return $null }
        'PageUp' { $State.Cursor = [Math]::Max(0, $State.Cursor - $Frame.Rows); return $null }
        'PageDown' { $State.Cursor = [Math]::Min([Math]::Max(0, $view.Count - 1), $State.Cursor + $Frame.Rows); return $null }
        'Home' { $State.Cursor = 0; return $null }
        'End' { $State.Cursor = [Math]::Max(0, $view.Count - 1); return $null }
        'Spacebar' {
            if ($State.Multi -and $view.Count) { $i = $view[$State.Cursor]; if ($chosen.ContainsKey($i)) { $chosen.Remove($i) } else { $chosen[$i] = $true } }
            return $null
        }
        'Escape' { if ($State.Filter) { $State.Filter = ''; return $null }; return @{ Value = $null } }
        'Enter' {
            if ($view.Count -eq 0) { return $null }
            if (-not $State.Multi) { return @{ Value = $State.Items[$view[$State.Cursor]] } }
            if ($chosen.Count -eq 0) { return @{ Value = @($State.Items[$view[$State.Cursor]]) } }
            return @{ Value = @($chosen.Keys | Sort-Object | ForEach-Object { $State.Items[$_] }) }
        }
        'A' {
            if ($ctrl -and $State.Multi) {
                $allChosen = @($view | Where-Object { $chosen.ContainsKey($_) }).Count -eq $view.Count
                foreach ($i in $view) { if ($allChosen) { $chosen.Remove($i) } else { $chosen[$i] = $true } }
                return $null
            }
        }
    }
    # any other printable key starts a search, so typing a title never triggers an action
    if (-not $ctrl -and -not [char]::IsControl($char) -and $char -ne ' ') {
        $State.Typing = $true
        if ($char -ne '/') { $State.Filter += $char; $State.Cursor = 0; $State.Top = 0 }
    }
    return $null
}

function Show-Picker {
    param(
        [string]$Title,
        [object[]]$Items,
        [switch]$Multi,
        [string]$Note = ''
    )
    if (-not $Items -or $Items.Count -eq 0) { return $null }
    $state = @{ Title = $Title; Items = $Items; Multi = [bool]$Multi; Note = $Note; Chosen = @{}; Cursor = 0; Top = 0; Filter = ''; Typing = $false; Status = (Get-StatusLine) }
    $shown = @{}
    $size = ''
    $origin = 0
    while ($true) {
        $state.Width = [Math]::Max(40, [Console]::WindowWidth - 1)
        $state.Height = [Math]::Max(12, [Console]::WindowHeight - 1)
        if ("$($state.Width)x$($state.Height)" -ne $size) {
            $size = "$($state.Width)x$($state.Height)"
            $shown = @{}
            Clear-Host
            $origin = [Console]::WindowTop
        }
        $frame = Format-PickerFrame $state
        Write-Frame $frame.Lines $shown $origin $state.Width $state.Height
        # apply every key already waiting before drawing again, so holding an arrow key never queues redraws
        do {
            $done = Step-Picker $state $frame ([Console]::ReadKey($true))
            if ($done) { Clear-Host; return $done.Value }
            $frame = Format-PickerFrame $state
        } while ([Console]::KeyAvailable)
    }
}

function Get-StatusLine {
    $parts = @()
    $parts += if ($Script:Data -like '*\Packages\Claude_*') { 'Store install' } else { 'Standalone install' }
    if ($Script:IsLive) { $parts += if (Test-ClaudeRunning) { 'Claude running' } else { 'Claude closed' } }
    $current = Get-CurrentProfile
    $parts += if (-not $current) { 'no profile saved yet' } elseif (Find-Profile $current) { "profile: $current" } else { "profile: $current (saved at your next switch)" }
    return ($parts -join "  $($G.Dot)  ")
}

function Read-Name([string]$Prompt) {
    Write-Host ''
    Write-Host "  $Prompt" -ForegroundColor White
    Write-Host '  > ' -ForegroundColor Cyan -NoNewline
    try { [Console]::CursorVisible = $true } catch {}
    $value = ([string](Read-Host)).Trim()
    return $value
}

function Wait-Key {
    Write-Host ''
    Write-Host '  Press any key to go back' -ForegroundColor DarkGray
    [void][Console]::ReadKey($true)
}

function Get-MenuItems {
    $current = Get-CurrentProfile
    $last = Get-LastJournal
    @(
        [pscustomobject]@{ Label = 'Switch account'; Detail = $(if ($current) { "now: $current" } else { 'save an account first' }); Value = 'switch' },
        [pscustomobject]@{ Label = 'Add another account'; Detail = 'sign in once, switch forever'; Value = 'add' },
        [pscustomobject]@{ Label = 'Move or copy chats between accounts'; Detail = 'pick chats one by one'; Value = 'chats' },
        [pscustomobject]@{ Label = 'Bring every chat into one account'; Detail = 'copies what is missing'; Value = 'merge' },
        [pscustomobject]@{ Label = 'Recover chats missing from the sidebar'; Detail = 'rebuilds them from history'; Value = 'rescue' },
        [pscustomobject]@{ Label = 'Undo the last chat change'; Detail = $(if ($last) { [string]$last.summary } else { 'nothing to undo' }); Value = 'undo' },
        [pscustomobject]@{ Label = 'Save the signed-in account'; Detail = 'or refresh a saved one'; Value = 'save' },
        [pscustomobject]@{ Label = 'Rename or remove a profile'; Detail = ''; Value = 'profiles' },
        [pscustomobject]@{ Label = 'Check my setup'; Detail = 'doctor'; Value = 'doctor' },
        [pscustomobject]@{ Label = 'Quit'; Detail = ''; Value = 'quit' }
    )
}

function Get-SwitchItems {
    $current = Get-CurrentProfile
    $live = Get-LiveAccount
    @(Get-Profiles | ForEach-Object {
        $detail = if ($_.Name -eq $current) { 'in use' } else { "account $(Short $_.Account)" }
        if ($_.Account -and $_.Account -eq $live -and $_.Name -ne $current) { $detail = 'signed in now' }
        [pscustomobject]@{ Label = $_.Name; Detail = $detail; Value = $_.Name; Color = $(if ($_.Name -eq $current) { 'DarkGray' } else { 'Gray' }) }
    })
}

function Get-SpaceItems([object[]]$Spaces) {
    @($Spaces | ForEach-Object {
        $detail = "$($_.Chats) chats"
        if ($_.SignedIn) { $detail += "  $($G.Dot)  signed in" }
        [pscustomobject]@{ Label = $_.Label; Detail = $detail; Value = $_; Color = $(if ($_.SignedIn) { 'White' } else { 'Gray' }) }
    })
}

function Get-ChatItems([object[]]$Chats) {
    @($Chats | ForEach-Object {
        $flags = @()
        if ($_.Project) { $flags += $_.Project }
        $flags += Format-Ago $_.Last
        if ($_.Archived) { $flags += 'archived' }
        if (-not $_.HasHistory) { $flags += 'no history' }
        [pscustomobject]@{ Label = $_.Title; Detail = ($flags -join "  $($G.Dot)  "); Value = $_; Color = $(if ($_.HasHistory) { 'Gray' } else { 'DarkGray' }) }
    })
}

function Get-OrphanItems([object[]]$Orphans) {
    @($Orphans | ForEach-Object {
        [pscustomobject]@{ Label = $_.Title; Detail = "$($_.Project)  $($G.Dot)  $(Format-Ago $_.Last)"; Value = $_ }
    })
}

function Select-Space([string]$Title, [object[]]$Spaces, [string]$Note = '') {
    $pick = Show-Picker -Title $Title -Items (Get-SpaceItems $Spaces) -Note $Note
    if ($pick) { return $pick.Value }
    return $null
}

function Show-ChatsFlow {
    $spaces = @(Get-Spaces)
    if ($spaces.Count -lt 2) { Warn 'Moving chats needs at least two accounts on this computer.'; return }
    $source = Select-Space 'Move chats from which account?' $spaces
    if (-not $source) { return }
    $chats = @(Get-Chats $source.Dir)
    if (-not $chats) { Warn "$($source.Label) has no chats."; return }
    $picked = @(Show-Picker -Title "Pick chats from $($source.Label)" -Items (Get-ChatItems $chats) -Multi -Note 'Space picks a chat, / searches titles and projects')
    if (-not $picked -or -not $picked[0]) { return }
    $target = Select-Space "Put $($picked.Count) chat(s) into which account?" @($spaces | Where-Object { $_.Dir -ne $source.Dir })
    if (-not $target) { return }
    $how = Show-Picker -Title "Copy or move $($picked.Count) chat(s) to $($target.Label)?" -Items @(
        [pscustomobject]@{ Label = 'Copy'; Detail = 'the chats show up in both accounts'; Value = $false },
        [pscustomobject]@{ Label = 'Move'; Detail = "they leave $($source.Label)"; Value = $true }
    )
    if (-not $how) { return }
    Invoke-Transfer $source $target @($picked | ForEach-Object { $_.Value }) ([bool]$how.Value)
}

function Show-RescueFlow {
    Say 'Looking for chats that no sidebar lists...' 'DarkGray'
    $orphans = @(Get-Orphans)
    if (-not $orphans) { Ok 'Every chat history on this computer is in a sidebar.'; return }
    $note = 'Some may be chats you deleted on purpose. To bring a chat from another account, use Move or copy chats.'
    if ($Script:HiddenParts) { $note = "$($Script:HiddenParts) older part(s) of existing chats are hidden. " + $note }
    $picked = @(Show-Picker -Title "$($orphans.Count) chat(s) are on disk but in no sidebar" -Items (Get-OrphanItems $orphans) -Multi -Note $note)
    if (-not $picked -or -not $picked[0]) { return }
    $spaces = @(Get-Spaces)
    if (-not $spaces) { Fail 'Sign in to Claude Desktop once so it creates a chat list, then try again.'; return }
    $target = if ($spaces.Count -eq 1) { $spaces[0] } else { Select-Space "Recover $($picked.Count) chat(s) into which account?" $spaces }
    if (-not $target) { return }
    Invoke-Rescue $target @($picked | ForEach-Object { $_.Value })
}

function Show-AddFlow {
    $current = Get-CurrentProfile
    if (-not $current -or -not (Find-Profile $current)) {
        if (-not (Get-LiveAccount)) { Fail 'Sign in to Claude Desktop first, then come back.'; return }
        Say 'First, a name for the account you are signed into now.' 'White'
        $first = Read-Name 'Name for the current account (for example: work)'
        if (-not $first) { return }
        Invoke-Save $first -StayClosed
        if (-not (Find-Profile $first)) { return }
    }
    $name = Read-Name 'Name for the new account (for example: personal)'
    if (-not $name) { return }
    Invoke-New $name
}

function Show-ProfilesFlow {
    $profiles = @(Get-Profiles)
    if (-not $profiles) { Info 'No saved profiles yet.'; return }
    $current = Get-CurrentProfile
    $pick = Show-Picker -Title 'Which profile?' -Items @($profiles | ForEach-Object {
        [pscustomobject]@{ Label = $_.Name; Detail = $(if ($_.Name -eq $current) { 'in use' } else { "saved $($_.Saved.ToString('yyyy-MM-dd'))" }); Value = $_ }
    })
    if (-not $pick) { return }
    $action = Show-Picker -Title "What should happen to '$($pick.Value.Name)'?" -Items @(
        [pscustomobject]@{ Label = 'Rename'; Detail = ''; Value = 'rename' },
        [pscustomobject]@{ Label = 'Remove the saved login'; Detail = 'chats stay in Claude'; Value = 'remove' }
    )
    if (-not $action) { return }
    if ($action.Value -eq 'rename') {
        $new = Read-Name "New name for '$($pick.Value.Name)'"
        if ($new) { Invoke-Rename $pick.Value.Name $new }
    }
    else { Invoke-Remove $pick.Value.Name }
}

function Start-Menu {
    try { [Console]::CursorVisible = $false } catch {}
    try {
        while ($true) {
            $current = Get-CurrentProfile
            $pick = Show-Picker -Title 'What would you like to do?' -Items (Get-MenuItems)
            if (-not $pick -or $pick.Value -eq 'quit') { break }
            try { [Console]::CursorVisible = $true } catch {}
            Write-Host ''
            switch ($pick.Value) {
                'switch' {
                    $profiles = @(Get-Profiles)
                    if (-not $profiles) { Info 'No saved profiles yet. Start with "Add another account".'; break }
                    $target = Show-Picker -Title 'Switch to which account?' -Items (Get-SwitchItems)
                    if ($target) { Write-Host ''; Invoke-Switch $target.Value }
                }
                'add' { Show-AddFlow }
                'chats' { Show-ChatsFlow }
                'merge' {
                    $target = Select-Space 'Bring every chat into which account?' @(Get-Spaces) 'Chats it already has are left alone'
                    if ($target) { Invoke-Merge $target @(Get-Spaces) }
                }
                'rescue' { Show-RescueFlow }
                'undo' { Invoke-Undo }
                'save' {
                    $name = Read-Name 'Save the signed-in account as (for example: work)'
                    if ($name) { Invoke-Save $name }
                }
                'profiles' { Show-ProfilesFlow }
                'doctor' { Invoke-Doctor }
            }
            Wait-Key
            try { [Console]::CursorVisible = $false } catch {}
        }
    }
    finally {
        try { [Console]::CursorVisible = $true } catch {}
        Clear-Host
    }
}

# ------------------------------------------------------------------ commands

function Show-Help {
    Write-Host ''
    Write-Host '  Claude Switcher ' -ForegroundColor White -NoNewline
    Write-Host "by Nyxon $($Script:Version)" -ForegroundColor DarkCyan
    Write-Host '  Switch Claude Desktop accounts without signing in again, and move your chats between them.' -ForegroundColor DarkGray
    Write-Host ''
    Write-Host '  Run with no arguments for the interactive menu, or:' -ForegroundColor Gray
    Write-Host ''
    @(
        @('switch <name>', 'switch to a saved account'),
        @('save <name>', 'save the account Desktop is signed into'),
        @('new <name>', 'open Desktop signed out to add another account'),
        @('list', 'saved accounts'),
        @('rename <old> <new>', 'rename a saved account'),
        @('remove <name>', 'forget a saved login'),
        @('accounts', 'chat lists on this computer, with counts'),
        @('chats <account>', 'chats in one account'),
        @('copy -From <a> -To <b> -Chat <id>[,<id>]', 'copy chats (or -All)'),
        @('move -From <a> -To <b> -Chat <id>[,<id>]', 'move chats (or -All)'),
        @('merge -To <account>', 'copy every missing chat into one account'),
        @('rescue [-To <account>] [-All | -Chat <id>]', 'recover chats missing from every sidebar'),
        @('undo', 'undo the last chat change'),
        @('doctor', 'check the setup'),
        @('repair', 'rebuild the Cowork VM disk if it is missing')
    ) | ForEach-Object { Write-Host ('    {0,-44}' -f $_[0]) -ForegroundColor Cyan -NoNewline; Write-Host $_[1] -ForegroundColor Gray }
    Write-Host ''
    Write-Host '  Accounts are profile names, signed-in, or an account id prefix. Chat ids come from `chats`.' -ForegroundColor DarkGray
    Write-Host '  -Yes skips confirmations, -NoRestart leaves Claude Desktop closed afterwards.' -ForegroundColor DarkGray
    Write-Host "  $($Script:RepoUrl)" -ForegroundColor DarkGray
    Write-Host ''
}

function Show-List {
    $profiles = @(Get-Profiles)
    $current = Get-CurrentProfile
    Info "Claude data: $($Script:Data)"
    if ($current -and -not (Find-Profile $current)) {
        Write-Host "  $($G.On) $current" -ForegroundColor Green -NoNewline; Write-Host '  in use, saved at your next switch' -ForegroundColor DarkGray
    }
    if (-not $profiles) { if (-not $current) { Info 'No saved accounts yet. Run: claude-switcher save <name>' }; return }
    foreach ($p in $profiles) {
        if ($p.Name -eq $current) { Write-Host "  $($G.On) $($p.Name)" -ForegroundColor Green -NoNewline; Write-Host '  in use' -ForegroundColor DarkGray }
        else { Write-Host "  $($G.Off) $($p.Name)" -ForegroundColor Gray }
    }
}

function Show-Accounts {
    $spaces = @(Get-Spaces)
    if (-not $spaces) { Info 'No chat lists yet. Open Claude Desktop and use the Code tab once.'; return }
    foreach ($s in $spaces) {
        $tail = if ($s.SignedIn) { '  signed in' } else { '' }
        Write-Host ('  {0,-28}' -f $s.Label) -ForegroundColor White -NoNewline
        Write-Host ('{0,5} chats  {1}/{2}{3}' -f $s.Chats, (Short $s.Account), (Short $s.Org), $tail) -ForegroundColor DarkGray
    }
}

function Show-Chats([string]$Ref) {
    $space = Resolve-Space $(if ($Ref) { $Ref } else { 'signed-in' }) @(Get-Spaces)
    if (-not $space) { return }
    Say "$($space.Label): $($space.Chats) chat(s)" 'White'
    foreach ($c in Get-Chats $space.Dir) {
        $id = $c.Id -replace '^local_', ''
        Write-Host ('  {0}  ' -f (Short $id)) -ForegroundColor Cyan -NoNewline
        Write-Host $c.Title -ForegroundColor Gray -NoNewline
        Write-Host ("  $($G.Dot) $($c.Project) $($G.Dot) $(Format-Ago $c.Last)$(if (-not $c.HasHistory) { ' ' + $G.Dot + ' no history' })") -ForegroundColor DarkGray
    }
}

function Invoke-TransferCommand([bool]$MoveThem) {
    $spaces = @(Get-Spaces)
    $source = Resolve-Space $From $spaces
    $target = Resolve-Space $To $spaces
    if (-not $source -or -not $target) { if (-not $From -or -not $To) { Fail 'Pass both -From and -To.' }; return }
    if (-not $All -and -not $Chat) { Fail 'Pass -Chat <id>[,<id>] or -All. List ids with: claude-switcher chats <account>'; return }
    $chats = @(Get-Chats $source.Dir)
    $picked = if ($All) { $chats } else { Resolve-Chats $Chat $chats }
    if ($null -eq $picked) { return }
    Invoke-Transfer $source $target @($picked) $MoveThem
}

function Invoke-RescueCommand {
    $orphans = @(Get-Orphans)
    if (-not $orphans) { Ok 'Every chat history on this computer is in a sidebar.'; return }
    if (-not $All -and -not $Chat) {
        Say "$($orphans.Count) chat(s) are on disk but in no sidebar:" 'White'
        foreach ($o in $orphans) {
            Write-Host ('  {0}  ' -f (Short $o.Session)) -ForegroundColor Cyan -NoNewline
            Write-Host $o.Title -ForegroundColor Gray -NoNewline
            Write-Host "  $($G.Dot) $($o.Project) $($G.Dot) $(Format-Ago $o.Last)" -ForegroundColor DarkGray
        }
        Info 'Recover with: claude-switcher rescue -To <account> -Chat <id>[,<id>]  (or -All)'
        if ($Script:HiddenParts) { Info "$($Script:HiddenParts) older part(s) of chats that already exist are not listed." }
        return
    }
    $picked = if ($All) { $orphans } else {
        @(foreach ($ref in @($Chat | ForEach-Object { $_ -split ',' } | Where-Object { $_ })) {
            $hits = @($orphans | Where-Object { $_.Session.StartsWith($ref, 'OrdinalIgnoreCase') })
            if ($hits.Count -ne 1) { Fail "'$ref' matches $($hits.Count) chats, use a longer id."; return }
            $hits[0]
        })
    }
    $target = Resolve-Space $(if ($To) { $To } else { 'signed-in' }) @(Get-Spaces)
    if ($target) { Invoke-Rescue $target $picked }
}

function Invoke-Main {
    switch ($Command.ToLower()) {
        '' { if ($Script:Interactive) { Start-Menu } else { Show-Help } }
        { $_ -in 'help', '-h', '--help', '/?' } { Show-Help }
        { $_ -in 'version', '--version', '-v' } { Write-Output $Script:Version }
        { $_ -in 'list', 'ls' } { Show-List }
        'current' { $c = Get-CurrentProfile; if ($c) { Say $c } else { Info 'No profile in use yet.' } }
        'switch' { if ($Name) { Invoke-Switch $Name } else { Fail 'Usage: claude-switcher switch <name>' } }
        { $_ -in 'save', 'create' } { if ($Name) { Invoke-Save $Name } else { Fail 'Usage: claude-switcher save <name>' } }
        { $_ -in 'new', 'add' } { if ($Name) { Invoke-New $Name } else { Fail 'Usage: claude-switcher new <name>' } }
        'rename' { if ($Name -and $NewName) { Invoke-Rename $Name $NewName } else { Fail 'Usage: claude-switcher rename <old> <new>' } }
        { $_ -in 'remove', 'rm' } { if ($Name) { Invoke-Remove $Name } else { Fail 'Usage: claude-switcher remove <name>' } }
        'accounts' { Show-Accounts }
        'chats' { Show-Chats $Name }
        'copy' { Invoke-TransferCommand $false }
        'move' { Invoke-TransferCommand $true }
        'merge' { $target = Resolve-Space $To @(Get-Spaces); if ($target) { Invoke-Merge $target @(Get-Spaces) } elseif (-not $To) { Fail 'Usage: claude-switcher merge -To <account>' } }
        'rescue' { Invoke-RescueCommand }
        'undo' { Invoke-Undo }
        'doctor' { Invoke-Doctor }
        'repair' { Repair-CoworkVM; Ok 'Cowork VM check complete' }
        default { Fail "Unknown command '$Command'."; Show-Help }
    }
}

Invoke-Main
