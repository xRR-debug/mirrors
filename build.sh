#!/bin/bash
set -e
export CGO_ENABLED=0
export GONOSUMCHECK=*
export GONOSUMDB=*
export GONOPROXY=*
export GOFLAGS="-mod=mod"

echo "=== CS2 Mirrors Build ==="

if [ ! -d "vendor" ]; then
    echo "[!] Скачиваем зависимости (нужен интернет, один раз)..."
    go mod download
    go mod vendor
    echo "[OK] vendor/ создан"
fi

go build -mod=vendor -o mirrors .
echo "[SUCCESS] ./mirrors готов"
