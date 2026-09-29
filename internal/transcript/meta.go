package transcript

import (
	"bufio"
	"cmp"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	headSize  = 256 << 10 // QuickMeta reads this much from the start: cwd, first prompt
	tailSize  = 64 << 10  // and this much from the end: titles, last prompt, last model
	titleMax  = 80
	promptMax = 300
)

// QuickMeta reads only the head and the tail of a transcript (fast, for lists and rescue).
// Prompts and Replies stay 0; Start and End come from the lines it read. It falls back to a
// full read when the head and the tail hold no message line.
func QuickMeta(path string) (Meta, error) {
	f, size, err := open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	b := newMetaBuilder(path, size)
	if size > headSize+tailSize {
		err = b.headTail(f, size)
	}
	if err == nil && b.m.Cwd == "" {
		b = newMetaBuilder(path, size)
		err = eachLine(newReader(f), b.add)
	}
	if err != nil {
		return Meta{}, err
	}
	m := b.meta()
	m.Prompts, m.Replies = 0, 0
	return m, nil
}

// ReadMeta reads the whole transcript (for the preview pane): exact counts and time span.
func ReadMeta(path string) (Meta, error) {
	f, size, err := open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()
	b := newMetaBuilder(path, size)
	if err := eachLine(newReader(f), b.add); err != nil {
		return Meta{}, err
	}
	return b.meta(), nil
}

func open(path string) (*os.File, int64, error) {
	f, err := openShared(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// metaBuilder folds the lines of one transcript into a Meta.
type metaBuilder struct {
	m                               Meta
	customTitle, agentName, aiTitle string
	lastPromptLine, lastHuman       string
	replies                         map[string]bool
}

func newMetaBuilder(path string, size int64) *metaBuilder {
	session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	return &metaBuilder{m: Meta{Session: session, Path: path, Size: size}, replies: map[string]bool{}}
}

// add folds one line in; it always returns true so it can be passed to eachLine.
func (b *metaBuilder) add(l *line) bool {
	switch l.Type {
	case "custom-title":
		setIf(&b.customTitle, l.CustomTitle)
	case "agent-name":
		setIf(&b.agentName, l.AgentName)
	case "ai-title":
		setIf(&b.aiTitle, l.AITitle)
	case "last-prompt":
		setIf(&b.lastPromptLine, l.LastPrompt)
	case "assistant":
		if l.Message.Model != synthetic {
			setIf(&b.m.Model, l.Message.Model)
			b.replies[cmp.Or(l.Message.ID, l.UUID)] = true
		}
	}
	if cmd, args, ok := l.humanPrompt(); ok {
		b.m.Prompts++
		b.lastHuman = cmp.Or(args, cmd)
		if b.m.FirstPrompt == "" {
			b.m.FirstPrompt = oneLine(b.lastHuman, promptMax)
		}
	}
	if l.envelope() {
		b.m.Cwd = cmp.Or(b.m.Cwd, l.Cwd)
		setIf(&b.m.Branch, l.GitBranch)
		if t := l.time(); !t.IsZero() {
			if b.m.Start.IsZero() || t.Before(b.m.Start) {
				b.m.Start = t
			}
			if t.After(b.m.End) {
				b.m.End = t
			}
		}
	}
	return true
}

// headTail reads the first headSize bytes, stopping once cwd, the first prompt and a model are
// known (the tail may hold no reply), then the last tailSize bytes.
func (b *metaBuilder) headTail(f *os.File, size int64) error {
	head := newReader(io.NewSectionReader(f, 0, headSize))
	err := eachLine(head, func(l *line) bool {
		return b.add(l) && (b.m.Cwd == "" || b.m.FirstPrompt == "" || b.m.Model == "")
	})
	if err != nil {
		return err
	}
	// Start one byte early so a line beginning exactly at the cut is not lost with the torn one.
	tail := newReader(io.NewSectionReader(f, size-tailSize-1, tailSize+1))
	skipLine(tail)
	return eachLine(tail, b.add)
}

func skipLine(br *bufio.Reader) {
	for {
		if _, err := br.ReadSlice('\n'); !errors.Is(err, bufio.ErrBufferFull) {
			return
		}
	}
}

func setIf(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

func (b *metaBuilder) meta() Meta {
	m := b.m
	m.Replies = len(b.replies)
	m.LastPrompt = oneLine(cmp.Or(b.lastPromptLine, b.lastHuman), promptMax)
	m.Title = oneLine(cmp.Or(b.customTitle, b.agentName, b.aiTitle, m.FirstPrompt, m.LastPrompt), titleMax)
	return m
}
