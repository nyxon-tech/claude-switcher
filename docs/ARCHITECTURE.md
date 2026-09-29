# Architecture

Claude Switcher is one Go binary. It switches Claude Desktop between saved accounts and manages
the Code-tab chats of every account. It only touches files on this machine and makes no network
requests, except `update` and a once-a-day release check that you can turn off.

```
cmd/claude-switcher        main: builds the CLI and runs it
internal/platform          where Claude Desktop lives on each OS, its processes, quit and launch
internal/claude            Desktop's chat records ("cards") and chat lists ("spaces")
internal/transcript        Claude Code transcripts: metadata, messages, export, usage
internal/store             our own state: saved logins, current profile, journal, settings
internal/ops               every action (switch, copy, rescue, undo...). The only writer of Claude data
internal/rtl               Persian and Arabic for terminals: letter joining and right-to-left order
internal/i18n              English and Persian text, digits, dates (Jalali), relative time
internal/ui                theme, logo, layout helpers and the full-screen app (Bubble Tea)
internal/cli               commands (Cobra + Fang); every UI action is also a command
```

Dependencies point downwards only: `cli` and `ui` use `ops`; `ops` uses `platform`, `claude`,
`transcript` and `store`; `claude` uses `transcript`; `rtl` and `i18n` use nothing of ours.

## Data on disk

| What | Where |
|---|---|
| Desktop data folder | Windows MSIX `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Roaming\Claude`, Windows classic `%APPDATA%\Claude`, macOS `~/Library/Application Support/Claude`, Linux `~/.config/Claude` |
| Chat lists | `<data>/claude-code-sessions/<accountUuid>/<orgUuid>/local_<uuid>.json` (one card per chat), `deleted_<cliSessionId>` tombstones |
| Transcripts | `~/.claude/projects/<slug>/<cliSessionId>.jsonl` (or `$CLAUDE_CONFIG_DIR/projects`), shared by every account |
| Signed-in account | `<data>/config.json` key `lastKnownAccountUuid` |
| Our vault | `~/.claude-instances/` (same layout as v2): `<profile>/` login snapshot + `_account`, `_current_profile`, `_journal/<stamp>/journal.json`, `settings.json`, `cache/` |

Login items swapped per profile (missing ones are skipped): `config.json`, `Preferences`, `DIPS`,
`DIPS-wal`, `SharedStorage`, `SharedStorage-wal`, `InterestGroups`, `InterestGroups-wal`, `Cookies`,
`Cookies-journal`, `ant-did`, `buddy-tokens.json`, `plan-usage-history.json`,
`cowork-enabled-cli-ops.json` and the folders `Local Storage`, `Session Storage`, `Network`,
`IndexedDB`, `WebStorage`, `Shared Dictionary`.

Never touched: `Local State` (holds the cookie encryption key), `claude_desktop_config.json`,
`vm_bundles`, `claude-code*`, `claude-code-sessions` (already per account), the macOS Keychain,
`~/.claude/.credentials.json`, `~/.claude/config.json`.

## Rules every change must keep

1. **Claude's data is only written by `ops`, and only while Desktop is closed.** Every write goes
   through the journal first, so `undo` restores the exact bytes.
2. **Cards are rewritten losslessly.** A card is a `map[string]json.RawMessage`; fields we do not
   own are kept byte for byte. Files are written compact, UTF-8 without a BOM, via a temp file and
   rename.
3. **Newest wins.** When two accounts hold the same chat, the card with the larger
   `lastActivityAt` is the current one. Copy and merge never replace a newer card with an older one.
4. **A chat is several transcripts.** A card's `cliSessionId` is the current part and
   `priorCliSessionIds` lists older parts. Rescue never offers an older part, a tombstoned chat, or
   a chat another card already titles in the same project.
5. **No credentials are read.** Login items are copied as opaque files. Only the
   `lastKnownAccountUuid` key of `config.json` is parsed.
6. **Every string shown in a terminal goes through `rtl`,** which reorders and joins Persian in
   terminals that cannot (Windows Terminal, conhost, VS Code) and leaves it alone in terminals that
   can (macOS Terminal, iTerm2 3.7+, Konsole).

## Extending

- **A new OS or install type:** add a file in `internal/platform` that returns an `Install` and
  implements `Desktop`.
- **A new language:** add `internal/i18n/locales/<lang>/*.json` with the same file names, keys and
  placeholders as `locales/en`, and register its rules (direction, digits, calendar) in the i18n
  package. The catalog tests check the rest.
- **A new command:** add an `ops` function, then a command in `internal/cli` and, if it needs a
  screen, a screen in `internal/ui`. Commands accept `--json` and return stable exit codes.
- **A theme:** add a palette in `internal/ui/theme.go`.
