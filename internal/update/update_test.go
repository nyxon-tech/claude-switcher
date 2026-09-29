package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeGitHub serves a release: the latest-release redirect, checksums.txt and one archive.
func fakeGitHub(t *testing.T, tag string, archive []byte, sum string) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	asset := Asset(runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/nyxon-tech/claude-switcher/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/latest/download/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0123  other.zip\n" + sum + "  " + asset + "\n"))
	})
	mux.HandleFunc("/releases/latest/download/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(archive) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := Releases
	Releases = srv.URL + "/releases"
	t.Cleanup(func() { Releases = old })
	return &hits
}

func TestLatest(t *testing.T) {
	fakeGitHub(t, "v3.1.0", nil, "")
	v, err := Latest(context.Background(), http.DefaultClient)
	if err != nil || v != "3.1.0" {
		t.Fatalf("Latest = %q, %v", v, err)
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"3.0.1", "3.0.0", true},
		{"v3.1.0", "3.0.9", true},
		{"3.0.0", "3.0.0", false},
		{"3.0.0", "3.0.1", false},
		{"10.0.0", "9.9.9", true},
		{"3.0.0", "3.0.0-rc.2", true},
		{"3.0.0-rc.2", "3.0.0", false},
		{"3.0.0-rc.10", "3.0.0-rc.2", true},
		{"3.0.0-rc.1", "3.0.0-beta", true},
		{"3.0.0-rc.1.1", "3.0.0-rc.1", true},
		{"3.0.1+build", "3.0.0", true},
		{"3.0.1", "dev", false},
		{"dev", "3.0.0", false},
		{"", "3.0.0", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestNotifierChecksOnceADay(t *testing.T) {
	hits := fakeGitHub(t, "v3.2.0", nil, "")
	cache := filepath.Join(t.TempDir(), "cache", "update.json")
	n := Notifier{Current: "3.0.0", Cache: cache}
	ctx := context.Background()
	if got := n.Newer(ctx); got != "3.2.0" || hits.Load() != 1 {
		t.Fatalf("first check = %q after %d requests", got, hits.Load())
	}
	if got := n.Newer(ctx); got != "3.2.0" || hits.Load() != 1 {
		t.Fatalf("a check within a day should use the cache: %q after %d requests", got, hits.Load())
	}
	stale, _ := json.Marshal(lastCheck{Checked: time.Now().Add(-25 * time.Hour), Latest: "3.0.0"})
	os.WriteFile(cache, stale, 0o600)
	if got := n.Newer(ctx); got != "3.2.0" || hits.Load() != 2 {
		t.Fatalf("a day-old check should ask again: %q after %d requests", got, hits.Load())
	}
	if got := (Notifier{Current: "3.2.0", Cache: cache}).Newer(ctx); got != "" {
		t.Errorf("the latest version is not newer than itself: %q", got)
	}
	for _, n := range []Notifier{{Current: "3.0.0", Cache: cache, Off: true}, {Current: "dev", Cache: cache}} {
		os.Remove(cache)
		if got := n.Newer(ctx); got != "" || hits.Load() != 2 {
			t.Errorf("%+v should not check: %q after %d requests", n, got, hits.Load())
		}
	}
}

func TestDisabled(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if Disabled(getenv) {
		t.Error("checks are on by default")
	}
	for _, k := range []string{"CLAUDE_SWITCHER_NO_UPDATE_CHECK", "DO_NOT_TRACK", "CI"} {
		env = map[string]string{k: "1"}
		if !Disabled(getenv) {
			t.Errorf("%s should turn checks off", k)
		}
	}
}

func TestMethodOf(t *testing.T) {
	t.Setenv("GOBIN", "")
	for exe, want := range map[string]Method{
		`C:\Users\a\AppData\Local\Microsoft\WinGet\Packages\Nyxon.ClaudeSwitcher_Microsoft.Winget.Source_8wekyb3d8bbwe\claude-switcher.exe`: Winget,
		`C:\Users\a\AppData\Local\Microsoft\WinGet\Links\claude-switcher.exe`:                                                               Winget,
		`C:\Users\a\scoop\apps\claude-switcher\current\claude-switcher.exe`:                                                                 Scoop,
		"/opt/homebrew/Caskroom/claude-switcher/3.0.0/claude-switcher":                                                                      Brew,
		"/home/linuxbrew/.linuxbrew/bin/claude-switcher":                                                                                    Brew,
		"/home/a/go/bin/claude-switcher":                                                                                                    Go,
		"/usr/bin/claude-switcher":                                                                                                          Package,
		`C:\Users\a\AppData\Local\Programs\claude-switcher\claude-switcher.exe`:                                                             Installer,
		"/home/a/.local/bin/claude-switcher":                                                                                                Installer,
	} {
		exe = strings.ReplaceAll(exe, `\`, string(filepath.Separator))
		if runtime.GOOS != "windows" {
			exe = strings.ReplaceAll(exe, `\`, "/")
		}
		if got := MethodOf(exe); got != want {
			t.Errorf("MethodOf(%s) = %s, want %s", exe, got, want)
		}
	}
	if Winget.Upgrade() != "winget upgrade Nyxon.ClaudeSwitcher" || Installer.Upgrade() != "" {
		t.Error("upgrade commands")
	}
}

func TestAsset(t *testing.T) {
	for _, c := range [][3]string{
		{"windows", "amd64", "claude-switcher_windows_amd64.zip"},
		{"windows", "arm64", "claude-switcher_windows_arm64.zip"},
		{"darwin", "arm64", "claude-switcher_darwin_all.tar.gz"},
		{"linux", "amd64", "claude-switcher_linux_amd64.tar.gz"},
	} {
		if got := Asset(c[0], c[1]); got != c[2] {
			t.Errorf("Asset(%s, %s) = %s", c[0], c[1], got)
		}
	}
}

// releaseArchive builds this OS's release archive holding bin as the executable.
func releaseArchive(t *testing.T, bin []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		for name, data := range map[string][]byte{"README.md": []byte("readme"), "claude-switcher.exe": bin} {
			w, _ := zw.Create(name)
			w.Write(data)
		}
		zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range map[string][]byte{"README.md": []byte("readme"), "claude-switcher": bin} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg})
		tw.Write(data)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestInstall(t *testing.T) {
	archive := releaseArchive(t, []byte("new build"))
	sum := sha256.Sum256(archive)
	fakeGitHub(t, "v3.1.0", archive, hex.EncodeToString(sum[:]))
	exe := filepath.Join(t.TempDir(), "claude-switcher.exe")
	os.WriteFile(exe, []byte("old build"), 0o755)

	if err := Install(context.Background(), http.DefaultClient, exe); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new build" {
		t.Fatalf("exe holds %q", got)
	}
	if got, _ := os.ReadFile(exe + ".old"); string(got) != "old build" {
		t.Fatalf("the old build should wait in .old for Cleanup, got %q", got)
	}
	Cleanup(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Error("Cleanup should delete the old build")
	}
}

func TestInstallRefusesABadChecksum(t *testing.T) {
	fakeGitHub(t, "v3.1.0", releaseArchive(t, []byte("tampered")), strings.Repeat("0", 64))
	exe := filepath.Join(t.TempDir(), "claude-switcher.exe")
	os.WriteFile(exe, []byte("old build"), 0o755)
	err := Install(context.Background(), http.DefaultClient, exe)
	if err == nil || !strings.Contains(err.Error(), "checksums.txt") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(exe); string(got) != "old build" {
		t.Errorf("a bad download must leave the exe alone, got %q", got)
	}
}
