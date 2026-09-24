# Draws the menu in a real console, which tests/run.ps1 cannot: SetCursorPosition and colours only
# exist there. Nothing is written to Claude's data. Run it in a terminal window:
#   pwsh -File tests/console.ps1          powershell -File tests/console.ps1
param([string]$Log = '')
$ErrorActionPreference = 'Stop'
$source = [IO.File]::ReadAllText((Join-Path (Split-Path $PSScriptRoot -Parent) 'claude-switcher.ps1')) -replace '(?m)^Invoke-Main\s*$', ''
$steps = New-Object System.Collections.ArrayList
try {
    . ([scriptblock]::Create($source)) -Command 'noop'
    function Key([ConsoleKey]$Key, [char]$Char = [char]0, [switch]$Ctrl) { [ConsoleKeyInfo]::new($Char, $Key, $false, $false, [bool]$Ctrl) }
    function Screen([string]$Title, [object[]]$Items, [switch]$Multi, [ConsoleKeyInfo[]]$Keys) {
        $state = @{ Title = $Title; Items = $Items; Multi = [bool]$Multi; Note = 'note'; Chosen = @{}; Cursor = 0; Top = 0; Filter = ''; Typing = $false; Status = (Get-StatusLine) }
        $state.Width = [Math]::Max(40, [Console]::WindowWidth - 1)
        $state.Height = [Math]::Max(12, [Console]::WindowHeight - 1)
        $shown = @{}
        Clear-Host
        $origin = [Console]::WindowTop
        Write-Frame (Format-PickerFrame $state).Lines $shown $origin $state.Width $state.Height
        foreach ($k in $Keys) {
            $frame = Format-PickerFrame $state
            [void](Step-Picker $state $frame $k)
            Write-Frame (Format-PickerFrame $state).Lines $shown $origin $state.Width $state.Height
        }
        [void]$steps.Add("$Title drawn, cursor $($state.Cursor), $($state.Chosen.Count) chosen, filter '$($state.Filter)'")
    }
    $demo = @(1..60 | ForEach-Object { [pscustomobject]@{ Label = "Chat number $_"; Detail = "project  $($G.Dot)  $($_)d ago"; Value = $_ } })
    Screen 'What would you like to do?' (Get-MenuItems) -Keys @((Key DownArrow), (Key DownArrow), (Key DownArrow), (Key UpArrow))
    $held = @(1..45 | ForEach-Object { Key DownArrow }) + @(Key PageUp)
    Screen 'Pick chats' $demo -Multi -Keys ($held + @((Key Spacebar), (Key A 'a'), (Key B 'b'), (Key Enter), (Key A ([char]1) -Ctrl)))
    Clear-Host
    $result = "OK  $($PSVersionTable.PSEdition) $($PSVersionTable.PSVersion)`n  " + ($steps -join "`n  ")
}
catch {
    Clear-Host
    $result = "FAIL  $($PSVersionTable.PSEdition) $($PSVersionTable.PSVersion): $($_.Exception.Message)`n  at $($_.InvocationInfo.PositionMessage)"
}
[Console]::ResetColor()
if ($Log) { [IO.File]::WriteAllText($Log, $result) } else { Write-Host $result }
