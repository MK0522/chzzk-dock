@echo off
chcp 65001 >nul
echo ========================================================
echo  CHZZK OBS Dock Server v0.1.1 빌드 스크립트
echo ========================================================
echo.

echo [1/2] Python 확인 및 필수 패키지 설치 중...
set "PYTHON_EXE="
where py >nul 2>nul
if not errorlevel 1 set "PYTHON_EXE=py -3"
if not defined PYTHON_EXE (
  where python >nul 2>nul
  if not errorlevel 1 set "PYTHON_EXE=python"
)
if not defined PYTHON_EXE if exist "%LocalAppData%\Python\pythoncore-3.14-64\python.exe" set "PYTHON_EXE=%LocalAppData%\Python\pythoncore-3.14-64\python.exe"
if not defined PYTHON_EXE (
  echo [오류] Python 3를 찾지 못했습니다. Python 설치 시 Add Python to PATH를 선택하세요.
  pause
  exit /b 1
)
%PYTHON_EXE% -m pip install -r requirements.txt

echo.
echo [2/2] PyInstaller 실행 파일(server.exe) 생성 중...
%PYTHON_EXE% -m PyInstaller --noconfirm --clean --onefile --noconsole --icon=icon.ico --add-data "icon.png;." --add-data "chzzk-obs-dock.html;." server.py

echo.
echo ========================================================
echo  🎉 빌드 완료!
echo  생성된 파일 경로: dist\server.exe
echo ========================================================
pause
