package store

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Journal ops are the two things a change does to a file, as v2 wrote them.
const (
	OpCreated = "created" // the file did not exist; undo removes it
	OpRemoved = "removed" // the file was removed or overwritten; undo copies Backup back
)

// Entry is one file a change touched.
type Entry struct {
	Op     string
	Path   string
	Backup string // copy of the file before the change, "" for created files
}

// Journal records one change so it can be undone byte for byte. On disk it is
// _journal/<yyyyMMdd-HHmmss-fff>/journal.json plus the backups, and an "undone" file once undone.
type Journal struct {
	ID      string // folder name: the local time the change started
	Dir     string
	Action  string // copy, move, merge or rescue
	At      time.Time
	Summary string
	Entries []Entry
	Undone  bool
}

type journalFile struct {
	Action  string  `json:"action"`
	At      string  `json:"at"`
	Summary string  `json:"summary"`
	Entries []Entry `json:"entries"`
}

const stampLayout = "20060102-150405.000"

// NewJournal starts a journal for a change made at now.
func (v Vault) NewJournal(action string, now time.Time) (*Journal, error) {
	root := filepath.Join(v.Dir, "_journal")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	for t := now; ; t = t.Add(time.Millisecond) {
		id := strings.Replace(t.Format(stampLayout), ".", "-", 1)
		dir := filepath.Join(root, id)
		err := os.Mkdir(dir, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue // two changes in the same millisecond
		}
		if err != nil {
			return nil, err
		}
		return &Journal{ID: id, Dir: dir, Action: action, At: now}, nil
	}
}

// Created records a file about to be created. Call it before writing the file.
func (j *Journal) Created(path string) error {
	j.Entries = append(j.Entries, Entry{Op: OpCreated, Path: path})
	return j.write()
}

// Removed backs up a file about to be removed or overwritten. Call it before changing the file.
func (j *Journal) Removed(path string) error {
	backup := filepath.Join(j.Dir, fmt.Sprintf("%d-%s", len(j.Entries), filepath.Base(path)))
	if err := copyFile(path, backup); err != nil {
		return err
	}
	j.Entries = append(j.Entries, Entry{Op: OpRemoved, Path: path, Backup: backup})
	return j.write()
}

// Save finishes the journal with its summary. A journal that recorded nothing is deleted.
func (j *Journal) Save(summary string) error {
	j.Summary = summary
	if len(j.Entries) == 0 {
		return os.RemoveAll(j.Dir)
	}
	return j.write()
}

// write rewrites journal.json after every entry, so a change cut short can still be undone. It
// goes through a temp file: a torn journal.json could not be read, so the change could not be undone.
func (j *Journal) write() error {
	data, err := json.MarshalIndent(journalFile{j.Action, j.At.Format(time.RFC3339Nano), j.Summary, j.Entries}, "", "\t")
	if err != nil {
		return err
	}
	path := filepath.Join(j.Dir, "journal.json")
	if err := os.WriteFile(path+".tmp", data, 0o600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

// Journals lists every readable journal that changed something, newest first, v2's included
// (v2 also saved journals of changes that wrote nothing).
func (v Vault) Journals() ([]Journal, error) {
	root := filepath.Join(v.Dir, "_journal")
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Journal
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if j, err := readJournal(filepath.Join(root, e.Name())); err == nil && len(j.Entries) > 0 {
			out = append(out, j)
		}
	}
	// By time, not by folder name: names are local time, which repeats when daylight saving ends.
	slices.SortFunc(out, func(a, b Journal) int { return cmp.Or(b.At.Compare(a.At), strings.Compare(b.ID, a.ID)) })
	return out, nil
}

func readJournal(dir string) (Journal, error) {
	data, err := os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil {
		return Journal{}, err
	}
	var f journalFile
	if err := json.Unmarshal(data, &f); err != nil {
		return Journal{}, err
	}
	at, _ := time.Parse(time.RFC3339Nano, f.At) // v2 wrote .NET's round-trip format, which parses the same
	_, err = os.Stat(filepath.Join(dir, "undone"))
	return Journal{ID: filepath.Base(dir), Dir: dir, Action: f.Action, At: at, Summary: f.Summary, Entries: f.Entries, Undone: err == nil}, nil
}

// Undo reverses a journal's entries, newest first, and marks it undone. If any file cannot be
// put back, the journal stays open so Undo can be tried again.
func (v Vault) Undo(j Journal) error {
	var errs []error
	for _, e := range slices.Backward(j.Entries) {
		switch e.Op {
		case OpCreated:
			if err := os.Remove(e.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		case OpRemoved:
			errs = append(errs, copyFile(e.Backup, e.Path))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(j.Dir, "undone"), []byte(time.Now().Format(time.RFC3339Nano)), 0o600)
}
