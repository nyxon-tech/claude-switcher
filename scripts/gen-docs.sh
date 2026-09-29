#!/bin/sh
# Shell completions and the man page, packaged with every release
set -eu
rm -rf completions manpages
mkdir completions manpages
for shell in bash zsh fish powershell; do
  go run ./cmd/claude-switcher completion "$shell" > "completions/claude-switcher.$shell"
done
mv completions/claude-switcher.powershell completions/claude-switcher.ps1
go run ./cmd/claude-switcher man | gzip -c > manpages/claude-switcher.1.gz
