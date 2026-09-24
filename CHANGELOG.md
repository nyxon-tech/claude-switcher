# Changelog

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
