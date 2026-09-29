package platform

import (
	"os"
	"path/filepath"
)

func detect() ([]Install, error) {
	support, err := os.UserConfigDir() // ~/Library/Application Support
	if err != nil {
		return nil, err
	}
	data := filepath.Join(support, "Claude")
	if !isDir(data) {
		return nil, ErrNotFound
	}
	in := Install{
		Kind:    MacApp,
		DataDir: data,
		LogDir:  filepath.Join(Home(), "Library", "Logs", "Claude"),
		AppID:   "com.anthropic.claudefordesktop",
	}
	for _, app := range []string{"/Applications/Claude.app", filepath.Join(Home(), "Applications", "Claude.app")} {
		if isDir(app) {
			in.AppPath = app
			break
		}
	}
	return []Install{in}, nil
}
