package claude

import (
	"os/exec"
	"testing"
)

// makeLink creates a directory junction, which needs no admin rights (a symlink does).
func makeLink(t *testing.T, target, link string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("cannot create a junction: %v %s", err, out)
	}
}
