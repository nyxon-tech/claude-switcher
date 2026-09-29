// Claude Switcher switches Claude Desktop between saved accounts without signing in again, and
// manages the Code-tab chats of every account.
package main

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/nyxon-tech/claude-switcher/v3/internal/cli"
)

// Set by the release build: -ldflags "-X main.version=3.0.0 -X main.commit=abc1234 -X main.date=...".
var version, commit, date = "dev", "", ""

func main() {
	os.Exit(cli.Execute(build()))
}

// build fills in what the Go toolchain recorded when -ldflags did not: a build from a checkout
// stays "dev" with its commit, and `go install ...@v3.0.0` knows its version.
func build() cli.Build {
	b := cli.Build{Version: version, Commit: commit, Date: date}
	info, ok := debug.ReadBuildInfo()
	if b.Version != "dev" || !ok {
		return b
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Commit = s.Value[:min(7, len(s.Value))]
		case "vcs.time":
			b.Date = s.Value
		}
	}
	if v := info.Main.Version; b.Commit == "" && v != "" && v != "(devel)" {
		b.Version = strings.TrimPrefix(v, "v")
	}
	return b
}
