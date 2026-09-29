//go:build !windows

package ops

import (
	"os"
	"testing"
)

func makeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
