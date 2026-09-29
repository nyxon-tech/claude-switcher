package cli

import (
	"os"
	"path/filepath"
	"testing"
	"unicode"
)

func TestShortPathAvoidsPersianNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "پارسا")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got := shortPath(dir)
	if got == dir {
		t.Skip("this volume keeps no 8.3 names")
	}
	for _, r := range got {
		if r > unicode.MaxASCII {
			t.Fatalf("shortPath(%q) = %q, still outside ASCII", dir, got)
		}
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("shortPath gave a path that does not exist: %v", err)
	}
}
