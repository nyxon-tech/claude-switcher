package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Method is how Claude Switcher was installed, read from where its executable lives.
type Method string

const (
	Installer Method = "installer" // install.ps1, install.sh or a downloaded archive: update replaces the file
	Winget    Method = "winget"
	Scoop     Method = "scoop"
	Brew      Method = "brew"
	Go        Method = "go"      // go install
	Package   Method = "package" // a .deb or .rpm package
)

// MethodOf tells how the executable at exe was installed.
func MethodOf(exe string) Method {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	p := strings.ToLower(filepath.ToSlash(exe))
	gobin := os.Getenv("GOBIN")
	switch {
	case strings.Contains(p, "/winget/packages/") || strings.Contains(p, "/winget/links/"):
		return Winget
	case strings.Contains(p, "/scoop/"):
		return Scoop
	case strings.Contains(p, "/caskroom/") || strings.Contains(p, "/homebrew/") || strings.Contains(p, "/linuxbrew/"):
		return Brew
	case strings.Contains(p, "/go/bin/") || gobin != "" && filepath.Dir(exe) == filepath.Clean(gobin):
		return Go
	case strings.HasPrefix(p, "/usr/bin/"):
		return Package
	}
	return Installer
}

// Upgrade is the command that updates a package-manager install; "" for Installer and Package.
func (m Method) Upgrade() string {
	return map[Method]string{
		Winget: "winget upgrade Nyxon.ClaudeSwitcher",
		Scoop:  "scoop update claude-switcher",
		Brew:   "brew upgrade nyxon-tech/tap/claude-switcher",
		Go:     "go install github.com/nyxon-tech/claude-switcher/v3/cmd/claude-switcher@latest",
	}[m]
}

// Asset is the release archive for an OS and CPU. macOS has one universal binary ("all").
func Asset(goos, goarch string) string {
	ext := "tar.gz"
	switch goos {
	case "windows":
		ext = "zip"
	case "darwin":
		goarch = "all"
	}
	return fmt.Sprintf("claude-switcher_%s_%s.%s", goos, goarch, ext)
}

// Install replaces the executable exe with the latest release, after checking the download
// against the release's checksums.txt.
func Install(ctx context.Context, client *http.Client, exe string) error {
	asset := Asset(runtime.GOOS, runtime.GOARCH)
	sums, err := get(ctx, client, Releases+"/latest/download/checksums.txt")
	if err != nil {
		return err
	}
	want, err := checksum(sums, asset)
	if err != nil {
		return err
	}
	archive, err := get(ctx, client, Releases+"/latest/download/"+asset)
	if err != nil {
		return err
	}
	if got := sha256.Sum256(archive); hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("%s does not match checksums.txt", asset)
	}
	bin, err := extract(archive, asset)
	if err != nil {
		return err
	}
	return replace(exe, bin)
}

// Cleanup deletes the executable an update on Windows had to leave behind.
func Cleanup(exe string) { _ = os.Remove(exe + ".old") }

func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", path.Base(url), resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// checksum finds name's SHA-256 in a checksums.txt ("<hex>  <name>" per line).
func checksum(sums []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s is not in checksums.txt", name)
}

// extract takes the executable out of a release archive.
func extract(archive []byte, asset string) ([]byte, error) {
	name := "claude-switcher"
	if strings.HasSuffix(asset, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if path.Base(f.Name) == name+".exe" {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, errors.New("no executable in " + asset)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, errors.New("no executable in " + asset)
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeReg && path.Base(h.Name) == name {
			return io.ReadAll(tr)
		}
	}
}

// replace swaps exe for bin. A running executable cannot be overwritten on Windows but it can be
// renamed, so the old one moves aside to exe.old and Cleanup deletes it at the next start.
func replace(exe string, bin []byte) error {
	next, old := exe+".new", exe+".old"
	if err := os.WriteFile(next, bin, 0o755); err != nil {
		return err
	}
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		os.Remove(next)
		return err
	}
	if err := os.Rename(next, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}
