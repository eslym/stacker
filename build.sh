#!/usr/bin/env bash

set -e

export GOOS=linux
export CGO_ENABLED=0

mkdir -p build
echo Building for Linux AMD64...
GOARCH=amd64 go build -o build/stacker-amd64 .

echo Building for Linux ARM64...
GOARCH=arm64 go build -o build/stacker-arm64 .
