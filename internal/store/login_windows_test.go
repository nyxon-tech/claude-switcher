package store

import (
	"os"
	"path/filepath"
	"testing"
)

// A config.json that cannot be removed must leave every other login item in place.
func TestClearLoginStopsAtLockedConfig(t *testing.T) {
	data := filepath.Join(t.TempDir(), "Claude")
	fillLogin(t, data, "work")
	before := login(t, data)
	f, err := os.Open(filepath.Join(data, "config.json")) // on Windows os.Open withholds delete sharing
	must(t, err)
	defer f.Close()
	if ClearLogin(data, items) == nil {
		t.Fatal("removing a locked config.json should fail")
	}
	if login(t, data) != before {
		t.Error("items after config.json were removed")
	}
}
