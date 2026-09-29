package transcript

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Index maps every main transcript's session id to its path: <projects>/<slug>/<uuid>.jsonl.
// Subagent files, .orphaned-* and .superseded-* copies are left out. A missing projects folder
// is an empty index.
func Index(projectsDir string) (map[string]string, error) {
	out := map[string]string{}
	slugs, err := os.ReadDir(projectsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		dir := filepath.Join(projectsDir, slug.Name())
		files, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue // removed while we listed
		}
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if id, ok := sessionID(f.Name()); ok && !f.IsDir() {
				out[id] = filepath.Join(dir, f.Name())
			}
		}
	}
	return out, nil
}

// sessionID returns the uuid of a main transcript file name "<uuid>.jsonl".
func sessionID(name string) (string, bool) {
	id, ok := strings.CutSuffix(name, ".jsonl")
	return id, ok && isUUID(id)
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}
