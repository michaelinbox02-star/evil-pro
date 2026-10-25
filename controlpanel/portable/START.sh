#!/usr/bin/env bash
# evil-pro control panel - portable launcher (linux/macOS)
set -e
cd "$(dirname "$0")"

BIN=./evilpro-panel-linux
[ "$(uname -s)" = "Darwin" ] && BIN=./evilpro-panel-mac
[ -x "$BIN" ] || BIN=./evilpro-panel
if [ ! -x "$BIN" ]; then
  echo "panel binary not found in this folder"; exit 1
fi

if [ ! -f panel.json ]; then
  echo
  echo "  First run - let's set up your login and port."
  echo
  "$BIN" -setup
fi

exec "$BIN"
