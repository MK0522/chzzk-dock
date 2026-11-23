@echo off
setlocal enabledelayedexpansion

set "PATH=C:\Program Files\Go\bin;%PATH%"

echo ======================================================
echo   CHZZK Dock - Fast Binary and Installer Build
echo ======================================================
echo.

REM 1. Dependencies tidy
echo [*] Dependencies tidy...
call go mod tidy
if errorlevel 1 goto :fail

REM 1.5 Build static Tailwind CSS when Tailwind CLI or npx is available
where tailwindcss.exe >nul 2>&1
if not errorlevel 1 (
    echo [*] Compiling static Tailwind CSS via tailwindcss.exe...
    call tailwindcss.exe -i ui\shared\tailwind.input.css -o ui\shared\tailwind.css --minify
    if errorlevel 1 goto :fail
) else (
    where npx.cmd >nul 2>&1
    if not errorlevel 1 (
        echo [*] Compiling static Tailwind CSS via npx tailwindcss...
        call npx.cmd --yes tailwindcss@3.4.17 -i ui\shared\tailwind.input.css -o ui\shared\tailwind.css --minify
        if errorlevel 1 echo [WARNING] Tailwind CSS rebuild skipped ^(using committed ui\shared\tailwind.css^).
    ) else (
        echo [*] Using committed static ui\shared\tailwind.css
    )
)

REM 2. Generate Windows PE Resource
echo [*] Generating Windows PE Resource (Icon and Manifest)...
del /f /q resource_windows_amd64.syso rsrc_*.syso >nul 2>&1
call go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -64 -o resource_windows_amd64.syso
if errorlevel 1 goto :fail

REM 3. Fast test executable
echo [*] Building chzzk-dock.exe for fast testing...
call go build -trimpath -ldflags="-H windowsgui" -o chzzk-dock.exe .
if errorlevel 1 goto :fail

echo [SUCCESS] chzzk-dock.exe build completed!
echo.

REM 4. Detect Inno Setup and build installer
set "ISCC="
if exist "%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe" set "ISCC=%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe"
if not defined ISCC if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
if not defined ISCC if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" set "ISCC=%ProgramFiles%\Inno Setup 6\ISCC.exe"
if not defined ISCC (
    where ISCC.exe >nul 2>&1
    if not errorlevel 1 (
        for /f "delims=" %%I in ('where ISCC.exe') do set "ISCC=%%I"
    )
)

if not defined ISCC goto :no_iscc

echo [*] Inno Setup compiler detected: "!ISCC!"
echo [*] Generating Windows Installer chzzk-dock-setup.exe...
"!ISCC!" installer.iss
if errorlevel 1 (
    echo [WARNING] Installer compilation failed.
) else (
    echo [SUCCESS] chzzk-dock-setup.exe generated!
)
goto :summary

:no_iscc
echo [NOTICE] Inno Setup compiler ISCC.exe not found.
echo          Skipping installer generation. chzzk-dock.exe is ready.

:summary
echo.
echo ======================================================
echo   Build Artifacts Summary
echo ======================================================
if exist chzzk-dock.exe (
    echo   [Executable] chzzk-dock.exe : Ready for testing
)
if exist chzzk-dock-setup.exe (
    echo   [Installer]  chzzk-dock-setup.exe : Ready for release
)
echo ======================================================
echo.
pause
exit /b 0

:fail
echo.
echo [FATAL] Build process aborted due to errors.
pause
exit /b 1
