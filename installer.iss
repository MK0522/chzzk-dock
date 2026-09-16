; ==============================================================================
;  CHZZK OBS Dock - Inno Setup Installer Script
; ==============================================================================

#define MyAppName "CHZZK OBS Dock"
#define MyAppVersion "0.5.8"
#define MyAppPublisher "CHZZK OBS Dock"
#define MyAppURL "https://github.com/Dingteus/chzzk-dock"
#define MyAppExeName "chzzk-dock.exe"

[Setup]
; 고유 AppId
AppId={{C8B3E4D1-0A8F-4D3B-9A2E-4F8A3B1C5D7E}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} v{#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}

; 관리자 권한 없이도 사용자 로컬 폴더에 설치 가능 (최신 윈도우 앱 표준)
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes

; 설치 결과 파일명
OutputDir=.
OutputBaseFilename=chzzk-dock-windows-amd64-installer
SetupIconFile=icon.ico
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern

; 설치 중 실행 중인 chzzk-dock.exe 자동 감지 및 종료 지원
CloseApplications=yes
CloseApplicationsFilter=chzzk-dock.exe

[Languages]
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"
Name: "startmenuicon"; Description: "시작 메뉴에 바로 가기 만들기(&M)"; GroupDescription: "{cm:AdditionalIcons}"

[InstallDelete]
; {app} 설치 폴더 내 구버전 잔여 파일 전면 정리 (새 버전에 필요한 파일 외 찌꺼기 원천 차단)
Type: files; Name: "{app}\*.html"
Type: files; Name: "{app}\*.ico"
Type: files; Name: "{app}\*.png"
Type: files; Name: "{app}\*.tmp"
Type: files; Name: "{app}\*.log"
; 구버전 WebView2 디스크 웹 캐시 강제 청소
Type: filesandordirs; Name: "{localappdata}\ChzzkObsDock\dock_profile\EBWebView\Default\Cache"
Type: filesandordirs; Name: "{localappdata}\ChzzkObsDock\dock_profile\EBWebView\Default\Code Cache"

[Files]
Source: "chzzk-dock.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "scripts\chzzk_dock_launcher.lua"; DestDir: "{app}\scripts"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startmenuicon
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startmenuicon
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"; Tasks: startmenuicon
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent



