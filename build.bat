@echo off
echo === CS2 Mirrors Build ===
set CGO_ENABLED=0

if not exist vendor (
    echo [ERROR] Папка vendor/ не найдена!
    echo Сначала запусти vendor.bat с интернетом - это нужно сделать ОДИН РАЗ.
    pause
    exit /b 1
)

go build -mod=vendor -o mirrors.exe .

if %errorlevel% == 0 (
    echo [SUCCESS] Собрано: mirrors.exe
) else (
    echo [FAILED] Ошибка сборки
)
pause
