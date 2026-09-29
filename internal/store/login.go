package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// SaveLogin copies Desktop's login items from dataDir into the profile: files are copied,
// folders replaced whole, and items Desktop does not have are removed from the profile.
func (v Vault) SaveLogin(name, dataDir string, items []string) error {
	if v.Dir == "" || dataDir == "" || name == "" {
		return errors.New("save login: empty path")
	}
	dest := filepath.Join(v.Dir, name)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return err
	}
	return replaceAll(dataDir, dest, items)
}

// RestoreLogin makes dataDir signed in as the profile: every login item is replaced by the
// profile's copy, and items the profile lacks are removed, so nothing of the previous account
// lingers.
func (v Vault) RestoreLogin(name, dataDir string, items []string) error {
	if v.Dir == "" || dataDir == "" || name == "" {
		return errors.New("restore login: empty path")
	}
	return replaceAll(filepath.Join(v.Dir, name), dataDir, items)
}

// ClearLogin removes every login item from dataDir, so Desktop opens signed out. It stops at the
// first error: while config.json (the first item) is there, Desktop looks signed in and a retry
// would save what is left over the profile, so the rest must stay until config.json is gone.
func ClearLogin(dataDir string, items []string) error {
	if dataDir == "" {
		return errors.New("clear login: empty path")
	}
	for _, it := range items {
		if err := os.RemoveAll(filepath.Join(dataDir, it)); err != nil {
			return err
		}
	}
	return nil
}

func replaceAll(from, to string, items []string) error {
	for _, it := range items {
		if err := replace(filepath.Join(from, it), filepath.Join(to, it)); err != nil {
			return err
		}
	}
	return nil
}

// replace makes dst a copy of src, or removes dst when src does not exist.
func replace(src, dst string) error {
	st, err := os.Stat(src)
	if errors.Is(err, fs.ErrNotExist) {
		return os.RemoveAll(dst)
	}
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst)
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case d.Type().IsRegular():
			return copyFile(path, target)
		}
		return nil // links and devices are not part of a login
	})
}

// copyFile copies one file, readable only by the user: saved logins hold credentials.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
}
