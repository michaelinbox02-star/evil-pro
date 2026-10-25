#!/usr/bin/env bash
# Build the engine (linux amd64+arm64) and the control panel for this machine.
set -e
cd "$(dirname "$0")"
export CGO_ENABLED=0

echo "[1/3] evilginx engine  linux/amd64"
GOOS=linux GOARCH=amd64 go build -mod=mod -o build/evilginx-linux .

echo "[2/3] evilginx engine  linux/arm64"
GOOS=linux GOARCH=arm64 go build -mod=mod -o build/evilginx-linux-arm64 .

echo "[3/3] control panel     $(go env GOOS)/$(go env GOARCH)"
go build -mod=mod -o build/evilpro-panel ./controlpanel

ls -lh build/evilginx-linux build/evilginx-linux-arm64 build/evilpro-panel
