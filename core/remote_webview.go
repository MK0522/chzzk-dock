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
	procGetClientRect            = user32.NewProc("GetClientRect")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procKeybdEvent               = user32.NewProc("keybd_event")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procReleaseCapture           = user32.NewProc("ReleaseCapture")
	procIsZoomed                 = user32.NewProc("IsZoomed")
	procAdjustWindowRectEx       = user32.NewProc("AdjustWindowRectEx")
	procGetSystemMenu            = user32.NewProc("GetSystemMenu")
	procCheckMenuItem            = user32.NewProc("CheckMenuItem")
	procSetWindowTextW           = user32.NewProc("SetWindowTextW")

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

	WM_CLOSE_VAL        = 0x0010
	WM_SYSCOMMAND_VAL   = 0x0112
	WM_EXITSIZEMOVE_VAL = 0x0232

	// Window Styles for Standard Native Dark Window
	WS_OVERLAPPED_VAL   = 0x00000000
	WS_CAPTION_VAL      = 0x00C00000
	WS_SYSMENU_VAL      = 0x00080000
	WS_THICKFRAME_VAL   = 0x00040000
	WS_MINIMIZEBOX_VAL  = 0x00020000
	WS_MAXIMIZEBOX_VAL  = 0x00010000
	WS_CLIPCHILDREN_VAL = 0x02000000
	WS_CLIPSIBLINGS_VAL = 0x04000000

	WS_OVERLAPPEDWINDOW_VAL = (WS_OVERLAPPED_VAL | WS_CAPTION_VAL | WS_SYSMENU_VAL | WS_THICKFRAME_VAL | WS_MINIMIZEBOX_VAL | WS_MAXIMIZEBOX_VAL | WS_CLIPCHILDREN_VAL | WS_CLIPSIBLINGS_VAL)

	WS_EX_APPWINDOW_VAL = 0x00040000
	WS_EX_TOPMOST_VAL   = 0x00000008

	IDM_REMOTE_TOPMOST = 0x1001
	MF_BYCOMMAND_VAL   = 0x00000000
	MF_UNCHECKED_VAL   = 0x00000000
	MF_CHECKED_VAL     = 0x00000008
)

type RECT_WIN struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type MARGINS struct {
	CxLeftWidth    int32
	CxRightWidth   int32
	CyTopHeight    int32
	CyBottomHeight int32
}

var (
	activeRemoteChromium *edge.Chromium
	isRemoteTopmost      = false
	remoteHwnd           uintptr
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
		Height:  880,
		Topmost: false,
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

	if state.Width < 400 || state.Height < 500 {
		state.Width = 720
		state.Height = 880
	}
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

	// Windows 11 둥근 모서리 (Rounded Corner) 적용
	cornerPref := int32(DWMWCP_ROUND)
	procDwmSetWindowAttribute.Call(hwnd, DWMWA_WINDOW_CORNER_PREFERENCE, uintptr(unsafe.Pointer(&cornerPref)), 4)
}

func updateWindowTitle(hwnd uintptr, topmost bool) {
	title := "치지직 리모컨"
	if topmost {
		title = "📌 치지직 리모컨"
	}
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(titlePtr)))
}

func updateSystemMenuTopmost(hwnd uintptr, topmost bool) {
	hSysMenu, _, _ := procGetSystemMenu.Call(hwnd, 0)
	if hSysMenu != 0 {
		flag := uintptr(MF_BYCOMMAND_VAL | MF_UNCHECKED_VAL)
		if topmost {
			flag = uintptr(MF_BYCOMMAND_VAL | MF_CHECKED_VAL)
		}
		procCheckMenuItem.Call(hSysMenu, IDM_REMOTE_TOPMOST, flag)
	}
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
	updateSystemMenuTopmost(hwnd, topmost)
	updateWindowTitle(hwnd, topmost)
}

func toggleRemoteTopmost(hwnd uintptr) {
	isRemoteTopmost = !isRemoteTopmost
	setWindowTopmost(hwnd, isRemoteTopmost)
	st := captureCurrentWindowState(hwnd)
	st.Topmost = isRemoteTopmost
	saveRemoteWindowState(st)
	if activeRemoteChromium != nil {
		activeRemoteChromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isRemoteTopmost))
	}
}

func captureCurrentWindowState(hwnd uintptr) RemoteWindowState {
	var winRect struct {
		Left, Top, Right, Bottom int32
	}
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&winRect)))

	var clientRect struct {
		Left, Top, Right, Bottom int32
	}
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&clientRect)))

	w := int(clientRect.Right - clientRect.Left)
	h := int(clientRect.Bottom - clientRect.Top)
	x := int(winRect.Left)
	y := int(winRect.Top)

	if w < 400 || h < 500 {
		w = 720
		h = 880
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
		}
		return 0

	case WM_EXITSIZEMOVE_VAL:
		// 사용자가 창 크기 조절 또는 이동을 마쳤을 때 즉시 상태 저장
		state := captureCurrentWindowState(uintptr(hwnd))
		saveRemoteWindowState(state)
		return 0

	case WM_SYSCOMMAND_VAL:
		if (wParam & 0xFFF0) == IDM_REMOTE_TOPMOST {
			toggleRemoteTopmost(uintptr(hwnd))
			return 0
		}

	case WM_CLOSE_VAL, WM_DESTROY_WV:
		state := captureCurrentWindowState(uintptr(hwnd))
		saveRemoteWindowState(state)
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

const remoteOverlayScript = `
(function() {
  if (window.__chzzkRemoteOverlayInjected) return;
  window.__chzzkRemoteOverlayInjected = true;

  function initOverlay() {
    if (document.getElementById('chzzk-floating-pin-btn')) return;

    var btn = document.createElement('button');
    btn.id = 'chzzk-floating-pin-btn';
    btn.title = '항상 위에 고정 (드래그하여 위치 이동)';

    // 이전 저장 위치 복원 (localStorage)
    var savedPos = null;
    try {
      var raw = localStorage.getItem('chzzk_floating_pin_pos');
      if (raw) savedPos = JSON.parse(raw);
    } catch(e) {}

    var initialTop = (savedPos && typeof savedPos.top === 'number') ? savedPos.top + 'px' : '10px';
    var initialLeft = (savedPos && typeof savedPos.left === 'number') ? savedPos.left + 'px' : '';
    var initialRight = (!savedPos || typeof savedPos.left !== 'number') ? '14px' : '';

    btn.style.cssText = [
      'position: fixed !important',
      'top: ' + initialTop + ' !important',
      (initialLeft ? 'left: ' + initialLeft + ' !important' : 'right: ' + initialRight + ' !important'),
      'width: 28px !important',
      'height: 28px !important',
      'border-radius: 7px !important',
      'border: 1.5px solid #334155 !important',
      'background: #161F2E !important',
      'color: #94A3B8 !important',
      'cursor: grab !important',
      'display: flex !important',
      'align-items: center !important',
      'justify-content: center !important',
      'font-size: 13px !important',
      'padding: 0 !important',
      'margin: 0 !important',
      'z-index: 9999999 !important',
      'transition: background 0.15s, border-color 0.15s, color 0.15s, box-shadow 0.15s !important',
      'box-shadow: 0 4px 10px rgba(0, 0, 0, 0.35) !important',
      'user-select: none !important',
      '-webkit-user-select: none !important',
      'touch-action: none !important'
    ].join(';');

    btn.innerHTML = '📌';

    var isDragging = false;
    var hasMoved = false;
    var startX = 0, startY = 0;
    var origLeft = 0, origTop = 0;

    btn.onmouseenter = function() {
      if (!btn.dataset.active && !isDragging) {
        btn.style.borderColor = '#475569';
        btn.style.background = '#1E293B';
        btn.style.color = '#F1F5F9';
      }
    };
    btn.onmouseleave = function() {
      if (!btn.dataset.active && !isDragging) {
        btn.style.borderColor = '#334155';
        btn.style.background = '#161F2E';
        btn.style.color = '#94A3B8';
      }
    };

    // --- 드래그 이동 핸들러 ---
    btn.addEventListener('mousedown', function(e) {
      if (e.button !== 0) return;
      isDragging = true;
      hasMoved = false;
      startX = e.clientX;
      startY = e.clientY;

      var rect = btn.getBoundingClientRect();
      origLeft = rect.left;
      origTop = rect.top;

      btn.style.cursor = 'grabbing';
      btn.style.transition = 'none';
      e.preventDefault();
      e.stopPropagation();
    });

    document.addEventListener('mousemove', function(e) {
      if (!isDragging) return;
      var dx = e.clientX - startX;
      var dy = e.clientY - startY;

      if (Math.abs(dx) > 3 || Math.abs(dy) > 3) {
        hasMoved = true;
      }

      var newLeft = origLeft + dx;
      var newTop = origTop + dy;

      var maxLeft = window.innerWidth - btn.offsetWidth;
      var maxTop = window.innerHeight - btn.offsetHeight;
      newLeft = Math.max(0, Math.min(maxLeft, newLeft));
      newTop = Math.max(0, Math.min(maxTop, newTop));

      btn.style.left = newLeft + 'px';
      btn.style.top = newTop + 'px';
      btn.style.right = 'auto';
    });

    document.addEventListener('mouseup', function(e) {
      if (!isDragging) return;
      isDragging = false;
      btn.style.cursor = 'grab';
      btn.style.transition = 'background 0.15s, border-color 0.15s, color 0.15s, box-shadow 0.15s';

      if (hasMoved) {
        try {
          var rect = btn.getBoundingClientRect();
          localStorage.setItem('chzzk_floating_pin_pos', JSON.stringify({
            left: rect.left,
            top: rect.top
          }));
        } catch(err) {}
      }
    });

    btn.addEventListener('click', function(e) {
      e.stopPropagation();
      e.preventDefault();
      if (hasMoved) {
        hasMoved = false;
        return;
      }
      if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage('toggle-topmost');
      }
    });

    document.documentElement.appendChild(btn);

    // Topmost UI Synchronizer
    window.__updateTopmostUI = function(isTopmost) {
      var b = document.getElementById('chzzk-floating-pin-btn');
      if (!b) return;
      if (isTopmost) {
        b.dataset.active = 'true';
        b.style.background = '#00FFA3 !important';
        b.style.borderColor = '#00C77F !important';
        b.style.color = '#000000 !important';
        b.style.boxShadow = '0 0 14px rgba(0, 255, 163, 0.7), 0 3px 8px rgba(0, 0, 0, 0.3) !important';
        b.title = '항상 위 고정 활성화됨 (클릭 시 해제, 드래그 이동 가능)';
      } else {
        delete b.dataset.active;
        b.style.background = '#161F2E !important';
        b.style.borderColor = '#334155 !important';
        b.style.color = '#94A3B8 !important';
        b.style.boxShadow = '0 4px 10px rgba(0, 0, 0, 0.35) !important';
        b.title = '항상 위에 고정 (드래그하여 위치 이동)';
      }
    };

    if (window.chrome && window.chrome.webview) {
      window.chrome.webview.postMessage('get-topmost-state');
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initOverlay);
  } else {
    initOverlay();
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

	dwStyle := uintptr(WS_OVERLAPPEDWINDOW_VAL)
	var exStyle uintptr = WS_EX_APPWINDOW_VAL
	if isRemoteTopmost {
		exStyle |= WS_EX_TOPMOST_VAL
	}

	// 클라이언트 영역(720x880)을 온전히 확보하기 위한 창 외곽 크기 계산
	rect := RECT_WIN{
		Left:   0,
		Top:    0,
		Right:  int32(state.Width),
		Bottom: int32(state.Height),
	}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rect)), dwStyle, 0, exStyle)
	winW := int(rect.Right - rect.Left)
	winH := int(rect.Bottom - rect.Top)

	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		dwStyle,
		uintptr(state.X), uintptr(state.Y), uintptr(winW), uintptr(winH),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		LogError("[Remote Webview Error] 리모컨 윈도우 생성 실패")
		return
	}
	remoteHwnd = hwnd

	// 타이틀바 및 작업표시줄 아이콘 설정
	if hIcon != 0 {
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), uintptr(hIcon))
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), uintptr(hIcon))
	}

	// 시스템 메뉴(창 우클릭 / 좌상단 아이콘 메뉴)에 '📌 항상 위에 고정' 등록
	hSysMenu, _, _ := procGetSystemMenu.Call(hwnd, 0)
	if hSysMenu != 0 {
		procAppendMenuW.Call(hSysMenu, MF_SEPARATOR, 0, 0)
		menuText, _ := syscall.UTF16PtrFromString("📌 항상 위에 고정")
		procAppendMenuW.Call(hSysMenu, MF_STRING, IDM_REMOTE_TOPMOST, uintptr(unsafe.Pointer(menuText)))
	}

	// Windows 11 순정 다크 타이틀바 테마 적용
	applyDarkTheme(hwnd)

	// 저장된 Topmost 상태 및 창 타이틀 초기 적용
	setWindowTopmost(hwnd, isRemoteTopmost)

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

	// 상단 우측 플로팅 핀 버튼 및 Ctrl+T 단축키 주입 (웹 본문 DOM/CSS 왜곡 0%)
	chromium.Init(remoteOverlayScript)

	// WebMessage 이벤트 핸들러 (항상 위 토글 및 상태 동기화)
	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		switch message {
		case "toggle-topmost":
			toggleRemoteTopmost(hwnd)

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
