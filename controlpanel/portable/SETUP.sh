#!/usr/bin/env bash
cd "$(dirname "$0")"
BIN=./evilpro-panel-linux
[ "$(uname -s)" = "Darwin" ] && BIN=./evilpro-panel-mac
[ -x "$BIN" ] || BIN=./evilpro-panel
exec "$BIN" -setup
