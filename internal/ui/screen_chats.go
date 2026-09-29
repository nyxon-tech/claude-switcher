package ui

import "github.com/nyxon-tech/claude-switcher/v3/internal/i18n"

// chatsScreen will be the chat browser (lists, search, preview, copy, move, export, rescue).
type chatsScreen struct{ placeholder }

func (*chatsScreen) Title() string { return i18n.T("tab.chats") }
