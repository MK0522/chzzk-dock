@echo off
set "PATH=C:\Program Files\Go\bin;%PATH%"
echo ======================================================
echo   CHZZK OBS Dock - Go Single Binary Build
echo ======================================================
echo.
echo [*] Dependencies tidy...
go mod tidy

echo [*] Generating Windows PE Resource (Icon ^& Manifest)...
del /f /q rsrc_*.syso >nul 2>&1
go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -64 -o resource_windows_amd64.syso

echo [*] Building chzzk-dock.exe with embedded icon...
go build -ldflags="-H windowsgui -s -w" -o chzzk-dock.exe .
if %errorlevel% equ 0 (
    echo.
    echo [SUCCESS] chzzk-dock.exe build completed with icon!
) else (
    echo.
    echo [ERROR] Build failed.
)
echo.
pause
