package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const canRepair = true

// rebuildDisk creates an empty expandable disk with diskpart, which needs administrator rights:
// Windows shows its elevation prompt, and this waits until diskpart is done.
func rebuildDisk(disk string) error {
	script, err := os.CreateTemp("", "claude-switcher-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(script.Name())
	disk = filepath.Join(shortPath(filepath.Dir(disk)), filepath.Base(disk))
	_, err = fmt.Fprintf(script, "create vdisk file=\"%s\" maximum=1024 type=expandable\r\nexit\r\n", disk)
	if cerr := script.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	quoted := strings.ReplaceAll(shortPath(script.Name()), "'", "''")
	return exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf(`Start-Process diskpart -ArgumentList '/s "%s"' -Verb RunAs -Wait -WindowStyle Hidden`, quoted)).Run()
}

// shortPath is the 8.3 form of an existing path. diskpart reads its script in the console's code
// page, so a folder name outside it (a Persian user name) must not reach it. Volumes without
// short names keep the path as it is.
func shortPath(path string) string {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return path
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return path
	}
	return windows.UTF16ToString(buf[:n])
}
