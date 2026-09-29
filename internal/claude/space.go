package claude

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Space is one chat list: Desktop keeps the chats of each account and organization in
// <data>/claude-code-sessions/<account>/<org>.
type Space struct {
	Account string
	Org     string
	Dir     string
}

// Spaces lists every chat list in a Desktop data folder, links to folders included.
func Spaces(dataDir string) ([]Space, error) {
	root := filepath.Join(dataDir, "claude-code-sessions")
	accounts, err := subdirs(root)
	if err != nil {
		return nil, err
	}
	var out []Space
	for _, account := range accounts {
		orgs, err := subdirs(filepath.Join(root, account))
		if err != nil {
			return nil, err
		}
		for _, org := range orgs {
			out = append(out, Space{Account: account, Org: org, Dir: filepath.Join(root, account, org)})
		}
	}
	return out, nil
}

// subdirs lists the folders in dir, following links; a missing dir has none.
func subdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && st.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Cards reads every local_*.json card. Cards that cannot be read are left out, as Desktop's own
// half-written *.tmp files are.
func (s Space) Cards() ([]*Card, error) {
	names, err := s.files("local_")
	if err != nil {
		return nil, err
	}
	var cards []*Card
	for _, name := range names {
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if c, err := ReadCard(filepath.Join(s.Dir, name)); err == nil {
			cards = append(cards, c)
		}
	}
	return cards, nil
}

// Tombstones are the transcript ids of chats deleted in the app (files named deleted_<id>).
func (s Space) Tombstones() ([]string, error) {
	names, err := s.files("deleted_")
	for i, name := range names {
		names[i] = strings.TrimPrefix(name, "deleted_")
	}
	return names, err
}

func (s Space) files(prefix string) ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Linked reports whether the list's account or org folder is a junction or symbolic link.
// Desktop reads chats through one but writes new ones elsewhere, so they vanish.
func (s Space) Linked() bool {
	for _, dir := range []string{filepath.Dir(s.Dir), s.Dir} {
		if st, err := os.Lstat(dir); err == nil && st.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return true
		}
	}
	return false
}

// LiveAccount is the account Desktop is signed into, "" when it is signed out. Only the
// lastKnownAccountUuid key of config.json is read; the rest of the file holds credentials.
func LiveAccount(dataDir string) string {
	data, err := os.ReadFile(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		LastKnownAccountUUID string `json:"lastKnownAccountUuid"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.LastKnownAccountUUID
}

// WriteFileAtomic writes data to a temp file next to path and renames it over path, so Desktop
// never sees a half-written card.
func WriteFileAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
