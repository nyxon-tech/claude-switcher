package platform

import (
	"os"
	"os/exec"
	"path/filepath"
)

func detect() ([]Install, error) {
	config, err := os.UserConfigDir() // $XDG_CONFIG_HOME or ~/.config
	if err != nil {
		return nil, err
	}
	data := filepath.Join(config, "Claude")
	if !isDir(data) {
		return nil, ErrNotFound
	}
	in := Install{Kind: Linux, DataDir: data}
	for _, name := range []string{"claude-desktop", "claude-desktop-unofficial"} {
		if exe, err := exec.LookPath(name); err == nil {
			in.AppPath = exe
			break
		}
	}
	return []Install{in}, nil
}
