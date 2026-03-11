@echo off
echo === Build for Linux ===
echo.
echo Choose server architecture:
echo   1 - x86_64  (most VPS)
echo   2 - aarch64 (Oracle ARM, Hetzner CAX)
echo   3 - armv7l  (Raspberry Pi)
echo.
set /p ARCH="Enter 1, 2 or 3: "

set CGO_ENABLED=0
set GOOS=linux

if "%ARCH%"=="1" set GOARCH=amd64
if "%ARCH%"=="2" set GOARCH=arm64
if "%ARCH%"=="3" set GOARCH=arm

if "%GOARCH%"=="" (
    echo [ERROR] Invalid choice
    pause
    exit /b 1
)

echo [..] Building linux/%GOARCH%...

if exist vendor (
    go build -mod=vendor -o mirrors .
) else (
    go mod download
    go mod vendor
    go build -mod=vendor -o mirrors .
)

if %errorlevel% == 0 (
    echo [SUCCESS] Done: mirrors
) else (
    echo [FAILED]
)
pause
