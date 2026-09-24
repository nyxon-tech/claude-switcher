# Changelog

## 2.0.2

### Fixed
- 2.0.1 stopped with `Cannot convert value " " to type "System.ConsoleColor"` as soon as the menu opened. Rows holding a single coloured segment were unrolled into that segment's text and colour, so a character of the text was read as a colour. `tests/console.ps1` now draws the menu in a real console window, which the fixture tests cannot do.

## 2.0.1

### Fixed
- Recover offered every older part of a chat as a separate lost chat. A Desktop chat is several history files (a `/clear`, a restart or a resume starts a new one under the same title) and its sidebar record points only at the newest, so recovering "everything" filled a list with duplicates. Recover now leaves out files whose title and project match a chat you still have, files that another file continues, and chats you deleted in the app, and offers each lost chat once at its newest file. On the machine that hit it, 54 offers became 6.
- Typing a letter in a multi-select list no longer acts on it: `A` used to select everything, so typing a title that contained an "a" picked every chat. Letters now start a search, and Ctrl+A selects all.
- Every move, copy and recovery asks to confirm the number of chats first.
- Holding an arrow key no longer freezes the menu. Only the rows that change are redrawn, queued key presses are applied together, and the status line is read once per screen instead of on every frame.

## 2.0.0 — Claude Switcher by Nyxon

The fork of [claude-profile-switcher](https://github.com/NeezerGu/claude-profile-switcher) becomes Claude Switcher.

### New
- Interactive menu when run with no arguments: arrow keys, search, multi-select, Esc to go back.
- Move or copy Claude Code chats between accounts, chat by chat or all at once.
- Merge: copy every chat an account is missing from all the others.
- Recover: rebuild sidebar entries for chat histories on disk that no account lists, with their real title, project, model and dates.
- Undo for every chat change, restored byte for byte from a journal.
- `doctor` checks the install, the signed-in account and every chat list, flags junctioned chat lists, and warns about Claude Code's 30-day history cleanup.
- `new <name>` adds an account without logging out of the saved one.
- `rename`, `remove`, `accounts`, `chats`, `version` commands.
- One-line installer with a `claude-switcher` command and a Start menu shortcut, no admin rights.
- Tests that run in PowerShell 7 and Windows PowerShell 5.1, and CI on Windows.

### Changed
- Finds the Microsoft Store install in `%LOCALAPPDATA%\Packages\Claude_*\LocalCache`, where `%APPDATA%\Claude` does not exist.
- Waits only for Claude Desktop itself: a Claude Code CLI is also named `claude.exe`, and `vmwp` belongs to every Hyper-V VM such as WSL2 or Docker.
- Swaps `buddy-tokens.json` and `plan-usage-history.json`, which Desktop 2.x keeps per account.
- Loading a profile clears every login file first, so nothing from the previous account survives.
- Refuses to save over a profile when Desktop was signed into a different account by hand.
- Profile markers and records are UTF-8 without a byte order mark, so non-Latin names and titles survive Windows PowerShell 5.1.

### Removed
- The Chinese README, which described 1.0. Translations of the new README are welcome.

## 1.0.0 — claude-profile-switcher

The original by [@NeezerGu](https://github.com/NeezerGu): save and switch Claude Desktop login profiles, keep the Cowork VM shared, repair its session disk.
