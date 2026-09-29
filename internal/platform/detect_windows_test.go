package platform

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// mkfile creates path (and its folders) with the given modification time.
func mkfile(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsInstalls(t *testing.T) {
	old, recent := time.Now().Add(-time.Hour), time.Now()

	t.Run("most recently used first", func(t *testing.T) {
		local, roaming := t.TempDir(), t.TempDir()
		msix := msixData(filepath.Join(local, "Packages", msixFamily))
		mkfile(t, filepath.Join(msix, "config.json"), old)
		mkfile(t, filepath.Join(roaming, "Claude", "config.json"), recent)
		mkfile(t, filepath.Join(local, "AnthropicClaude", "claude.exe"), old)

		want := []Install{
			{Kind: Squirrel, DataDir: filepath.Join(roaming, "Claude"), LogDir: filepath.Join(roaming, "Claude", "logs"),
				AppPath: filepath.Join(local, "AnthropicClaude", "claude.exe")},
			{Kind: MSIX, DataDir: msix, LogDir: filepath.Join(local, "Claude", "logs"), AppID: "Claude_pzs8sxrjxfjjc!Claude"},
		}
		if got := windowsInstalls(local, roaming); !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v\nwant %+v", got, want)
		}
	})

	t.Run("other package family", func(t *testing.T) {
		local, roaming := t.TempDir(), t.TempDir()
		mkfile(t, filepath.Join(msixData(filepath.Join(local, "Packages", "Claude_otherhash")), "config.json"), old)
		mkfile(t, filepath.Join(roaming, "Claude", "Preferences"), old) // no login or chats: not an install

		got := windowsInstalls(local, roaming)
		if len(got) != 1 || got[0].AppID != "Claude_otherhash!Claude" {
			t.Errorf("got %+v, want one MSIX install of Claude_otherhash", got)
		}
	})

	t.Run("nothing installed", func(t *testing.T) {
		if got := windowsInstalls(t.TempDir(), t.TempDir()); len(got) != 0 {
			t.Errorf("got %+v, want none", got)
		}
	})

	t.Run("roaming folder redirected to the package", func(t *testing.T) {
		local, roaming := t.TempDir(), t.TempDir()
		msix := msixData(filepath.Join(local, "Packages", msixFamily))
		mkfile(t, filepath.Join(msix, "config.json"), old)
		if err := os.Symlink(msix, filepath.Join(roaming, "Claude")); err != nil {
			t.Skipf("symlinks need developer mode here: %v", err)
		}
		if got := windowsInstalls(local, roaming); len(got) != 1 || got[0].Kind != MSIX {
			t.Errorf("got %+v, want only the MSIX install", got)
		}
	})
}
