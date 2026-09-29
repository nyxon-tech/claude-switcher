// Package claude reads Claude Desktop's Code-tab chat records ("cards") and chat lists
// ("spaces"). It never writes on its own; ops writes through WriteFileAtomic while Desktop is
// closed.
package claude

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

// Card is one chat record, local_<uuid>.json. Every field is kept as raw JSON in its original
// order, so fields this package does not know survive a rewrite byte for byte.
type Card struct {
	Path   string // file the card was read from, "" for a new card
	keys   []string
	fields map[string]json.RawMessage
}

// ReadCard reads a card file.
func ReadCard(path string) (*Card, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := ParseCard(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c.Path = path
	return c, nil
}

// ParseCard reads a card from JSON. Values are kept compact, otherwise byte for byte.
func ParseCard(data []byte) (*Card, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New("card is not a json object")
	}
	c := &Card{fields: map[string]json.RawMessage{}}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return nil, err
		}
		c.put(tok.(string), buf.Bytes())
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return c, nil
}

// Marshal writes the card as compact JSON without a byte order mark (Desktop reads cards with
// JSON.parse, which rejects one).
func (c *Card) Marshal() []byte {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range c.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(mustEncode(k))
		b.WriteByte(':')
		b.Write(c.fields[k])
	}
	b.WriteByte('}')
	return b.Bytes()
}

// Set replaces a field in place, or adds it at the end.
func (c *Card) Set(key string, value any) error {
	raw, err := encode(value)
	if err != nil {
		return err
	}
	c.put(key, raw)
	return nil
}

func (c *Card) put(key string, raw json.RawMessage) {
	if _, ok := c.fields[key]; !ok {
		c.keys = append(c.keys, key)
	}
	c.fields[key] = raw
}

// ID is the card's id, "local_<uuid>": its file name, or sessionId for a card not yet written.
func (c *Card) ID() string {
	if c.Path != "" {
		return strings.TrimSuffix(filepath.Base(c.Path), ".json")
	}
	return c.str("sessionId")
}

// Session is the chat's current transcript id (cliSessionId), "" before the first message.
func (c *Card) Session() string { return c.str("cliSessionId") }

// PriorSessions are the chat's older transcripts (priorCliSessionIds).
func (c *Card) PriorSessions() []string {
	var ids []string
	c.get("priorCliSessionIds", &ids)
	return ids
}

func (c *Card) Title() string { return c.str("title") }
func (c *Card) Cwd() string   { return c.str("cwd") }
func (c *Card) Model() string { return c.str("model") }

// LastActivity is lastActivityAt in epoch milliseconds, 0 when missing.
func (c *Card) LastActivity() int64 { return c.ms("lastActivityAt") }

// Created is createdAt in epoch milliseconds, 0 when missing.
func (c *Card) Created() int64 { return c.ms("createdAt") }

func (c *Card) Archived() bool {
	var b bool
	c.get("isArchived", &b)
	return b
}

func (c *Card) str(key string) string {
	var s string
	c.get(key, &s)
	return s
}

func (c *Card) ms(key string) int64 {
	var n float64
	c.get(key, &n)
	return int64(n)
}

// get decodes a field into v, leaving v alone when the field is missing or of another type.
func (c *Card) get(key string, v any) {
	if raw, ok := c.fields[key]; ok {
		_ = json.Unmarshal(raw, v) // a field of an unexpected type reads as missing
	}
}

// NewCard builds the card for a chat rescued from its transcript, with the same fields v2 wrote.
func NewCard(m transcript.Meta) *Card {
	c := &Card{fields: map[string]json.RawMessage{}}
	add := func(k string, v any) { c.put(k, mustEncode(v)) }
	add("sessionId", "local_"+newUUID())
	add("cliSessionId", m.Session)
	add("cwd", m.Cwd)
	add("originCwd", m.Cwd)
	add("lastFocusedAt", millis(m.End))
	add("createdAt", millis(m.Start))
	add("lastActivityAt", millis(m.End))
	add("isArchived", false)
	add("title", TitleFor(m))
	add("titleSource", "auto")
	add("permissionMode", "default")
	add("remoteMcpServersConfig", []any{})
	add("alwaysAllowedReasons", []any{})
	add("sessionPermissionUpdates", []any{})
	add("spawnSeed", struct{}{})
	if strings.HasPrefix(m.Model, "claude-") {
		add("model", m.Model)
	}
	return c
}

// RecoveredTitle names a rescued chat whose transcript has neither a title nor a prompt.
const RecoveredTitle = "Recovered chat"

// TitleFor is the title a rescued chat gets: the transcript's title, else its first prompt.
func TitleFor(m transcript.Meta) string {
	for _, s := range []string{m.Title, m.FirstPrompt} {
		if t := CleanTitle(s); t != "" {
			return t
		}
	}
	return RecoveredTitle
}

// CleanTitle puts a title on one line and cuts it to 80 characters.
func CleanTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= 80 {
		return s
	}
	return string([]rune(s)[:79]) + "…"
}

func millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// encode is json.Marshal without HTML escaping, so titles keep < > & as typed.
func encode(v any) (json.RawMessage, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// mustEncode encodes strings, numbers, bools and empty containers, which cannot fail.
func mustEncode(v any) json.RawMessage {
	raw, err := encode(v)
	if err != nil {
		panic(err)
	}
	return raw
}
