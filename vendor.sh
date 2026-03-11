#!/bin/bash
echo "=== Скачивание зависимостей (нужен интернет, один раз) ==="
export CGO_ENABLED=0
go mod tidy && go mod vendor && echo "[OK] vendor/ создан. Теперь ./build.sh не нужен интернет."
