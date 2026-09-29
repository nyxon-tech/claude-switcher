package ops

import (
	"io"
	"path/filepath"

	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// Preview reads a chat's transcript for the preview pane: its metadata and last 20 messages.
func (e *Env) Preview(c Chat) (transcript.Meta, []transcript.Message, error) {
	m, err := e.meta(c)
	if err != nil {
		return m, nil, err
	}
	msgs, err := transcript.ReadMessages(c.Transcript, 20)
	return m, msgs, err
}

// Export writes a whole chat as "md" (Markdown) or "html" (one self-contained page).
func (e *Env) Export(c Chat, format string, w io.Writer) error {
	export := map[string]func(io.Writer, transcript.Meta, []transcript.Message) error{
		"md": transcript.WriteMarkdown, "html": transcript.WriteHTML,
	}[format]
	if export == nil {
		return errf(BadFormat, "format", format)
	}
	m, err := e.meta(c)
	if err != nil {
		return err
	}
	msgs, err := transcript.ReadMessages(c.Transcript, 0)
	if err != nil {
		return err
	}
	return export(w, m, msgs)
}

// meta reads a chat's transcript, titled as the chat list shows it.
func (e *Env) meta(c Chat) (transcript.Meta, error) {
	if c.Transcript == "" {
		return transcript.Meta{}, errf(NoHistory)
	}
	m, err := e.ReadMeta(c.Transcript)
	if c.Title != "" {
		m.Title = c.Title
	}
	return m, err
}

// Usage adds up token usage over every transcript, cached in the vault between runs.
// progress may be nil.
func (e *Env) Usage(progress func(done, total int)) (transcript.Stats, error) {
	return transcript.Collect(e.Projects, filepath.Join(e.Vault.Dir, "cache", "usage.gob"), progress)
}
