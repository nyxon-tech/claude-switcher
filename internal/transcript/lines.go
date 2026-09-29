package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// synthetic is the model name of API-error placeholders, which are not real replies.
const synthetic = "<synthetic>"

// line holds the fields of a transcript line this package uses. Heavy fields (toolUseResult,
// image data) are left out so they are skipped, and content is kept raw until needed.
type line struct {
	Type                      string `json:"type"`
	UUID                      string `json:"uuid"`
	Timestamp                 string `json:"timestamp"`
	Cwd                       string `json:"cwd"`
	GitBranch                 string `json:"gitBranch"`
	IsMeta                    bool   `json:"isMeta"`
	IsCompactSummary          bool   `json:"isCompactSummary"`
	IsVisibleInTranscriptOnly bool   `json:"isVisibleInTranscriptOnly"`
	Message                   struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
	Attachment struct {
		Type        string          `json:"type"`
		CommandMode string          `json:"commandMode"`
		Prompt      json.RawMessage `json:"prompt"`
	} `json:"attachment"`
	CustomTitle string `json:"customTitle"`
	AgentName   string `json:"agentName"`
	AITitle     string `json:"aiTitle"`
	LastPrompt  string `json:"lastPrompt"`
}

// time returns the line's timestamp, zero when it has none.
func (l *line) time() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, l.Timestamp)
	return t
}

// envelope reports whether the line is a message line carrying cwd, branch and timestamp.
func (l *line) envelope() bool {
	switch l.Type {
	case "user", "assistant", "system", "attachment":
		return true
	}
	return false
}

// humanPrompt returns the prompt a person wrote on a user line or a queued-prompt attachment.
// For a slash command cmd is "/name" and args its arguments; otherwise cmd is "" and args is
// the text. ok is false for anything a person did not type.
func (l *line) humanPrompt() (cmd, args string, ok bool) {
	var raw json.RawMessage
	switch {
	case l.Type == "user" && !l.IsMeta && !l.IsCompactSummary && !l.IsVisibleInTranscriptOnly:
		raw = l.Message.Content
	case l.Type == "attachment" && l.Attachment.Type == "queued_command" && l.Attachment.CommandMode == "prompt":
		raw = l.Attachment.Prompt
	default:
		return "", "", false
	}
	text, ok := humanText(raw)
	if !ok {
		return "", "", false
	}
	return splitPrompt(text)
}

// Machine-written user text that must not count as a prompt.
var notPrompts = []string{"<local-command-caveat>", "<local-command-stdout>", "<task-notification>", "[Request interrupted"}

func splitPrompt(text string) (cmd, args string, ok bool) {
	text = strings.TrimSpace(text)
	for _, p := range notPrompts {
		if strings.HasPrefix(text, p) {
			return "", "", false
		}
	}
	if strings.HasPrefix(text, "<command-name>") || strings.HasPrefix(text, "<command-message>") {
		name := strings.TrimPrefix(tagText(text, "command-name"), "/")
		if name == "" {
			name = tagText(text, "command-message")
		}
		args := tagText(text, "command-args")
		return "/" + name, args, name != "" || args != ""
	}
	return "", text, text != ""
}

// tagText returns the trimmed text inside <tag>...</tag>, "" when the tag is missing.
func tagText(s, tag string) string {
	_, rest, ok := strings.Cut(s, "<"+tag+">")
	if !ok {
		return ""
	}
	inner, _, _ := strings.Cut(rest, "</"+tag+">")
	return strings.TrimSpace(inner)
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

// humanText returns the text of a user content value (a string or blocks), with images and
// documents as placeholders. Tool result blocks are skipped; ok is false when nothing is left.
func humanText(raw json.RawMessage) (string, bool) {
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		return s, json.Unmarshal(raw, &s) == nil
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			parts = append(parts, b.Text)
		case "image", "document":
			parts = append(parts, "["+b.Type+"]")
		}
	}
	return strings.Join(parts, "\n"), len(parts) > 0
}

// oneLine collapses all whitespace to single spaces and cuts the result to n runes.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// scanLines calls fn with every non-empty line of br until fn returns false. Lines may be any
// length; the slice is only valid during the call.
func scanLines(br *bufio.Reader, fn func([]byte) bool) error {
	var long []byte
	for {
		chunk, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			long = append(long, chunk...)
			continue
		}
		data := chunk
		if len(long) > 0 {
			long = append(long, chunk...)
			data = long
		}
		if len(bytes.TrimSpace(data)) > 0 && !fn(data) {
			return nil
		}
		long = long[:0]
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// eachLine decodes every line of br and calls fn until it returns false. Lines that are not
// valid JSON (a torn last line, a cut head) are skipped.
func eachLine(br *bufio.Reader, fn func(*line) bool) error {
	var l line
	return scanLines(br, func(b []byte) bool {
		l = line{}
		if json.Unmarshal(b, &l) != nil {
			return true
		}
		return fn(&l)
	})
}

// openShared opens a transcript for reading. On Windows os.Open withholds delete sharing, so
// while we read, Claude could not rename or delete the file; opens through os.Root share it.
func openShared(path string) (*os.File, error) {
	return os.OpenInRoot(filepath.Dir(path), filepath.Base(path))
}

// readLines opens path and calls fn with every decoded line.
func readLines(path string, fn func(*line)) error {
	f, err := openShared(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return eachLine(newReader(f), func(l *line) bool { fn(l); return true })
}

func newReader(r io.Reader) *bufio.Reader { return bufio.NewReaderSize(r, 64<<10) }
