# Runs claude-switcher.ps1 against a throwaway fixture: no real Claude data is read or written.
# Usage: pwsh -File tests/run.ps1   (also works in Windows PowerShell 5.1)
$ErrorActionPreference = 'Stop'
$script = Join-Path (Split-Path $PSScriptRoot -Parent) 'claude-switcher.ps1'
$root = Join-Path ([IO.Path]::GetTempPath()) "claude-switcher-test-$PID"
$live = "$root\Claude"; $vault = "$root\vault"; $projects = "$root\projects"
$utf8 = New-Object System.Text.UTF8Encoding $false
$script:failures = 0; $script:passes = 0

$WORK = '11111111-1111-4111-8111-111111111111'
$PERS = '22222222-2222-4222-8222-222222222222'
$ORG = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa'
$persianTitle = -join ([char[]](0x628, 0x627, 0x6AF, 0x20, 0x62A, 0x631, 0x62C, 0x645, 0x647))

function Check([string]$Label, [bool]$Ok) {
    if ($Ok) { $script:passes++; Write-Host "  pass  $Label" -ForegroundColor Green }
    else { $script:failures++; Write-Host "  FAIL  $Label" -ForegroundColor Red }
}
function Put([string]$Path, [string]$Text) {
    New-Item -ItemType Directory -Path (Split-Path $Path) -Force | Out-Null
    [IO.File]::WriteAllText($Path, $Text, $utf8)
}
function Run {
    $out = & $script @args -ClaudeDir $live -InstanceDir $vault -ProjectsDir $projects -NoRestart -Yes *>&1
    return ($out | ForEach-Object { "$_" }) -join "`n"
}
function Hash([string]$Dir) {
    if (-not (Test-Path $Dir)) { return '-' }
    (Get-ChildItem $Dir -Recurse -File | Sort-Object FullName | ForEach-Object { $_.FullName.Substring($Dir.Length) + '=' + (Get-FileHash $_.FullName).Hash }) -join ';'
}
$names = 'config.json', 'Preferences', 'DIPS', 'DIPS-wal', 'SharedStorage', 'SharedStorage-wal', 'ant-did', 'buddy-tokens.json', 'plan-usage-history.json', 'Local Storage', 'Session Storage', 'Network', 'IndexedDB', 'WebStorage'
function Login {
    ($names | ForEach-Object {
        $p = "$live\$_"
        if (Test-Path $p -PathType Leaf) { "$_=" + (Get-FileHash $p).Hash } elseif (Test-Path $p) { "$_/" + (Hash $p) } else { "$_=-" }
    }) -join ';'
}
function Card([string]$Account, [string]$Id, [string]$Session, [string]$Title, [int64]$Last) {
    Put "$live\claude-code-sessions\$Account\$ORG\local_$Id.json" (@{ sessionId = "local_$Id"; cliSessionId = $Session; cwd = 'C:\work\proj'; title = $Title; lastActivityAt = $Last } | ConvertTo-Json -Compress)
}
function Transcript([string]$Session, [string[]]$Lines) { Put "$projects\C--work-proj\$Session.jsonl" ($Lines -join "`n") }

if (Test-Path $root) { Remove-Item $root -Recurse -Force }

Write-Host "`nprofiles" -ForegroundColor Cyan
Put "$live\config.json" "{`"lastKnownAccountUuid`":`"$WORK`",`"userThemeMode`":`"dark`"}"
foreach ($f in 'Preferences', 'DIPS', 'DIPS-wal', 'SharedStorage', 'SharedStorage-wal', 'ant-did', 'buddy-tokens.json', 'plan-usage-history.json') { Put "$live\$f" "work-$f" }
foreach ($d in 'Local Storage\leveldb\000003.log', 'Session Storage\000003.log', 'Network\Cookies', 'IndexedDB\https_claude.ai_0.indexeddb.leveldb\LOG', 'WebStorage\QuotaManager') { Put "$live\$d" "work-$d" }
Put "$live\Local State" 'shared-dpapi-key'
Put "$live\claude_desktop_config.json" '{"mcpServers":{}}'
$shared = { "{0}|{1}" -f (Get-FileHash "$live\Local State").Hash, (Get-FileHash "$live\claude_desktop_config.json").Hash }
$sharedBefore = & $shared
$workLogin = Login

$out = Run save work
Check 'save records the account id' ([IO.File]::ReadAllText("$vault\work\_account") -eq $WORK)
Check 'save marks the profile in use' ([IO.File]::ReadAllText("$vault\_current_profile") -eq 'work')
$out = Run new personal
Check 'new clears the login so Desktop opens signed out' (-not (Test-Path "$live\config.json") -and -not (Test-Path "$live\Network"))
Check 'new leaves shared files alone' ((& $shared) -eq $sharedBefore)
$out = Run list
Check 'list shows a profile that is in use but not saved yet' ($out -match 'personal' -and $out -match 'saved at your next switch')

Put "$live\config.json" "{`"lastKnownAccountUuid`":`"$PERS`"}"
foreach ($f in 'Preferences', 'DIPS', 'SharedStorage', 'ant-did', 'buddy-tokens.json') { Put "$live\$f" "personal-$f" }
Put "$live\Network\Cookies" 'personal-cookie'
$personalLogin = Login
$out = Run switch work
Check 'switch saves the account it leaves' ([IO.File]::ReadAllText("$vault\personal\_account") -eq $PERS)
Check 'switch restores the work login byte for byte' ((Login) -eq $workLogin)
$out = Run switch personal
Check 'switch back restores personal byte for byte' ((Login) -eq $personalLogin)
Check 'no stale DIPS-wal survives from the other account' (-not (Test-Path "$live\DIPS-wal"))

Put "$live\config.json" '{"lastKnownAccountUuid":"33333333-3333-4333-8333-333333333333"}'
$tampered = Login; $saved = Hash "$vault\personal"
$out = Run switch work
Check 'switch refuses when Desktop was signed into another account by hand' ((Login) -eq $tampered -and (Hash "$vault\personal") -eq $saved -and $out -match 'different account')
Put "$live\config.json" "{`"lastKnownAccountUuid`":`"$PERS`"}"

$out = Run save '_hidden'
Check 'names starting with an underscore are refused' ($out -match 'profile name' -and -not (Test-Path "$vault\_hidden\config.json"))
$out = Run save 'trailing.'
Check 'names ending with a dot are refused' (-not (Test-Path "$vault\trailing.\config.json"))
$persianName = -join ([char[]](0x6A9, 0x627, 0x631))
$out = Run rename work $persianName
Check 'profiles can be renamed, Persian names included' ((Test-Path "$vault\$persianName\config.json") -and -not (Test-Path "$vault\work"))
$out = Run rename $persianName work
$out = Run remove personal
Check 'the profile in use cannot be removed' (Test-Path "$vault\personal\config.json")
Check 'shared files untouched after every profile action' ((& $shared) -eq $sharedBefore)

Write-Host "`nchats" -ForegroundColor Cyan
$now = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$s1 = '10000000-0000-4000-8000-000000000001'; $s2 = '10000000-0000-4000-8000-000000000002'; $s3 = '10000000-0000-4000-8000-000000000003'
Card $WORK 'c1' $s1 'Design review' ($now - 3600000)
Card $WORK 'c2' $s2 'Release notes' ($now - 7200000)
Card $PERS 'c3' $s3 'Holiday plan' ($now - 60000)
foreach ($s in $s1, $s2, $s3) { Transcript $s @("{`"type`":`"user`",`"timestamp`":`"2026-09-20T10:00:00.000Z`",`"cwd`":`"C:\\\\work\\\\proj`"}") }
$workDir = "$live\claude-code-sessions\$WORK\$ORG"; $persDir = "$live\claude-code-sessions\$PERS\$ORG"
$workBefore = Hash $workDir

$out = Run accounts
Check 'accounts lists both chat lists with counts' ($out -match 'work' -and $out -match '2 chats' -and $out -match '1 chats')
$out = Run chats work
Check 'chats lists titles newest first' ($out.IndexOf('Design review') -lt $out.IndexOf('Release notes') -and $out.IndexOf('Design review') -ge 0)
$out = Run copy -From work -To personal -Chat c1
Check 'copy puts the chat in the target' (Test-Path "$persDir\local_c1.json")
Check 'copy leaves the source unchanged' ((Hash $workDir) -eq $workBefore)
$out = Run undo
Check 'undo removes the copied chat' (-not (Test-Path "$persDir\local_c1.json"))
$out = Run move -From work -To personal -Chat c2
Check 'move puts the chat in the target and removes it from the source' ((Test-Path "$persDir\local_c2.json") -and -not (Test-Path "$workDir\local_c2.json"))
$out = Run undo
Check 'undo of a move restores the source byte for byte' ((Hash $workDir) -eq $workBefore -and -not (Test-Path "$persDir\local_c2.json"))
$out = Run move -From work -To personal -Chat nothing-like-this
Check 'an unknown chat id changes nothing' ($out -match 'matches 0' -and (Hash $workDir) -eq $workBefore)

Card $PERS 'c1' $s1 'Design review (older copy)' ($now - 9999999)
$out = Run merge -To work
Check 'merge copies only what the target is missing' ((Test-Path "$workDir\local_c3.json") -and ([IO.File]::ReadAllText("$workDir\local_c1.json") -match 'Design review"'))
$out = Run undo
Check 'undo of a merge restores the target' ((Hash $workDir) -eq $workBefore)

Write-Host "`nrescue" -ForegroundColor Cyan
$o1 = '20000000-0000-4000-8000-000000000001'; $o2 = '20000000-0000-4000-8000-000000000002'
Transcript $o1 @(
    ('{"type":"user","message":{"role":"user","content":"<command-name>/init</command-name>"},"timestamp":"2026-09-20T10:00:00.000Z","cwd":"C:\\work\\proj","sessionId":"' + $o1 + '"}'),
    '{"type":"user","message":{"role":"user","content":"fix the translation bug"},"timestamp":"2026-09-20T10:01:00.000Z","cwd":"C:\\work\\proj"}',
    '{"type":"assistant","message":{"model":"claude-opus-5","content":[{"type":"text","text":"done"}]},"timestamp":"2026-09-20T10:05:00.000Z"}',
    ('{"type":"custom-title","customTitle":"' + $persianTitle + '","sessionId":"' + $o1 + '"}')
)
Transcript $o2 @(
    '{"type":"user","message":{"role":"user","content":[{"type":"text","text":"build the login page"}]},"timestamp":"2026-09-21T08:00:00.000Z","cwd":"C:\\work\\site"}'
)
$out = Run rescue
Check 'rescue lists only chats no sidebar has' ($out -match '2 chat\(s\) are on disk' -and $out -notmatch 'Design review')
$out = Run rescue -To work -All
$made = @(Get-ChildItem $workDir -Filter 'local_*.json' | Where-Object { $_.Name -notin 'local_c1.json', 'local_c2.json' })
Check 'rescue writes one record per chat' ($made.Count -eq 2)
$records = @($made | ForEach-Object { [IO.File]::ReadAllText($_.FullName, $utf8) | ConvertFrom-Json })
$first = @($records | Where-Object { $_.cliSessionId -eq $o1 })[0]
$second = @($records | Where-Object { $_.cliSessionId -eq $o2 })[0]
Check 'records have no byte order mark, so JSON.parse accepts them' (@($made | Where-Object { $b = [IO.File]::ReadAllBytes($_.FullName); $b[0] -eq 0xEF }).Count -eq 0)
Check 'the Persian custom title survives' ($first.title -eq $persianTitle)
Check 'without a custom title the first real question becomes the title' ($second.title -eq 'build the login page')
Check 'cwd, model and timestamps come from the transcript' ($first.cwd -eq 'C:\work\proj' -and $first.model -eq 'claude-opus-5' -and $first.createdAt -eq 1789898400000 -and $first.lastActivityAt -eq 1789898700000)
$out = Run rescue
Check 'nothing is left to rescue afterwards' ($out -match 'Every chat history')
$out = Run undo
Check 'undo removes the recovered records' (@(Get-ChildItem $workDir -Filter 'local_*.json').Count -eq 2)

Write-Host "`ndoctor and commands" -ForegroundColor Cyan
$out = Run doctor
Check 'doctor is happy with a clean setup' ($out -notmatch 'junction')
$linkedAccount = '44444444-4444-4444-8444-444444444444'
New-Item -ItemType Directory -Path "$live\claude-code-sessions\$linkedAccount" -Force | Out-Null
New-Item -ItemType Junction -Path "$live\claude-code-sessions\$linkedAccount\$ORG" -Target $persDir | Out-Null
$out = Run doctor
Check 'doctor flags a junctioned chat list' ($out -match 'junction')
[IO.Directory]::Delete("$live\claude-code-sessions\$linkedAccount\$ORG")
$out = Run help
Check 'help lists the commands' ($out -match 'switch <name>' -and $out -match 'rescue')
$out = Run version
Check 'version prints' ($out -match '^\d+\.\d+\.\d+')
$out = Run nonsense
Check 'an unknown command says so' ($out -match "Unknown command 'nonsense'")

Write-Host "`nmenu frames" -ForegroundColor Cyan
$source = [IO.File]::ReadAllText($script) -replace '(?m)^Invoke-Main\s*$', ''
$frames = & {
    . ([scriptblock]::Create($source)) -Command 'noop' -ClaudeDir $live -InstanceDir $vault -ProjectsDir $projects
    $chats = @(Get-Chats "$live\claude-code-sessions\$WORK\$ORG")
    foreach ($spec in @(
        @{ Title = 'What would you like to do?'; Items = (Get-MenuItems); Multi = $false },
        @{ Title = 'Pick chats'; Items = (Get-ChatItems $chats); Multi = $true },
        @{ Title = 'Switch to which account?'; Items = (Get-SwitchItems); Multi = $false }
    )) {
        $state = @{ Title = $spec.Title; Items = $spec.Items; Multi = $spec.Multi; Note = 'note'; Chosen = @{ 0 = $true }; Cursor = 0; Top = 0; Filter = ''; Typing = $false; Status = 'status'; Width = 70; Height = 16 }
        ,(Format-PickerFrame $state).Lines
    }
}
$segments = @($frames | ForEach-Object { $_ } | ForEach-Object { $_ })
$colors = [Enum]::GetNames([ConsoleColor])
Check 'every menu segment names a console colour' (@($segments | Where-Object { $_ -and $colors -notcontains [string]$_[1] }).Count -eq 0)
Check 'no menu line is wider than the window' (@($frames | ForEach-Object { $_ } | Where-Object { (@($_ | ForEach-Object { ([string]$_[0]).Length }) | Measure-Object -Sum).Sum -gt 70 }).Count -eq 0)
Check 'menu details are drawn, not swallowed into the label' (@($segments | Where-Object { [string]$_[0] -match 'DarkGray' }).Count -eq 0)

Remove-Item $root -Recurse -Force
Write-Host ''
$color = if ($script:failures) { 'Red' } else { 'Green' }
Write-Host "  $($script:passes) passed, $($script:failures) failed  ($($PSVersionTable.PSEdition) $($PSVersionTable.PSVersion))" -ForegroundColor $color
exit [int]($script:failures -gt 0)
