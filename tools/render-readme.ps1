# Renders the README images from the switcher's own menu code, fed with demo data.
# Needs PowerShell 7. Run from the repository root:  pwsh tools/render-readme.ps1
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$out = Join-Path $repo 'assets/readme'
New-Item -ItemType Directory -Path $out -Force | Out-Null

# ---------------------------------------------------------------- demo data, never real accounts
$demo = Join-Path ([IO.Path]::GetTempPath()) "claude-switcher-demo-$PID"
if (Test-Path $demo) { Remove-Item $demo -Recurse -Force }
$vault = "$demo/vault"
$utf8 = New-Object System.Text.UTF8Encoding $false
function Put([string]$Path, [string]$Text) { New-Item -ItemType Directory -Path (Split-Path $Path) -Force | Out-Null; [IO.File]::WriteAllText($Path, $Text, $utf8) }
$accounts = @{ work = '5f3a9c21-0000-4000-8000-000000000001'; personal = '8b21d0e4-0000-4000-8000-000000000002'; freelance = 'c47e9a13-0000-4000-8000-000000000003' }
foreach ($name in $accounts.Keys) { Put "$vault/$name/config.json" '{}'; Put "$vault/$name/_account" $accounts[$name] }
Put "$vault/_current_profile" 'work'
Put "$vault/_journal/20260924-101500-000/journal.json" '{"action":"copy","summary":"Copied 3 chat(s) from work to personal","entries":[]}'

$source = [IO.File]::ReadAllText("$repo/claude-switcher.ps1") -replace '(?m)^Invoke-Main\s*$', ''
. ([scriptblock]::Create($source)) -Command 'noop' -ClaudeDir "$demo/Claude" -InstanceDir $vault -ProjectsDir "$demo/projects"

$now = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$min = 60000; $hour = 60 * $min; $day = 24 * $hour
function Chat([string]$Title, [string]$Project, [int64]$Ago, [switch]$Archived, [switch]$Lost) {
    [pscustomobject]@{ Title = $Title; Project = $Project; Last = $now - $Ago; Archived = [bool]$Archived; HasHistory = -not $Lost }
}
$chats = @(
    (Chat 'Refactor the auth middleware' 'api-gateway' (12 * $min)),
    (Chat 'Fix the flaky payment webhook test' 'billing' (2 * $hour)),
    (Chat 'Migrate the dashboard to Tailwind 4' 'web-app' (1 * $day)),
    (Chat 'رفع باگ ترجمه در صفحه ورود' 'web-app' (2 * $day)),
    (Chat 'Write the Q3 release notes' 'docs' (3 * $day)),
    (Chat 'Profile the slow search query' 'api-gateway' (5 * $day) -Archived),
    (Chat 'Set up the Windows CI runner' 'infra' (8 * $day)),
    (Chat 'Draft the onboarding tour copy' 'web-app' (11 * $day)),
    (Chat 'Upgrade Nx and Rsbuild together' 'monorepo' (16 * $day)),
    (Chat 'Chase the memory leak in the queue worker' 'queue-worker' (23 * $day) -Lost)
)
$orphans = @(
    [pscustomobject]@{ Title = 'Debug the Docker Compose network'; Project = 'infra'; Last = $now - 3 * $day },
    [pscustomobject]@{ Title = 'Plan the Postgres 17 upgrade'; Project = 'api-gateway'; Last = $now - 9 * $day },
    [pscustomobject]@{ Title = 'Customer export CSV script'; Project = 'billing'; Last = $now - 14 * $day },
    [pscustomobject]@{ Title = 'Explain the Kubernetes ingress setup'; Project = 'infra'; Last = $now - 31 * $day }
)
$status = "Store install  $($G.Dot)  Claude closed  $($G.Dot)  profile: work"

function Frame([string]$Title, [object[]]$Items, [int]$Width, [int]$Height, [int]$Cursor = 0, [int[]]$Chosen = @(), [switch]$Multi, [string]$Note = '') {
    $picked = @{}; foreach ($i in $Chosen) { $picked[$i] = $true }
    $state = @{ Title = $Title; Items = $Items; Multi = [bool]$Multi; Note = $Note; Chosen = $picked; Cursor = $Cursor; Top = 0; Filter = ''; Typing = $false; Status = $status; Width = $Width; Height = $Height }
    (Format-PickerFrame $state).Lines
}

# ---------------------------------------------------------------- Nyxon palette
$C = @{ Void = '#090b14'; Window = '#0e1120'; Bar = '#141830'; Line = '#303345'; Cream = '#f3f0e8'; Muted = '#aaaec4'; Star = '#a8b2ff'; Dim = '#6e7391' }
$Console = @{ White = $C.Cream; Gray = '#c5c8d8'; DarkGray = $C.Dim; Cyan = $C.Star; DarkCyan = '#8a93e8'; Green = '#86e0a8'; Yellow = '#f2d68b'; Red = '#ff8f8f' }
$Mono = "'Cascadia Mono', 'Cascadia Code', Consolas, ui-monospace, 'SF Mono', Menlo, monospace"
$Sans = "'Segoe UI', -apple-system, BlinkMacSystemFont, 'Helvetica Neue', Arial, sans-serif"
function Esc([string]$Text) { $Text.Replace('&', '&amp;').Replace('<', '&lt;').Replace('>', '&gt;') }

# A terminal window whose text sits on a fixed column grid, so glyph fallbacks never drift the layout
function Terminal($Lines, [int]$Cols, [double]$X, [double]$Y, [double]$Size, [string]$Tab = 'claude-switcher') {
    $cw = $Size * 0.6; $lh = [Math]::Round($Size * 1.45, 1); $pad = 22; $bar = 40
    $w = [Math]::Round($Cols * $cw + 2 * $pad, 1); $h = [Math]::Round($bar + $pad + $Lines.Count * $lh + $pad * 0.6, 1)
    $sb = [Text.StringBuilder]::new()
    [void]$sb.Append("<g transform=`"translate($X $Y)`">")
    [void]$sb.Append("<rect width=`"$w`" height=`"$h`" rx=`"14`" fill=`"$($C.Window)`" stroke=`"$($C.Line)`" stroke-width=`"1.5`"/>")
    [void]$sb.Append("<path d=`"M0 14a14 14 0 0 1 14-14h$($w - 28)a14 14 0 0 1 14 14v$($bar - 14)h-$w z`" fill=`"$($C.Bar)`"/>")
    [void]$sb.Append("<rect x=`"14`" y=`"8`" width=`"196`" height=`"$($bar - 8)`" rx=`"8`" fill=`"$($C.Window)`"/>")
    [void]$sb.Append("<text x=`"30`" y=`"$($bar - 11)`" font-family=`"$Mono`" font-size=`"15`" fill=`"$($C.Star)`">&gt;_</text>")
    [void]$sb.Append("<text x=`"58`" y=`"$($bar - 11)`" font-family=`"$Sans`" font-size=`"15`" fill=`"$($C.Muted)`">$(Esc $Tab)</text>")
    $cx = $w - 34
    [void]$sb.Append("<g stroke=`"$($C.Dim)`" stroke-width=`"1.6`" stroke-linecap=`"round`" fill=`"none`"><path d=`"M$($cx - 62) 20h12`"/><rect x=`"$($cx - 34)`" y=`"14`" width=`"12`" height=`"12`" rx=`"1.5`"/><path d=`"M$($cx - 4) 14l12 12M$($cx + 8) 14l-12 12`"/></g>")
    $row = 0
    foreach ($line in $Lines) {
        $ty = [Math]::Round($bar + $pad + $row * $lh + $Size * 0.8, 1)
        $col = 0
        foreach ($seg in $line) {
            $text = [string]$seg[0]
            if ($col + $text.Length -gt $Cols) { $text = $text.Substring(0, [Math]::Max(0, $Cols - $col)) }
            $fill = $Console[[string]$seg[1]]
            # every run of one script sits at its own column; a right-to-left title stays one run so its words keep their order
            foreach ($m in [regex]::Matches($text, '[^\x00-\x7F]+(?: +[^\x00-\x7F]+)*|[\x21-\x7E]+(?: +[\x21-\x7E]+)*')) {
                if ($m.Value.Trim()) {
                    $tx = [Math]::Round($pad + ($col + $m.Index) * $cw, 1)
                    [void]$sb.Append("<text x=`"$tx`" y=`"$ty`" xml:space=`"preserve`" font-family=`"$Mono`" font-size=`"$Size`" fill=`"$fill`">$(Esc $m.Value)</text>")
                }
            }
            $col += $text.Length
        }
        $row++
    }
    [void]$sb.Append('</g>')
    [pscustomobject]@{ Svg = $sb.ToString(); Width = $w; Height = $h }
}

function Save-Svg([string]$Name, [int]$W, [int]$H, [string]$Title, [string]$Desc, [string]$Body, [switch]$Bare) {
    $bg = if ($Bare) { '' } else { "<rect width=`"$W`" height=`"$H`" rx=`"24`" fill=`"$($C.Void)`"/>" }
    $svg = "<svg xmlns=`"http://www.w3.org/2000/svg`" width=`"$W`" height=`"$H`" viewBox=`"0 0 $W $H`" role=`"img`" aria-labelledby=`"title desc`"><title id=`"title`">$(Esc $Title)</title><desc id=`"desc`">$(Esc $Desc)</desc>$bg$Body</svg>`n"
    [IO.File]::WriteAllText("$out/$Name", $svg, $utf8)
    Write-Host "  wrote assets/readme/$Name ($([int]($svg.Length / 1024)) KB)"
}

# ---------------------------------------------------------------- hero: name, promise, real command output
$heroCmd = @(
    @(@('> ', 'DarkGray'), @('claude-switcher switch personal', 'White')),
    @(, @('  Switching work -> personal', 'Cyan')),
    @(, @("  $($G.On) Claude Desktop is closed", 'Green')),
    @(, @("  $($G.On) Saved 'work'", 'Green')),
    @(, @("  $($G.On) Signed in as 'personal'", 'Green')),
    @(, @("  $($G.On) Reopening Claude Desktop", 'Green'))
)
$heroTerm = Terminal $heroCmd 36 660 112 20 'Windows PowerShell'
$logo = [IO.File]::ReadAllText("$repo/assets/readme/source/nyxon-logo.svg")
$logoInner = [regex]::Match($logo, '(?s)<g id="nyxon"[^>]*>(.*)</g>\s*</svg>').Groups[1].Value
$hero = @"
<g font-family="$Sans">
  <text x="64" y="104" font-family="$Mono" font-size="18" letter-spacing="2.5" fill="$($C.Star)">FOR CLAUDE DESKTOP ON WINDOWS</text>
  <text x="60" y="186" font-size="76" font-weight="700" fill="$($C.Cream)" letter-spacing="-1.5">Claude Switcher</text>
  <text x="64" y="240" font-size="27" fill="$($C.Cream)">Switch accounts without signing in again.</text>
  <text x="64" y="278" font-size="27" fill="$($C.Muted)">Take your Code chats with you.</text>
  <g font-family="$Mono" font-size="19">
    <text x="64" y="340" fill="$($C.Star)">$($G.Ptr)</text><text x="82" y="340" fill="$($C.Muted)">switch</text>
    <text x="178" y="340" fill="$($C.Star)">$($G.Ptr)</text><text x="196" y="340" fill="$($C.Muted)">move chats</text>
    <text x="336" y="340" fill="$($C.Star)">$($G.Ptr)</text><text x="354" y="340" fill="$($C.Muted)">recover lost chats</text>
  </g>
  <g transform="translate(64 372) scale(0.088)" fill="$($C.Muted)" fill-rule="evenodd">$logoInner</g>
</g>
$($heroTerm.Svg)
"@
Save-Svg 'hero.svg' 1200 440 'Claude Switcher by Nyxon' 'Switch Claude Desktop accounts on Windows without signing in again, and take your Claude Code chats with you. Shown: the real output of claude-switcher switch personal.' $hero

# ---------------------------------------------------------------- full-width frames from the menu code
function Board([string]$Name, [string]$Title, [string]$Desc, $Lines, [int]$Cols, [double]$Size) {
    $t = Terminal $Lines $Cols 0 0 $Size
    $W = 1200; $H = [int]($t.Height + 48)
    $x = [Math]::Round(($W - $t.Width) / 2, 1)
    Save-Svg $Name $W $H $Title $Desc ($t.Svg -replace '^<g transform="translate\(0 0\)">', "<g transform=`"translate($x 24)`">")
}

$menu = Frame 'What would you like to do?' (Get-MenuItems) 80 18 -Cursor 2
Board 'menu.svg' 'The Claude Switcher menu' 'The interactive menu: switch account, add an account, move or copy chats, bring every chat into one account, recover lost chats, undo, and a setup check.' $menu 80 19

$chatFrame = Frame 'Pick chats from work' (Get-ChatItems $chats) 80 18 -Cursor 3 -Chosen @(0, 1, 3) -Multi -Note 'Space picks a chat, / searches titles and projects'
Board 'chats.svg' 'Picking chats to move' 'The chat picker lists every Claude Code chat in an account with its project and age. Three chats are selected to move to another account.' $chatFrame 80 19

$rescueFrame = Frame "$($orphans.Count) chat(s) are on disk but in no sidebar" (Get-OrphanItems $orphans) 80 12 -Cursor 1 -Chosen @(0, 1) -Multi -Note 'Some may be chats you deleted on purpose, pick the ones you want back'
Board 'rescue.svg' 'Recovering chats the sidebar lost' 'Claude Switcher finds chat histories on disk that no account lists and rebuilds their sidebar entries.' $rescueFrame 80 19

# ---------------------------------------------------------------- how it works: the three things Desktop keeps
function Lane([int]$Y, [string]$Kicker, [string]$Path, [string]$Action, [string]$Why) {
    @"
  <rect x="48" y="$Y" width="1104" height="96" rx="14" fill="$($C.Window)" stroke="$($C.Line)" stroke-width="1.5"/>
  <text x="80" y="$($Y + 36)" font-family="$Mono" font-size="16" letter-spacing="2" fill="$($C.Star)">$Kicker</text>
  <text x="80" y="$($Y + 70)" font-family="$Mono" font-size="20" fill="$($C.Cream)">$(Esc $Path)</text>
  <text x="640" y="$($Y + 58)" font-family="$Mono" font-size="26" fill="$($C.Star)">$($G.Ptr)</text>
  <text x="680" y="$($Y + 42)" font-size="22" fill="$($C.Cream)">$(Esc $Action)</text>
  <text x="680" y="$($Y + 72)" font-size="19" fill="$($C.Muted)">$(Esc $Why)</text>
"@
}
$how = @"
<g font-family="$Sans">
$(Lane 40 'LOGIN' 'config.json, Network, Local Storage' 'Switch swaps these login files' 'about 5 MB per profile, kept in .claude-instances')
$(Lane 152 'SIDEBAR' 'claude-code-sessions\account\*.json' 'Move, copy or merge chats' 'Desktop keeps one chat list per account')
$(Lane 264 'HISTORY' '.claude\projects\...\session.jsonl' 'Recover chats from the full history' 'shared by every account, never modified')
  <text x="48" y="404" font-family="$Mono" font-size="17" fill="$($C.Dim)">never touched:  Cowork VM  $($G.Dot)  MCP servers  $($G.Dot)  encryption key  $($G.Dot)  Claude Code settings</text>
</g>
"@
Save-Svg 'how-it-works.svg' 1200 440 'How Claude Switcher works' 'Claude Desktop keeps login files, one chat list per account, and a shared chat history. Switching swaps the login files, chat moves edit the per-account lists, and recovery rebuilds list entries from the shared history, which is never modified.' $how

# ---------------------------------------------------------------- footer signature
$made = @"
<text x="28" y="40" font-family="$Mono" font-size="15" letter-spacing="2" fill="$($C.Dim)">MADE BY</text>
<g transform="translate(120 21) scale(0.094)" fill="$($C.Cream)" fill-rule="evenodd">$logoInner</g>
"@
Save-Svg 'made-by-nyxon.svg' 250 64 'Made by Nyxon' 'Claude Switcher is made by Nyxon.' $made
Remove-Item $demo -Recurse -Force
