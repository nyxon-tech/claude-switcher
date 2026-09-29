#!/bin/sh
# Claude Switcher by Nyxon - installer for macOS and Linux
#   curl -fsSL https://raw.githubusercontent.com/nyxon-tech/claude-switcher/main/install.sh | sh
# Installs to ~/.local/bin (or $CLAUDE_SWITCHER_BIN) for the current user only. The download is
# checked against the release's checksums.txt before anything is installed.
set -eu

repo=nyxon-tech/claude-switcher
version=${CLAUDE_SWITCHER_VERSION:-latest}
bin=${CLAUDE_SWITCHER_BIN:-$HOME/.local/bin}

case "$(uname -s)" in
  Darwin) os=darwin arch=all ;;
  Linux)
    os=linux
    case "$(uname -m)" in
      x86_64 | amd64) arch=amd64 ;;
      aarch64 | arm64) arch=arm64 ;;
      *) echo "Unsupported CPU: $(uname -m)" >&2; exit 1 ;;
    esac ;;
  *)
    echo "On Windows run in PowerShell:" >&2
    echo "  irm https://raw.githubusercontent.com/$repo/main/install.ps1 | iex" >&2
    exit 1 ;;
esac

if [ "$version" = latest ]; then
  base="https://github.com/$repo/releases/latest/download"
else
  base="https://github.com/$repo/releases/download/$version"
fi
file="claude-switcher_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cd "$tmp"
curl -fsSLO "$base/$file"
curl -fsSLO "$base/checksums.txt"
grep "  $file\$" checksums.txt > want.txt || { echo "$file is not listed in checksums.txt" >&2; exit 1; }
if command -v sha256sum > /dev/null 2>&1; then sha256sum -c want.txt > /dev/null; else shasum -a 256 -c want.txt > /dev/null; fi

mkdir -p "$bin"
tar -xzf "$file" claude-switcher
mv claude-switcher "$bin/claude-switcher"
chmod 755 "$bin/claude-switcher"

echo
echo "  Claude Switcher $("$bin/claude-switcher" version --short) installed to $bin"
case ":$PATH:" in
  *":$bin:"*) echo "  Run: claude-switcher" ;;
  *) echo "  Add it to your PATH, e.g. in ~/.zshrc or ~/.bashrc:  export PATH=\"$bin:\$PATH\"" ;;
esac
echo
