package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

// ==============================================================================
// [CHZZK OBS DOCK - Remote Control Webview Window]
// - 치지직 스튜디오 공식 리모컨 (studio.chzzk.naver.com/{channelId}/remotecontrol)
// - Win32 DWM 다크 테마 일체화 (DwmSetWindowAttribute)
// - 상단 커스텀 미니 툴바 (📌 항상 위 토글, 🔄 새로고침)
// - 마지막 창 위치 / 크기 / Always-on-top 영속적 기억
// - Windows Credential Manager 쿠키 자동 주입 (무로그인 0초 로드)
// - 단일 인스턴스 Mutex 및 중복 실행 시 기존 창 포커스
// ==============================================================================

type RemoteWindowState struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Width   int  `json:"width"`
	Height  int  `json:"height"`
	Topmost bool `json:"topmost"`
}

var (
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procKeybdEvent               = user32.NewProc("keybd_event")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procReleaseCapture           = user32.NewProc("ReleaseCapture")
	procIsZoomed                 = user32.NewProc("IsZoomed")

	dwmapiDLL                        = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute        = dwmapiDLL.NewProc("DwmSetWindowAttribute")
	procDwmExtendFrameIntoClientArea = dwmapiDLL.NewProc("DwmExtendFrameIntoClientArea")

	gdi32DLL             = syscall.NewLazyDLL("gdi32.dll")
	procCreateSolidBrush = gdi32DLL.NewProc("CreateSolidBrush")
)

const (
	HWND_TOPMOST_VAL   = ^uintptr(0) // -1
	HWND_NOTOPMOST_VAL = ^uintptr(1) // -2
	SWP_NOSIZE_VAL     = 0x0001
	SWP_NOMOVE_VAL     = 0x0002

	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD = 19
	DWMWA_USE_IMMERSIVE_DARK_MODE     = 20
	DWMWA_WINDOW_CORNER_PREFERENCE    = 33
	DWMWA_CAPTION_COLOR               = 35

	DWMWCP_ROUND = 2

	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	WM_CLOSE_VAL = 0x0010

	// Window Styles for Frameless Resizable Window
	WS_POPUP_VAL        = 0x80000000
	WS_THICKFRAME_VAL   = 0x00040000
	WS_MINIMIZEBOX_VAL  = 0x00020000
	WS_MAXIMIZEBOX_VAL  = 0x00010000
	WS_CLIPCHILDREN_VAL = 0x02000000
	WS_CLIPSIBLINGS_VAL = 0x04000000

	WS_EX_APPWINDOW_VAL = 0x00040000
	WS_EX_TOPMOST_VAL   = 0x00000008
)

type MARGINS struct {
	CxLeftWidth    int32
	CxRightWidth   int32
	CyTopHeight    int32
	CyBottomHeight int32
}

var (
	activeRemoteChromium *edge.Chromium
	isRemoteTopmost      = true
)

func getRemoteWindowStatePath() string {
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	return filepath.Join(appData, "ChzzkObsDock", "remote_window.json")
}

func loadRemoteWindowState() RemoteWindowState {
	def := RemoteWindowState{
		X:       0,
		Y:       0,
		Width:   720,
		Height:  820,
		Topmost: true,
	}

	path := getRemoteWindowStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}

	var state RemoteWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return def
	}

	if state.Width < 500 || state.Height < 600 {
		state.Width = 720
		state.Height = 820
	}
	state.Topmost = true
	return state
}

func saveRemoteWindowState(state RemoteWindowState) {
	path := getRemoteWindowStatePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func applyDarkTheme(hwnd uintptr) {
	darkMode := int32(1)
	// Windows 10 20H1+ & Windows 11
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, uintptr(unsafe.Pointer(&darkMode)), 4)
	// Windows 10 older builds fallback
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE_OLD, uintptr(unsafe.Pointer(&darkMode)), 4)
	// Caption color: #0B0E11 (COLORREF: 0x00110E0B)
	captionColor := uint32(0x00110E0B)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_CAPTION_COLOR, uintptr(unsafe.Pointer(&captionColor)), 4)

	// DWM 시스템 그림자 (Drop Shadow) 활성화
	margins := MARGINS{1, 1, 1, 1}
	procDwmExtendFrameIntoClientArea.Call(hwnd, uintptr(unsafe.Pointer(&margins)))

	// Windows 11 둥근 모서리 (Rounded Corner) 적용
	cornerPref := int32(DWMWCP_ROUND)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE, uintptr(unsafe.Pointer(&cornerPref)), 4)
}

// ForceForegroundWindow: Windows의 포그라운드 락을 우회하여 창을 최상단으로 강제 활성화
func ForceForegroundWindow(hwnd uintptr, topmost bool) {
	if hwnd == 0 {
		return
	}

	// 1. 최소화되어 있다면 복원 (SW_RESTORE = 9), 아니면 활성화 표시 (SW_SHOW = 5)
	procShowWindow.Call(hwnd, 9)

	// 2. 현재 포그라운드 창 및 스레드 ID 확인
	fgHwnd, _, _ := procGetForegroundWindow.Call()
	curThreadId, _, _ := procGetCurrentThreadId.Call()
	var fgThreadId uintptr
	if fgHwnd != 0 {
		fgThreadId, _, _ = procGetWindowThreadProcessId.Call(fgHwnd, 0)
	}

	// 3. 포그라운드 락 해제를 위한 AttachThreadInput 연결
	if fgThreadId != 0 && fgThreadId != curThreadId {
		procAttachThreadInput.Call(fgThreadId, curThreadId, 1) // TRUE
	}

	// 4. Alt 키 탭 (Windows 전역 포그라운드 전환 잠금 해제 트리거)
	procKeybdEvent.Call(0x12, 0, 0, 0)      // VK_MENU down
	procKeybdEvent.Call(0x12, 0, 0x0002, 0) // VK_MENU up

	// 5. Z-order 조정 및 최상단 설정
	targetZ := HWND_NOTOPMOST_VAL
	if topmost {
		targetZ = HWND_TOPMOST_VAL
	}
	procSetWindowPos.Call(hwnd, targetZ, 0, 0, 0, 0, SWP_NOMOVE_VAL|SWP_NOSIZE_VAL|0x0040)
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)

	// 6. 스레드 입력 분리
	if fgThreadId != 0 && fgThreadId != curThreadId {
		procAttachThreadInput.Call(fgThreadId, curThreadId, 0) // FALSE
	}
}

func setWindowTopmost(hwnd uintptr, topmost bool) {
	ForceForegroundWindow(hwnd, topmost)
}

func captureCurrentWindowState(hwnd uintptr) RemoteWindowState {
	var rect struct {
		Left, Top, Right, Bottom int32
	}
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))

	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	x := int(rect.Left)
	y := int(rect.Top)

	if w < 320 || h < 400 {
		w = 420
		h = 720
	}

	return RemoteWindowState{
		X:       x,
		Y:       y,
		Width:   w,
		Height:  h,
		Topmost: isRemoteTopmost,
	}
}

func remoteWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_MOVE_WV:
		if activeRemoteChromium != nil {
			_ = activeRemoteChromium.NotifyParentWindowPositionChanged()
		}
		return 0

	case WM_SIZE_WV:
		if activeRemoteChromium != nil {
			activeRemoteChromium.Resize()
			isZoomed, _, _ := procIsZoomed.Call(uintptr(hwnd))
			activeRemoteChromium.Eval(fmt.Sprintf("if (window.__updateMaximizeUI) window.__updateMaximizeUI(%t);", isZoomed != 0))
		}
		return 0

	case WM_CLOSE_VAL, WM_DESTROY_WV:
		state := captureCurrentWindowState(uintptr(hwnd))
		saveRemoteWindowState(state)
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

const remoteToolbarScript = `
(function() {
  if (window.__chzzkRemoteBarInjected) return;
  window.__chzzkRemoteBarInjected = true;

  function initBar() {
    if (document.getElementById('chzzk-remote-toolbar')) return;

    // 1. All-in-One Frameless Titlebar
    var bar = document.createElement('div');
    bar.id = 'chzzk-remote-toolbar';
    bar.style.cssText = 'position:fixed; top:0; left:0; right:0; height:32px; background:#0B0E11; border-bottom:1px solid #1E2738; z-index:9999999; display:flex; align-items:center; justify-content:space-between; padding:0 4px 0 8px; box-sizing:border-box; font-family:Pretendard,-apple-system,BlinkMacSystemFont,sans-serif; user-select:none; -webkit-user-select:none;';

    bar.innerHTML = 
      '<div style="display:flex; align-items:center; gap:6px; height:100%; flex-shrink:0;">' +
        // Pin Button: Far Left, Icon Only, No Text
        '<button id="chzzk-btn-pin" title="항상 위 고정 토글" style="background:#161F2E; border:1px solid #243044; color:#94A3B8; width:24px; height:24px; border-radius:5px; cursor:pointer; display:flex; align-items:center; justify-content:center; padding:0; transition:all 0.15s; font-size:11px;">' +
          '<span>📌</span>' +
        '</button>' +
        // App Title
        '<div id="chzzk-title-box" style="display:flex; align-items:center; gap:5px; cursor:default; margin-left:2px;">' +
          '<span style="font-size:13px; line-height:1; display:flex; align-items:center;">🎮</span>' +
          '<span style="font-size:11.5px; font-weight:700; color:#F1F5F9; letter-spacing:-0.2px;">치지직 리모컨</span>' +
        '</div>' +
        // Reload Button
        '<button id="chzzk-btn-reload" title="새로고침" style="background:#161F2E; border:1px solid #243044; color:#94A3B8; width:24px; height:24px; border-radius:5px; cursor:pointer; display:flex; align-items:center; justify-content:center; padding:0; transition:all 0.15s; font-size:11px; margin-left:4px;">' +
          '🔄' +
        '</button>' +
      '</div>' +
      // Drag Region Spacer
      '<div id="chzzk-drag-spacer" style="flex:1; height:100%; cursor:default;"></div>' +
      // Window Controls (Min, Max, Close)
      '<div style="display:flex; align-items:center; height:100%; flex-shrink:0;">' +
        '<button id="chzzk-btn-min" title="최소화" style="background:transparent; border:none; color:#94A3B8; width:36px; height:32px; display:flex; align-items:center; justify-content:center; cursor:pointer; padding:0; transition:background 0.1s;">' +
          '<svg width="10" height="1" viewBox="0 0 10 1"><rect width="10" height="1" fill="currentColor"/></svg>' +
        '</button>' +
        '<button id="chzzk-btn-max" title="최대화" style="background:transparent; border:none; color:#94A3B8; width:36px; height:32px; display:flex; align-items:center; justify-content:center; cursor:pointer; padding:0; transition:background 0.1s;">' +
          '<svg id="chzzk-max-svg" width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1"><rect x="0.5" y="0.5" width="9" height="9"/></svg>' +
        '</button>' +
        '<button id="chzzk-btn-close" title="닫기" style="background:transparent; border:none; color:#94A3B8; width:36px; height:32px; display:flex; align-items:center; justify-content:center; cursor:pointer; padding:0; transition:background 0.1s;">' +
          '<svg width="10" height="10" viewBox="0 0 10 10"><path d="M1 1 L9 9 M9 1 L1 9" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/></svg>' +
        '</button>' +
      '</div>';

    document.documentElement.appendChild(bar);

    // 2. Layout CSS: Prevent top/bottom clipping caused by 32px titlebar
    var style = document.createElement('style');
    style.innerHTML = [
      // [Core Fix] Push entire page content below 32px titlebar and constrain height
      // to prevent bottom overflow (volume slider, TTS skip, alert stop buttons)
      'html { margin-top: 32px !important; height: calc(100vh - 32px) !important; overflow: hidden !important; }',
      'body { height: 100% !important; overflow: hidden !important; margin: 0 !important; }',
      // Next.js / SPA root container: fill available height with scrollable overflow
      '#__next, [id^="__next"], body > div:first-child {',
      '  height: 100% !important;',
      '  max-height: 100% !important;',
      '  overflow-y: auto !important;',
      '  overflow-x: hidden !important;',
      '}',
      // [Version-Independent Selectors] Use tag/role-based selectors instead of
      // brittle webpack CSS module hashes (_header_3o1tv, _container_169h1)
      // that break on every Chzzk Studio frontend deployment.
      // Chzzk Studio uses a fixed header that already has position:fixed/sticky,
      // so we don't need to manually shift it — the html margin-top handles it.

      // Right sidebar panel: ensure it fills available height properly
      'body > div aside, [role="complementary"], [class*="_aside_"], [class*="_sidebar_"] {',
      '  max-height: calc(100vh - 32px) !important;',
      '  overflow-y: auto !important;',
      '}',

      // Custom scrollbar styling for clean dark UI
      '::-webkit-scrollbar { width: 4px; }',
      '::-webkit-scrollbar-track { background: transparent; }',
      '::-webkit-scrollbar-thumb { background: #1E2738; border-radius: 4px; }',
      '::-webkit-scrollbar-thumb:hover { background: #2D3F5A; }',

      // Frameless Window Titlebar Buttons Hover & Active
      '#chzzk-btn-pin:hover, #chzzk-btn-reload:hover { background: #223045 !important; color: #F1F5F9 !important; border-color: #384A68 !important; }',
      '#chzzk-btn-min:hover, #chzzk-btn-max:hover { background: rgba(255, 255, 255, 0.08) !important; color: #FFFFFF !important; }',
      '#chzzk-btn-close:hover { background: #E81123 !important; color: #FFFFFF !important; }',
      '#chzzk-btn-close:active { background: #C4101F !important; color: #FFFFFF !important; }'
    ].join('\n');
    document.documentElement.appendChild(style);

    // 4. Button Event Handlers
    var pinBtn = document.getElementById('chzzk-btn-pin');
    if (pinBtn) {
      pinBtn.onclick = function(e) {
        e.stopPropagation();
        if (window.chrome && window.chrome.webview) {
          window.chrome.webview.postMessage('toggle-topmost');
        }
      };
    }

    var reloadBtn = document.getElementById('chzzk-btn-reload');
    if (reloadBtn) {
      reloadBtn.onclick = function(e) {
        e.stopPropagation();
        location.reload();
      };
    }

    var minBtn = document.getElementById('chzzk-btn-min');
    if (minBtn) {
      minBtn.onclick = function(e) {
        e.stopPropagation();
        if (window.chrome && window.chrome.webview) {
          window.chrome.webview.postMessage('window-minimize');
        }
      };
    }

    var maxBtn = document.getElementById('chzzk-btn-max');
    if (maxBtn) {
      maxBtn.onclick = function(e) {
        e.stopPropagation();
        if (window.chrome && window.chrome.webview) {
          window.chrome.webview.postMessage('window-maximize');
        }
      };
    }

    var closeBtn = document.getElementById('chzzk-btn-close');
    if (closeBtn) {
      closeBtn.onclick = function(e) {
        e.stopPropagation();
        if (window.chrome && window.chrome.webview) {
          window.chrome.webview.postMessage('window-close');
        }
      };
    }

    // 5. Titlebar Dragging & Double-Click Maximize
    bar.addEventListener('mousedown', function(e) {
      if (e.target.closest('button')) return;
      if (e.button === 0) { // Left-click drag
        if (window.chrome && window.chrome.webview) {
          window.chrome.webview.postMessage('window-drag');
        }
      }
    });

    bar.addEventListener('dblclick', function(e) {
      if (e.target.closest('button')) return;
      if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage('window-maximize');
      }
    });

    // 6. UI Synchronizers
    window.__updateTopmostUI = function(isTopmost) {
      var btn = document.getElementById('chzzk-btn-pin');
      if (!btn) return;
      if (isTopmost) {
        btn.style.background = 'rgba(0, 255, 163, 0.15)';
        btn.style.borderColor = '#00FFA3';
        btn.style.color = '#00FFA3';
        btn.style.boxShadow = '0 0 8px rgba(0, 255, 163, 0.35)';
        btn.title = '항상 위 고정 활성화됨 (클릭 시 해제)';
      } else {
        btn.style.background = '#161F2E';
        btn.style.borderColor = '#243044';
        btn.style.color = '#94A3B8';
        btn.style.boxShadow = 'none';
        btn.title = '항상 위 고정 토글';
      }
    };

    window.__updateMaximizeUI = function(isMax) {
      var btn = document.getElementById('chzzk-btn-max');
      if (!btn) return;
      if (isMax) {
        btn.title = '이전 크기로 복원';
        btn.innerHTML = '<svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1"><path d="M2.5 2.5V0.5H9.5V7.5H7.5"/><rect x="0.5" y="2.5" width="7" height="7"/></svg>';
      } else {
        btn.title = '최대화';
        btn.innerHTML = '<svg width="10" height="10" viewBox="0 0 10 10" fill="none" stroke="currentColor" stroke-width="1"><rect x="0.5" y="0.5" width="9" height="9"/></svg>';
      }
    };

    if (window.chrome && window.chrome.webview) {
      window.chrome.webview.postMessage('get-topmost-state');
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initBar);
  } else {
    initBar();
  }
})();
`

// RunRemoteWebview: 치지직 공식 리모컨 독립 WebView2 플로팅 윈도우 구동
func RunRemoteWebview(channelId string) {
	runtime.LockOSThread()

	// [단일 인스턴스 보장] 이미 실행 중인 경우 기존 창을 찾아 화면 앞으로 포커스
	mutexNamePtr, _ := syscall.UTF16PtrFromString(`Local\ChzzkObsDock_Remote_Mutex`)
	mutexHandle, _, _ := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	lastErr, _, _ := kernel32.NewProc("GetLastError").Call()

	className, _ := syscall.UTF16PtrFromString("ChzzkRemoteWindowClass")

	if lastErr == 183 { // ERROR_ALREADY_EXISTS
		LogInfo("[Remote Webview] 이미 리모컨 창이 실행 중입니다. 기존 창을 맨 앞으로 가져옵니다.")
		existingHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
		if existingHwnd != 0 {
			ForceForegroundWindow(existingHwnd, true)
		}
		if mutexHandle != 0 {
			kernel32.NewProc("CloseHandle").Call(mutexHandle)
		}
		os.Exit(0)
	}
	defer func() {
		if mutexHandle != 0 {
			kernel32.NewProc("CloseHandle").Call(mutexHandle)
		}
	}()

	// 저장된 창 위치 및 크기 복원
	state := loadRemoteWindowState()
	isRemoteTopmost = state.Topmost

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	// 로그인 창과 세션을 공유하여 무로그인으로 즉시 진입
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")
	_ = os.MkdirAll(profileDir, 0755)

	windowTitle, _ := syscall.UTF16PtrFromString("치지직 리모컨 - CHZZK OBS Dock")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hIcon := LoadAppIcon()

	hBrush, _, _ := procCreateSolidBrush.Call(0x00110E0B) // #0B0E11 Dark Brush
	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(remoteWndProc),
		HInstance:     syscall.Handle(hInst),
		HIcon:         hIcon,
		HCursor:       syscall.Handle(0),
		HbrBackground: syscall.Handle(hBrush),
		LpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	// 화면 중앙 좌표 계산 (저장된 좌표가 0인 경우)
	if state.X == 0 && state.Y == 0 {
		screenW, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
		screenH, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
		if screenW > 0 && screenH > 0 {
			state.X = (int(screenW) - state.Width) / 2
			state.Y = (int(screenH) - state.Height) / 2
		} else {
			state.X = 150
			state.Y = 150
		}
	}

	dwStyle := uintptr(WS_POPUP_VAL | WS_THICKFRAME_VAL | WS_MINIMIZEBOX_VAL | WS_MAXIMIZEBOX_VAL | WS_CLIPCHILDREN_VAL | WS_CLIPSIBLINGS_VAL)
	var exStyle uintptr = WS_EX_APPWINDOW_VAL
	if isRemoteTopmost {
		exStyle |= WS_EX_TOPMOST_VAL
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		dwStyle,
		uintptr(state.X), uintptr(state.Y), uintptr(state.Width), uintptr(state.Height),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		LogError("[Remote Webview Error] 리모컨 윈도우 생성 실패")
		return
	}

	// 타이틀바 및 작업표시줄 아이콘 설정
	if hIcon != 0 {
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), uintptr(hIcon))
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), uintptr(hIcon))
	}

	// DWM 다크 테마, 그림자 및 둥근 모서리 적용
	applyDarkTheme(hwnd)

	// 저장된 Topmost 상태 적용
	if isRemoteTopmost {
		setWindowTopmost(hwnd, true)
	}

	chromium := edge.NewChromium()
	chromium.DataPath = profileDir

	// [렌더링 가속 및 백그라운드 스로틀링 방지 인자]
	chromium.AdditionalBrowserArgs = []string{
		"--disable-features=CalculateNativeWinOcclusion",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	}
	activeRemoteChromium = chromium

	// 프로세스 실패 감지 콜백
	chromium.ProcessFailedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		LogError("[Remote Webview] WebView2 렌더러 프로세스 장애 발생. 다시 시도해 주세요.")
	}

	// 페이지 로딩 완료 감지 콜백
	chromium.NavigationCompletedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		src, _ := sender.GetSource()
		LogInfo("[Remote Webview] 리모컨 페이지 로드 완료: %s", src)

		// 페이지 로드 시 최신 로그인 쿠키 감지 및 자동 동기화
		if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
			_ = callGetCookies(cm, "https://chzzk.naver.com", func(listPtr uintptr, err error) {
				if err != nil || listPtr == 0 {
					return
				}
				aut, ses, found := inspectCookies(listPtr)
				if found && aut != "" && ses != "" {
					cfg := LoadConfig()
					if cfg.NidAut != aut || cfg.NidSes != ses {
						SaveConfig(map[string]interface{}{
							"nid_aut": aut,
							"nid_ses": ses,
						})
						LogInfo("[Remote Webview] 리모컨 창에서 최신 네이버 세션 쿠키를 감지하여 자동 동기화했습니다.")
					}
				}
			})
		}
	}

	if !chromium.Embed(hwnd) {
		LogError("[Remote Webview Error] WebView2 임베딩 실패 (WebView2 Runtime 확인 필요)")
		procDestroyWindow.Call(hwnd)
		return
	}

	// [초기 순백색 화면 방지] 컨트롤러 배경색을 다크 테마(#0B0E11)로 즉시 지정
	chromium.SetBackgroundColour(0x0B, 0x0E, 0x11, 255)

	// 상단 커스텀 툴바 주입 (Embed 완료 후 e.webview가 생성된 시점에 호출)
	chromium.Init(remoteToolbarScript)

	// WebMessage 이벤트 핸들러 (창 드래그, 최소화, 최대화, 닫기, 항상 위 토글)
	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		switch message {
		case "window-drag":
			procReleaseCapture.Call()
			procSendMessageW.Call(hwnd, 0x0112 /* WM_SYSCOMMAND */, 0xF012 /* SC_DRAGMOVE */, 0)

		case "window-minimize":
			procShowWindow.Call(hwnd, 6 /* SW_MINIMIZE */)

		case "window-maximize":
			isZoomed, _, _ := procIsZoomed.Call(hwnd)
			if isZoomed != 0 {
				procShowWindow.Call(hwnd, 9 /* SW_RESTORE */)
			} else {
				procShowWindow.Call(hwnd, 3 /* SW_MAXIMIZE */)
			}

		case "window-close":
			procPostMessageW.Call(hwnd, WM_CLOSE_VAL, 0, 0)

		case "toggle-topmost":
			isRemoteTopmost = !isRemoteTopmost
			setWindowTopmost(hwnd, isRemoteTopmost)
			st := captureCurrentWindowState(hwnd)
			st.Topmost = isRemoteTopmost
			saveRemoteWindowState(st)
			chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isRemoteTopmost))

		case "get-topmost-state":
			chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isRemoteTopmost))
		}
	}

	// [쿠키 사전 주입: 0초 무로그인 인증 (RFC 6265 루트 도메인 최적화)]
	cm, err := chromium.GetCookieManager()
	if err == nil && cm != nil {
		cfg := LoadConfig()
		if cfg.NidAut != "" && cfg.NidSes != "" {
			LogInfo("[Remote Webview] 저장된 세션 쿠키를 WebView2에 자동 주입합니다 (무로그인 0초 로드)")
			domains := []string{
				".naver.com",
				".chzzk.naver.com",
			}
			for _, dom := range domains {
				if cookie, err := cm.CreateCookie("NID_AUT", cfg.NidAut, dom, "/"); err == nil && cookie != nil {
					_ = cookie.PutIsSecure(true)
					_ = cookie.PutIsHttpOnly(true)
					_ = cm.AddOrUpdateCookie(cookie)
					cookie.Release()
				}
				if cookie, err := cm.CreateCookie("NID_SES", cfg.NidSes, dom, "/"); err == nil && cookie != nil {
					_ = cookie.PutIsSecure(true)
					_ = cookie.PutIsHttpOnly(true)
					_ = cm.AddOrUpdateCookie(cookie)
					cookie.Release()
				}
			}
		} else {
			LogWarn("[Remote Webview] 저장된 네이버 로그인 쿠키(NID_AUT, NID_SES)가 없어 쿠키 주입을 건너뜁니다.")
		}
	}

	// 창 표시 및 강제 포그라운드 활성화
	ForceForegroundWindow(hwnd, isRemoteTopmost)

	// 컨트롤러 가시화 및 포커스 부여
	_ = chromium.Show()
	chromium.Focus()
	chromium.Resize()

	// 공식 리모컨 페이지 URL 로드
	remoteURL := "https://studio.chzzk.naver.com"
	if channelId != "" {
		remoteURL = fmt.Sprintf("https://studio.chzzk.naver.com/%s/remotecontrol", channelId)
	}
	time.Sleep(50 * time.Millisecond)
	chromium.Navigate(remoteURL)

	// 렌더러 로드 후 최상단 z-order 재확인
	ForceForegroundWindow(hwnd, isRemoteTopmost)

	var m MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	time.Sleep(200 * time.Millisecond)
}
