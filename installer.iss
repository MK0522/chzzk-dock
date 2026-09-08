; ==============================================================================
;  CHZZK OBS Dock - Inno Setup Installer Script
; ==============================================================================

#define MyAppName "CHZZK OBS Dock"
#define MyAppVersion "0.5.0"
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

[Files]
Source: "chzzk-dock.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "scripts\chzzk_dock_launcher.lua"; DestDir: "{app}\scripts"; Flags: ignoreversion
Source: "chzzk-obs-dock.html"; DestDir: "{app}"; Flags: ignoreversion
Source: "icon.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\icon.ico"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\icon.ico"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent
