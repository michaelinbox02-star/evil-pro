#!/bin/sh
# Register the evil-pro native host with your browsers, so "Own profile" works.
set -e
cd "$(dirname "$0")"

BIN=bin/evilpro-host-linux
[ "$(uname)" = "Darwin" ] && BIN=bin/evilpro-host-mac

if [ ! -x "$BIN" ] && [ ! -f "$BIN" ]; then
  echo "host binary not found at $BIN"
  echo "build it with:  go build -o \"$BIN\" ./extension/host"
  exit 1
fi
chmod +x "$BIN"

echo "Registering the evil-pro native host..."
"$BIN" install
echo
echo "Done. \"Own profile\" is now available in the extension popup."
echo "Restart the browser if it was already running."
