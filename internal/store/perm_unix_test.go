//go:build !windows

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavedLoginIsPrivate(t *testing.T) {
	root := t.TempDir()
	data, v := filepath.Join(root, "Claude"), Vault{Dir: filepath.Join(root, "vault")}
	fillLogin(t, data, "work")
	os.Chmod(filepath.Join(data, "Preferences"), 0o644)
	must(t, v.SaveLogin("work", data, items))
	for path, want := range map[string]os.FileMode{
		v.Dir:                                   0o700,
		filepath.Join(v.Dir, "work"):            0o700,
		filepath.Join(v.Dir, "work", "Network"): 0o700,
		filepath.Join(v.Dir, "work", "Preferences"):        0o600,
		filepath.Join(v.Dir, "work", "Network", "Cookies"): 0o600,
	} {
		st, err := os.Stat(path)
		if err != nil {
			t.Error(err)
		} else if st.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v", path, st.Mode().Perm(), want)
		}
	}
}
