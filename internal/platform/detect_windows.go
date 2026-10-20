package platform

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const msixFamily = "Claude_pzs8sxrjxfjjc"

func detect() ([]Install, error) {
	local, err1 := os.UserCacheDir()    // %LOCALAPPDATA%
	roaming, err2 := os.UserConfigDir() // %APPDATA%
	if err := errors.Join(err1, err2); err != nil {
		return nil, err
	}
	ins := windowsInstalls(local, roaming)
	if len(ins) == 0 {
		return nil, ErrNotFound
	}
	return ins, nil
}

// windowsInstalls finds the MSIX and Squirrel installs under the given AppData folders, the
// most recently used first.
func windowsInstalls(local, roaming string) []Install {
	ins, pkgData := msixInstalls(local)
	data := filepath.Join(roaming, "Claude")
	// Inside Desktop's MSIX container %APPDATA%\Claude is redirected to the package copy,
	// so it can be the MSIX folder seen under another name, even one Desktop has moved out of.
	used := exists(filepath.Join(data, "config.json")) || exists(filepath.Join(data, "claude-code-sessions"))
	if used && !slices.ContainsFunc(pkgData, func(dir string) bool { return sameDir(dir, data) }) {
		in := Install{Kind: Squirrel, DataDir: data, LogDir: filepath.Join(data, "logs")}
		if exe := filepath.Join(local, "AnthropicClaude", "claude.exe"); exists(exe) {
			in.AppPath = exe
		}
		ins = append(ins, in)
	}
	slices.SortStableFunc(ins, func(a, b Install) int { return lastUsed(b.DataDir).Compare(lastUsed(a.DataDir)) })
	return ins
}

// msixInstalls returns the Claude package with the known family name or, when it is missing,
// any Claude_* package (another publisher or architecture). It also returns the data folders
// inside the packages, which Desktop may have left for movedData.
func msixInstalls(local string) ([]Install, []string) {
	pkgs := []string{filepath.Join(local, "Packages", msixFamily)}
	if !isDir(msixData(pkgs[0])) && !isDir(movedData(local, pkgs[0])) {
		pkgs, _ = filepath.Glob(filepath.Join(local, "Packages", "Claude_*"))
	}
	var ins []Install
	var pkgData []string
	for _, pkg := range pkgs {
		pkgData = append(pkgData, msixData(pkg))
		if data := activeData(local, pkg); data != "" {
			ins = append(ins, Install{
				Kind:    MSIX,
				DataDir: data,
				LogDir:  filepath.Join(local, "Claude", "logs"),
				AppID:   filepath.Base(pkg) + "!Claude",
			})
		}
	}
	return ins, pkgData
}

func msixData(pkg string) string { return filepath.Join(pkg, "LocalCache", "Roaming", "Claude") }

// movedData is where newer Desktop builds keep their data outside the package:
// %LOCALAPPDATA%\Claude-Data\<package family in lower case>.
func movedData(local, pkg string) string {
	return filepath.Join(local, "Claude-Data", strings.ToLower(filepath.Base(pkg)))
}

// activeData is the data folder Desktop of package pkg uses, or "" when it has none. Once Desktop
// moves its data out of the package it leaves a .user-data-left.<family> file in the old folder,
// whose login files may linger there but are no longer read.
func activeData(local, pkg string) string {
	old, moved := msixData(pkg), movedData(local, pkg)
	left := exists(filepath.Join(old, ".user-data-left."+strings.ToLower(filepath.Base(pkg))))
	switch {
	case isDir(moved) && (left || !isDir(old)):
		return moved
	case isDir(old):
		return old
	}
	return ""
}

// lastUsed is when Desktop last wrote its login or chat list in dir.
func lastUsed(dir string) time.Time {
	var t time.Time
	for _, name := range []string{"config.json", "claude-code-sessions"} {
		if fi, err := os.Stat(filepath.Join(dir, name)); err == nil && fi.ModTime().After(t) {
			t = fi.ModTime()
		}
	}
	return t
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sameDir(a, b string) bool {
	fa, err1 := os.Stat(a)
	fb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}
