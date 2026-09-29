# Changelog

## 3.0.0

A rewrite from scratch in Go: one binary for Windows, macOS and Linux, with a full-screen app and a command line.

### New
- **macOS and Linux.** Claude Desktop is found on every OS, including the Windows MSIX and older `.exe` installs side by side. Desktop is asked to quit where the OS allows it (macOS, Linux); on Windows you quit it from the tray and the app waits.
- **A full-screen app** with tabs: Accounts, Chats, Activity, Usage, Doctor and Settings. Mouse support, dialogs, a step-by-step view of every change, and help for every key.
- **Chats from every account in one list**, with search as you type, a preview of any chat (first and last prompt, models, dates, recent messages), and export to a web page or Markdown.
- **Usage**: tokens by model, the last 30 days and the top projects, counted from local transcripts with every API message counted once.
- **Commands for everything**, with `--json`, `--yes`, `--no-launch`, stable exit codes, shell completions and a man page.
- **`update`** installs the latest release after checking its checksum, and an optional once-a-day check tells you when one is out.
- **Installers** that verify the SHA-256 checksum before installing, for Windows (`install.ps1`) and macOS/Linux (`install.sh`).

### Fixed
- Chat titles in Persian or Arabic showed as `????`. They now display correctly, with letters joined and right-to-left order, even in terminals that cannot lay out right-to-left text themselves, such as Windows Terminal.
- Recover now reads which older transcripts belong to a chat from Desktop's own records (`priorCliSessionIds`), so an older part of a chat you still have is never offered as a lost chat.
- Undo skips empty journal entries and orders changes by their recorded time, so it always reverses the change you just made.
- Switching refuses when Desktop is signed into an account no profile has saved, instead of losing that login.
- The doctor's warning about the 30-day history cleanup now matches Claude Code: chats from Desktop are kept at any age unless `desktopSessionCleanupPeriodDays` is set.

### Compatibility
- Saved logins in `~/.claude-instances` and the undo journal from v2 work unchanged. Running the v2 install line upgrades in place.

## 2.0.3

### Fixed
- Copying a chat the other account already had did nothing, even when your copy was newer: it said the chat was `already in` that account and left the old one. A chat continued in one account (a resume or a restart points its sidebar record at a newer history file) now replaces the older copy, and undo puts the older one back. Merge does the same. A copy that is newer in the target is still left alone.

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
