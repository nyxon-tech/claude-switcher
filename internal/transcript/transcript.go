// Package transcript reads Claude Code transcripts (~/.claude/projects/<slug>/<session>.jsonl):
// what a chat is called, what was asked, which models answered, and how many tokens it used.
// Everything here is read-only.
package transcript

import "time"

// Meta is what the chat list, the preview and rescue need to know about one transcript.
type Meta struct {
	Session     string    // file name without .jsonl, equals the lines' sessionId
	Path        string    // full path of the .jsonl file
	Size        int64     // bytes
	Cwd         string    // working folder of the chat
	Title       string    // last custom-title, else agent-name, ai-title, first then last prompt; "" if none
	FirstPrompt string    // first real human prompt, one line, at most 300 characters
	LastPrompt  string    // last human prompt (last-prompt line or last user text), one line
	Model       string    // model of the last assistant message ("claude-opus-5"), "" if none
	Start, End  time.Time // earliest and latest message timestamp (lines are not in order)
	Prompts     int       // human prompts, including queued ones
	Replies     int       // assistant messages, deduplicated by message.id
	Branch      string    // last gitBranch seen, "" if none
}

// HasMessages reports whether the transcript holds a conversation, not only metadata lines.
func (m Meta) HasMessages() bool { return m.Prompts > 0 || m.Replies > 0 }

// Usage counts tokens the way the API bills them.
type Usage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheWrite int64 `json:"cacheWrite"`
	CacheRead  int64 `json:"cacheRead"`
	Messages   int   `json:"messages"`
}

// Total is every token of the usage.
func (u Usage) Total() int64 { return u.Input + u.Output + u.CacheWrite + u.CacheRead }

// Add returns the sum of two usages.
func (u Usage) Add(o Usage) Usage {
	return Usage{u.Input + o.Input, u.Output + o.Output, u.CacheWrite + o.CacheWrite, u.CacheRead + o.CacheRead, u.Messages + o.Messages}
}

// Message is one turn shown in a preview or an export.
type Message struct {
	ID    string    // the line's uuid
	Role  string    // "user" or "assistant"
	Time  time.Time // when it was written
	Text  string    // the text the human typed or the assistant wrote; tool calls are listed in Tools
	Tools []string  // tool names the assistant called in this turn ("Bash", "Edit")
	Model string    // assistant model, "" for user turns
}

// Stats is the usage dashboard: tokens per model, per day and per project folder.
type Stats struct {
	ByModel    map[string]Usage `json:"byModel"`
	ByDay      map[string]Usage `json:"byDay"`     // key "2006-01-02", local time
	ByProject  map[string]Usage `json:"byProject"` // key: cwd
	Sessions   int              `json:"sessions"`  // main transcripts with at least one reply
	First      time.Time        `json:"first"`
	Last       time.Time        `json:"last"`
	FilesRead  int              `json:"filesRead"` // transcripts parsed this run (the rest came from the cache)
	FilesTotal int              `json:"filesTotal"`
}
