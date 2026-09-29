package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type obj = map[string]any

const sid = "0b6c1f7e-2d7a-4c1e-9d65-3f0a1b2c3d4e"

// writeJSONL writes one JSON line per value; strings are written as they are.
func writeJSONL(t *testing.T, path string, lines ...any) string {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		if s, ok := l.(string); ok {
			b.WriteString(s)
		} else {
			j, err := json.Marshal(l)
			if err != nil {
				t.Fatal(err)
			}
			b.Write(j)
		}
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fixture writes lines to <tmp>/projects/slug/<sid>.jsonl.
func fixture(t *testing.T, lines ...any) string {
	return writeJSONL(t, filepath.Join(t.TempDir(), "projects", "C--work-app", sid+".jsonl"), lines...)
}

func msg(typ, ts string, extra obj) obj {
	l := obj{"type": typ, "cwd": "/work/app", "gitBranch": "main", "sessionId": sid, "uuid": "u-" + ts, "timestamp": ts}
	for k, v := range extra {
		l[k] = v
	}
	return l
}

func user(ts string, content any) obj {
	return msg("user", ts, obj{"message": obj{"role": "user", "content": content}})
}

func reply(ts, id, model string, blocks ...obj) obj {
	usage := obj{"input_tokens": 1, "output_tokens": 10, "cache_creation_input_tokens": 100, "cache_read_input_tokens": 1000}
	return msg("assistant", ts, obj{"message": obj{"id": id, "model": model, "role": "assistant", "content": blocks, "usage": usage}})
}

func queued(ts, mode, prompt string) obj {
	return msg("attachment", ts, obj{"attachment": obj{"type": "queued_command", "commandMode": mode, "prompt": prompt}})
}

func text(s string) obj { return obj{"type": "text", "text": s} }
func tool(name string) obj {
	return obj{"type": "tool_use", "id": "toolu_1", "name": name, "input": obj{}}
}

func meta(typ, key, value string) obj { return obj{"type": typ, key: value, "sessionId": sid} }
