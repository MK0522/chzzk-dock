; ==============================================================================
;  CHZZK OBS Dock - Inno Setup Installer Script
; ==============================================================================

#define MyAppName "CHZZK OBS Dock"
#define MyAppVersion "0.6.3-rc2"
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

; 관리자 권한 없이 사용자 로컬 폴더에 설치 (최신 윈도우 앱 표준, 모든 사용자용 선택 팝업 생략)
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=commandline
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes

; 설치 결과 파일명
OutputDir=.
OutputBaseFilename=chzzk-dock-setup
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
; {app} 설치 폴더 내 임시 파일 및 구버전 바로가기 정리 (상세 잔재 및 %TEMP% 청소는 Go 백엔드 CleanupLegacyArtifactsIfNeeded에서 수행)
Type: files; Name: "{app}\*.html"
Type: files; Name: "{app}\*.png"
Type: files; Name: "{app}\*.tmp"
Type: files; Name: "{app}\*.log"
Type: files; Name: "{autoprograms}\chzzk-dock.lnk"
Type: files; Name: "{commonprograms}\{#MyAppName}.lnk"
Type: files; Name: "{commonprograms}\chzzk-dock.lnk"
Type: files; Name: "{commonprograms}\{#MyAppName}\*.lnk"
Type: dirifempty; Name: "{commonprograms}\{#MyAppName}"

[Files]
Source: "chzzk-dock.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "icon.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "scripts\chzzk_dock_launcher.lua"; DestDir: "{app}\scripts"; Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startmenuicon
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startmenuicon
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"; Tasks: startmenuicon
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall

[Code]
// 치지직 독 프로세스 또는 창 활성화 여부 확인
function IsChzzkDockRunning(): Boolean;
begin
  Result := CheckForMutexes('Local\ChzzkDock') or
            (FindWindowByClassName('ChzzkPureTrayClass') <> 0) or
            (FindWindowByClassName('ChzzkDockWindowClass') <> 0);
end;

function PostMessage(hWnd: HWND; Msg: Cardinal; wParam: LongInt; lParam: LongInt): Boolean;
  external 'PostMessageW@user32.dll stdcall';

// 정상 종료 신호(WM_CLOSE) 전송
procedure SendGracefulClose();
var
  Hwnd: HWND;
begin
  Hwnd := FindWindowByClassName('ChzzkDockWindowClass');
  if Hwnd <> 0 then
    PostMessage(Hwnd, 16, 0, 0);

  Hwnd := FindWindowByClassName('ChzzkPureTrayClass');
  if Hwnd <> 0 then
    PostMessage(Hwnd, 16, 0, 0);
end;

// 2단계 안전 종료: WM_CLOSE 정상 종료 대기 후 미종료 시에만 강제 종료
procedure EnsureDockTerminated();
var
  I, ResultCode: Integer;
begin
  if not IsChzzkDockRunning() then
    Exit;

  SendGracefulClose();
  for I := 1 to 10 do
  begin
    Sleep(200);
    if not IsChzzkDockRunning() then
      Exit;
  end;

  Exec('taskkill.exe', '/F /IM chzzk-dock.exe', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  Sleep(300);
end;

// 수동 설치 및 인앱 무인 업데이트(/SILENT) 모두에서 파일 교체 직전 실행
function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  Result := '';
  if IsChzzkDockRunning() then
    EnsureDockTerminated();
end;

// 위저드 페이지 이동 시 이벤트 ('다음' 또는 '설치' 버튼 클릭 시점)
function NextButtonClick(CurPageID: Integer): Boolean;
var
  PromptResult: Integer;
begin
  Result := True;

  // 자동 인앱 업데이트(/SILENT) 시에는 UI 팝업 생략 (PrepareToInstall에서 자동 처리)
  if WizardSilent then
    Exit;

  // [1차 안내]: 시작 페이지(wpWelcome)에서 '다음' 클릭 시 실행 중이면 1차 권장 경고
  if CurPageID = wpWelcome then
  begin
    if IsChzzkDockRunning() then
    begin
      PromptResult := MsgBox(
        '치지직 독(CHZZK OBS Dock)이 현재 실행 중입니다.' + #13#10#13#10 +
        '원활한 설치 및 파일 교체를 위해 시스템 트레이 아이콘을 우클릭하여 [종료] 후 진행하는 것을 권장합니다.' + #13#10#13#10 +
        '무시하고 계속 설치를 진행하시려면 [확인]을, 설치를 취소하시려면 [취소]를 눌러주세요.',
        mbInformation, MB_OKCANCEL
      );

      if PromptResult = IDOK then
      begin
        Result := True;
      end
      else
      begin
        WizardForm.Close;
        Result := False;
        Exit;
      end;
    end;
  end;

  // [2차 안내]: 설치 준비 완료(wpReady)에서 '설치' 클릭 시 아직도 안 꺼져 있으면 종료 확인
  if CurPageID = wpReady then
  begin
    if IsChzzkDockRunning() then
    begin
      PromptResult := MsgBox(
        '치지직 독이 아직 종료되지 않아 파일 교체를 진행할 수 없습니다.' + #13#10#13#10 +
        '실행 중인 프로그램을 종료하고 설치를 계속 진행하시겠습니까?',
        mbConfirmation, MB_YESNO
      );

      if PromptResult = IDYES then
      begin
        EnsureDockTerminated();
        Result := True;
      end
      else
      begin
        Result := False;
        Exit;
      end;
    end;
  end;
end;

