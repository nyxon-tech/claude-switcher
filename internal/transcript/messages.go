package transcript

import (
	"encoding/json"
	"strings"
)

// ReadMessages returns the human prompts and assistant replies in order, without tool results,
// thinking, meta lines and attachments (queued prompts are kept). One API message split over
// several lines becomes one Message. limit > 0 keeps only the last limit messages.
func ReadMessages(path string, limit int) ([]Message, error) {
	var out []Message
	lastID := "" // message.id of out's last entry when it is an assistant reply
	err := readLines(path, func(l *line) {
		if cmd, args, ok := l.humanPrompt(); ok {
			text := strings.TrimSpace(cmd + " " + args)
			out = append(out, Message{ID: l.UUID, Role: "user", Time: l.time(), Text: text})
			lastID = ""
			return
		}
		if l.Type != "assistant" || l.Message.Model == synthetic {
			return
		}
		text, tools := replyContent(l.Message.Content)
		if l.Message.ID != "" && l.Message.ID == lastID {
			last := &out[len(out)-1]
			last.Text = joinText(last.Text, text)
			last.Tools = append(last.Tools, tools...)
			return
		}
		if text == "" && len(tools) == 0 {
			return
		}
		out = append(out, Message{ID: l.UUID, Role: "assistant", Time: l.time(), Text: text, Tools: tools, Model: l.Message.Model})
		lastID = l.Message.ID
	})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// replyContent returns the text blocks and tool names of assistant content.
func replyContent(raw json.RawMessage) (string, []string) {
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var text string
	var tools []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text = joinText(text, strings.TrimSpace(b.Text))
		case "tool_use":
			tools = append(tools, b.Name)
		}
	}
	return text, tools
}

func joinText(a, b string) string {
	if a == "" || b == "" {
		return a + b
	}
	return a + "\n\n" + b
}
