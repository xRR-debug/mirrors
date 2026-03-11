@echo off
echo === Скачивание зависимостей (нужен интернет, один раз) ===
set CGO_ENABLED=0
set GONOSUMCHECK=*
set GOFLAGS=-mod=mod

go mod tidy
if %errorlevel% neq 0 (
    echo [ERROR] go mod tidy упал. Проверь интернет.
    pause
    exit /b 1
)

go mod vendor
if %errorlevel% neq 0 (
    echo [ERROR] go mod vendor упал.
    pause
    exit /b 1
)

echo.
echo [OK] Папка vendor/ создана. Теперь запускай build.bat - интернет не нужен.
pause
