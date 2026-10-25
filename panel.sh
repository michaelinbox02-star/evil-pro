#!/usr/bin/env bash
# Launch the control panel (linux/macOS).
set -e
cd "$(dirname "$0")"

BIN=build/evilpro-panel
if [ ! -x "$BIN" ]; then
  echo "building control panel first..."
  ./build-panel.sh
fi

BIND="${PANEL_BIND:-127.0.0.1:8080}"
USER="${PANEL_USER:-admin}"
exec "$BIN" -bind "$BIND" -user "$USER" -pass "${PANEL_PASS:-}"
