package core

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

// ==============================================================================
// [CHZZK OBS DOCK - User Webview Windows Engine]
// - 통합 모듈: 메인 방송 독 창, 실시간 채팅창, 공식 스튜디오 리모컨 창
// - 순수 Win32 DWM 다크 테마 일체화 및 WebView2 바인딩
// - 창 위치 / 크기 / Always-on-top 상태 영속화 (%LOCALAPPDATA%\ChzzkObsDock\*.json)
// - 단일 인스턴스 Mutex 관리 및 중복 실행 시 기존 창 최상단 포커스/복원
// ==============================================================================

// -----------------------------------------------------------------------------
// 1. 공통 상수 및 Win32 프로시저 정의
// -----------------------------------------------------------------------------

const (
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	HWND_TOPMOST_VAL   = ^uintptr(0) // -1
	HWND_NOTOPMOST_VAL = ^uintptr(1) // -2
	SWP_NOSIZE_VAL     = 0x0001
	SWP_NOMOVE_VAL     = 0x0002

	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD = 19
	DWMWA_USE_IMMERSIVE_DARK_MODE     = 20
	WM_CLOSE_VAL                      = 0x0010
	WS_OVERLAPPEDWINDOW_VAL           = 0x00CF0000 | 0x02000000 | 0x04000000
	WS_EX_APPWINDOW_VAL               = 0x00040000
	WS_EX_TOPMOST_VAL                 = 0x00000008
)

var (
	procGetWindowRect          = user32.NewProc("GetWindowRect")
	procGetClientRect          = user32.NewProc("GetClientRect")
	procFindWindowW            = user32.NewProc("FindWindowW")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procBringWindowToTop       = user32.NewProc("BringWindowToTop")
	procAdjustWindowRectEx     = user32.NewProc("AdjustWindowRectEx")
	procGetSystemMenu          = user32.NewProc("GetSystemMenu")
	procCheckMenuItem          = user32.NewProc("CheckMenuItem")
	procSetWindowTextW         = user32.NewProc("SetWindowTextW")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")

	dwmapiDLL                 = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapiDLL.NewProc("DwmSetWindowAttribute")

	gdi32DLL             = syscall.NewLazyDLL("gdi32.dll")
	procCreateSolidBrush = gdi32DLL.NewProc("CreateSolidBrush")
)

// ForceForegroundWindow: 지정된 윈도우 창을 화면 최상단으로 강제 포커스/활성화
func ForceForegroundWindow(hwnd uintptr, isTopmost bool) {
	if hwnd == 0 {
		return
	}
	user32.NewProc("ShowWindow").Call(hwnd, 9 /* SW_RESTORE */)
	user32.NewProc("SetForegroundWindow").Call(hwnd)
	procBringWindowToTop.Call(hwnd)

	var insertAfter uintptr = HWND_NOTOPMOST_VAL
	if isTopmost {
		insertAfter = HWND_TOPMOST_VAL
	}
	user32.NewProc("SetWindowPos").Call(
		hwnd, insertAfter, 0, 0, 0, 0,
		uintptr(SWP_NOMOVE_VAL|SWP_NOSIZE_VAL|0x0040 /* SWP_SHOWWINDOW */),
	)
}

// applyDarkTheme: Windows 10/11 순정 DWM 다크 테마 적용
func applyDarkTheme(hwnd uintptr) {
	darkMode := int32(1)
	r, _, _ := procDwmSetWindowAttribute.Call(
		hwnd,
		DWMWA_USE_IMMERSIVE_DARK_MODE,
		uintptr(unsafe.Pointer(&darkMode)),
		unsafe.Sizeof(darkMode),
	)
	if r != 0 {
		procDwmSetWindowAttribute.Call(
			hwnd,
			DWMWA_USE_IMMERSIVE_DARK_MODE_OLD,
			uintptr(unsafe.Pointer(&darkMode)),
			unsafe.Sizeof(darkMode),
		)
	}
}

// -----------------------------------------------------------------------------
// 2. 메인 독 웹뷰 창 (Standalone Main Dock Window)
// -----------------------------------------------------------------------------

type DockWindowState struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type MINMAXINFO struct {
	PtReserved     POINT
	PtMaxSize      POINT
	PtMaxPosition  POINT
	PtMinTrackSize POINT
	PtMaxTrackSize POINT
}

var (
	activeDockChromium *edge.Chromium
	dockHwnd           uintptr
	dockRegisteredMsg  uint32
)

// GetShowDockMessageId: 전역 고유 등록 윈도우 메시지 ID 조회 (RegisterWindowMessageW)
func GetShowDockMessageId() uint32 {
	msgNamePtr, _ := syscall.UTF16PtrFromString("ChzzkDock_ShowUI")
	msgId, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(msgNamePtr)))
	return uint32(msgId)
}

func getDockWindowStatePath() string {
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	return filepath.Join(appData, "ChzzkObsDock", "dock_window.json")
}

func loadDockWindowState() DockWindowState {
	defaultState := DockWindowState{
		X:      0,
		Y:      0,
		Width:  480,
		Height: 850,
	}
	data, err := os.ReadFile(getDockWindowStatePath())
	if err != nil {
		return defaultState
	}
	var state DockWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return defaultState
	}
	if state.Width < 360 || state.Height < 480 {
		state.Width = 480
		state.Height = 850
	}
	return state
}

func saveDockWindowState(state DockWindowState) {
	path := getDockWindowStatePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func captureDockWindowState(hwnd uintptr) DockWindowState {
	var rect struct {
		Left, Top, Right, Bottom int32
	}
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))

	w := int(rect.Right - rect.Left)
	h := int(rect.Bottom - rect.Top)
	x := int(rect.Left)
	y := int(rect.Top)

	if w < 360 || h < 480 {
		w = 480
		h = 850
	}

	return DockWindowState{
		X:      x,
		Y:      y,
		Width:  w,
		Height: h,
	}
}

func dockWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	// 버전별 등록 윈도우 메시지 수신 시 (중복 실행 프로세스에서 창 열기 요청)
	if dockRegisteredMsg != 0 && msg == dockRegisteredMsg {
		procShowWindow.Call(uintptr(hwnd), 9 /* SW_RESTORE */)
		procUpdateWindow.Call(uintptr(hwnd))
		if activeDockChromium != nil {
			_ = activeDockChromium.Show()
			activeDockChromium.Resize()
			activeDockChromium.Focus()
		}
		ForceForegroundWindow(uintptr(hwnd), false)
		return 0
	}

	switch msg {
	case WM_MOVE_WV:
		if activeDockChromium != nil {
			_ = activeDockChromium.NotifyParentWindowPositionChanged()
		}
		return 0

	case WM_SIZE_WV:
		if activeDockChromium != nil {
			activeDockChromium.Resize()
		}
		return 0

	case 0x0024: // WM_GETMINMAXINFO: 최소 창 크기 제한
		if lParam != 0 {
			mmi := (*MINMAXINFO)(unsafe.Pointer(lParam))
			mmi.PtMinTrackSize.X = 360
			mmi.PtMinTrackSize.Y = 480
			return 0
		}

	case WM_CLOSE_VAL:
		// [핵심] 창 닫기(X 버튼 또는 화면 닫기) 시 프로세스를 종료하지 않고 숨김 처리 (트레이 & 백그라운드 유지)
		saveDockWindowState(captureDockWindowState(uintptr(hwnd)))
		procShowWindow.Call(uintptr(hwnd), 0 /* SW_HIDE */)
		return 0

	case WM_DESTROY_WV:
		saveDockWindowState(captureDockWindowState(uintptr(hwnd)))
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// ShowDockWindow: 독 윈도우를 화면에 복원 및 표시하고 맨 앞으로 가져옵니다.
func ShowDockWindow() {
	if dockHwnd == 0 {
		return
	}
	if dockRegisteredMsg != 0 {
		procPostMessageW.Call(dockHwnd, uintptr(dockRegisteredMsg), 0, 0)
		return
	}
	procShowWindow.Call(dockHwnd, 9 /* SW_RESTORE */)
	procUpdateWindow.Call(dockHwnd)
	ForceForegroundWindow(dockHwnd, false)
}

// HideDockWindow: 독 윈도우를 숨깁니다 (트레이 유지).
func HideDockWindow() {
	if dockHwnd == 0 {
		return
	}
	saveDockWindowState(captureDockWindowState(dockHwnd))
	procShowWindow.Call(dockHwnd, 0 /* SW_HIDE */)
}

// ToggleDockWindow: 독 윈도우가 표시 중이면 숨기고, 숨겨져 있으면 화면 앞으로 가져옵니다.
func ToggleDockWindow() {
	if dockHwnd == 0 {
		return
	}
	if IsDockWindowVisible() {
		HideDockWindow()
	} else {
		ShowDockWindow()
	}
}

// IsDockWindowVisible: 독 윈도우가 현재 화면에 표시(가시 상태) 중인지 확인
func IsDockWindowVisible() bool {
	if dockHwnd == 0 {
		return false
	}
	r, _, _ := procIsWindowVisible.Call(dockHwnd)
	return r != 0
}

// IsDockWindowCreated: 독 윈도우 생성 여부 확인
func IsDockWindowCreated() bool {
	return dockHwnd != 0
}

// InitDockWindow: 독 독립 윈도우 및 WebView2 생성
func InitDockWindow(port int, version string, showInitially bool) uintptr {
	dockRegisteredMsg = GetShowDockMessageId()

	classNameStr := "ChzzkDockWindowClass"
	className, _ := syscall.UTF16PtrFromString(classNameStr)
	displayVer := version
	if !strings.HasPrefix(displayVer, "v") {
		displayVer = "v" + displayVer
	}
	windowTitle, _ := syscall.UTF16PtrFromString(fmt.Sprintf("CHZZK OBS Dock (%s)", displayVer))

	state := loadDockWindowState()

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	profileDir := filepath.Join(appData, "ChzzkObsDock", "dock_profile")
	_ = os.MkdirAll(profileDir, 0755)

	hInst, _, _ := procGetModuleHandleW.Call(0)
	hIcon := LoadAppIcon()

	hBrush, _, _ := procCreateSolidBrush.Call(0x00110E0B) // #0B0E11 다크 브러시
	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(dockWndProc),
		HInstance:     syscall.Handle(hInst),
		HIcon:         hIcon,
		HCursor:       syscall.Handle(0),
		HbrBackground: syscall.Handle(hBrush),
		LpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	// 화면 이탈 방지 및 중앙 좌표 계산
	screenW, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	if state.X <= 0 || state.Y <= 0 || (screenW > 0 && state.X >= int(screenW)-100) || (screenH > 0 && state.Y >= int(screenH)-100) {
		if screenW > 0 && screenH > 0 {
			state.X = (int(screenW) - state.Width) / 2
			state.Y = (int(screenH) - state.Height) / 2
		} else {
			state.X = 150
			state.Y = 150
		}
	}

	// 표준 윈도우 스타일 (캡션, 최소화/최대화/닫기 버튼, 테두리 크기조절 가능)
	dwStyle := uintptr(0x00CF0000 | 0x02000000 | 0x04000000) // WS_OVERLAPPEDWINDOW | WS_CLIPCHILDREN | WS_CLIPSIBLINGS
	var exStyle uintptr = WS_EX_APPWINDOW_VAL

	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		dwStyle,
		uintptr(state.X), uintptr(state.Y), uintptr(state.Width), uintptr(state.Height),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		LogError("[Dock Window Error] [%s] 독 윈도우 생성 실패", ErrSysFileIoFailed)
		return 0
	}

	dockHwnd = hwnd

	// 타이틀바 및 작업표시줄 아이콘 설정
	if hIcon != 0 {
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), uintptr(hIcon))
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), uintptr(hIcon))
	}

	// DWM 다크 테마 적용
	applyDarkTheme(hwnd)

	chromium := edge.NewChromium()
	chromium.DataPath = profileDir

	dockBrowserArgs := []string{
		"--disable-features=CalculateNativeWinOcclusion",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	}
	if !GetEnableGPU() {
		dockBrowserArgs = append(dockBrowserArgs, "--disable-gpu")
	}
	chromium.AdditionalBrowserArgs = dockBrowserArgs
	activeDockChromium = chromium

	chromium.ProcessFailedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		LogError("[Dock Window] [%s] WebView2 렌더러 프로세스 장애 발생", ErrSysWebviewRuntime)
	}

	dockUrl := fmt.Sprintf("http://127.0.0.1:%d", port)
	chromium.NavigationCompletedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		src, _ := sender.GetSource()
		LogInfo("[Dock Window] 방송 독 페이지 로드 완료: %s", src)
		if activeDockChromium != nil {
			_ = activeDockChromium.Show()
			activeDockChromium.Resize()
		}
	}

	if !chromium.Embed(hwnd) {
		LogWarn("[Dock Window] [%s] WebView2 임베딩 실패 (WebView2 Runtime 미설치 가능성)", ErrSysWebviewRuntime)
		if showInitially {
			procShellExecuteW := shell32.NewProc("ShellExecuteW")
			urlPtr, _ := syscall.UTF16PtrFromString(dockUrl)
			procShellExecuteW.Call(0, 0, uintptr(unsafe.Pointer(urlPtr)), 0, 0, 1)
		}
		return hwnd
	}

	// 컨트롤러 가시성 보장 및 크기 동기화
	_ = chromium.Show()
	chromium.Resize()
	chromium.Focus()

	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		if strings.HasPrefix(message, "open-external:") {
			targetURL := strings.TrimPrefix(message, "open-external:")
			if GetExternalBrowserGuard() {
				OpenBrowser(targetURL)
			}
		}
	}

	// 외부 링크 가드 스크립트 적용
	chromium.Init(ExternalLinkGuardScript)

	// 초기 백색 화면 방지 (배경 다크 테마)
	chromium.SetBackgroundColour(0x0B, 0x0E, 0x11, 255)

	// 서버 구동 안정화를 위한 150ms 지연 후 페이지 로드
	time.Sleep(150 * time.Millisecond)
	chromium.Navigate(dockUrl)

	if showInitially {
		procShowWindow.Call(hwnd, 5 /* SW_SHOW */)
		procUpdateWindow.Call(hwnd)
		_ = chromium.Show()
		chromium.Resize()
		chromium.Focus()
		ForceForegroundWindow(hwnd, false)
	}

	return hwnd
}

// DestroyDockWindow: 프로그램 완전 종료 시 윈도우 파괴
func DestroyDockWindow() {
	if dockHwnd != 0 {
		saveDockWindowState(captureDockWindowState(dockHwnd))
		procDestroyWindow.Call(dockHwnd)
		dockHwnd = 0
	}
}

// -----------------------------------------------------------------------------
// 3. 플로팅 툴 웹뷰 공통 엔진 (Floating Tool Engine)
// -----------------------------------------------------------------------------

type ToolWindowState struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Width   int  `json:"width"`
	Height  int  `json:"height"`
	Topmost bool `json:"topmost"`
}

type ToolWebviewConfig struct {
	Title          string
	ClassName      string
	MutexName      string
	StateFileName  string
	StoragePinKey  string
	DefaultWidth   int
	DefaultHeight  int
	InitialTop     string
	InitialLeft    string
	TargetURL      string
	ProfileDir     string
	CookieInjector func(chromium *edge.Chromium)
}

func loadToolWindowState(fileName string, defW, defH int) ToolWindowState {
	defState := ToolWindowState{
		X:       0,
		Y:       0,
		Width:   defW,
		Height:  defH,
		Topmost: false,
	}
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	path := filepath.Join(appData, "ChzzkObsDock", fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return defState
	}
	var state ToolWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return defState
	}
	if state.Width < 300 || state.Height < 400 {
		state.Width = defW
		state.Height = defH
	}
	return state
}

func saveToolWindowState(fileName string, state ToolWindowState) {
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	dir := filepath.Join(appData, "ChzzkObsDock")
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, fileName)
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func makeFloatingToolbarScript(storageKey, defaultTop, defaultLeft string) string {
	return fmt.Sprintf(`
(function() {
  if (window.__chzzkToolbarInjected) return;
  window.__chzzkToolbarInjected = true;

  function initToolbar() {
    if (document.getElementById('chzzk-floating-toolbar')) return;

    var bar = document.createElement('div');
    bar.id = 'chzzk-floating-toolbar';

    var savedPos = null;
    try {
      var raw = localStorage.getItem('%s');
      if (raw) savedPos = JSON.parse(raw);
    } catch(e) {}

    var initialTop = (savedPos && typeof savedPos.top === 'number') ? savedPos.top + 'px' : '%s';
    var initialLeft = (savedPos && typeof savedPos.left === 'number') ? savedPos.left + 'px' : '%s';

    bar.style.cssText = [
      'position: fixed !important',
      'top: ' + initialTop + ' !important',
      'left: ' + initialLeft + ' !important',
      'display: flex !important',
      'gap: 5px !important',
      'align-items: center !important',
      'z-index: 9999999 !important',
      'user-select: none !important',
      '-webkit-user-select: none !important',
      'touch-action: none !important'
    ].join(';');

    function createBtn(id, text, title) {
      var btn = document.createElement('button');
      btn.id = id;
      btn.title = title;
      btn.innerHTML = text;
      btn.style.cssText = [
        'width: 28px !important',
        'height: 28px !important',
        'border-radius: 7px !important',
        'background: #161F2E !important',
        'border: 1px solid #334155 !important',
        'color: #94A3B8 !important',
        'font-size: 13px !important',
        'cursor: grab !important',
        'display: flex !important',
        'align-items: center !important',
        'justify-content: center !important',
        'box-shadow: 0 4px 10px rgba(0, 0, 0, 0.35) !important',
        'transition: background 0.15s, border-color 0.15s, transform 0.1s, box-shadow 0.15s !important',
        'outline: none !important',
        'padding: 0 !important',
        'line-height: 1 !important'
      ].join(';');
      btn.onmouseenter = function() {
        if (!this.dataset.active) {
          this.style.background = '#1E293B !important';
          this.style.borderColor = '#475569 !important';
          this.style.color = '#F8FAFC !important';
        }
      };
      btn.onmouseleave = function() {
        if (!this.dataset.active) {
          this.style.background = '#161F2E !important';
          this.style.borderColor = '#334155 !important';
          this.style.color = '#94A3B8 !important';
        }
      };
      return btn;
    }

    var pinBtn = createBtn('chzzk-floating-pin-btn', '&#x1F4CC;', '항상 위에 고정 (드래그하여 위치 이동)');
    var reloadBtn = createBtn('chzzk-floating-reload-btn', '&#x1F504;', '화면 새로고침 (드래그하여 위치 이동)');

    bar.appendChild(pinBtn);
    bar.appendChild(reloadBtn);

    var isDragging = false;
    var startX, startY, origLeft, origTop;
    var hasMoved = false;

    bar.addEventListener('mousedown', function(e) {
      if (e.button !== 0) return;
      isDragging = true;
      hasMoved = false;
      startX = e.clientX;
      startY = e.clientY;
      var rect = bar.getBoundingClientRect();
      origLeft = rect.left;
      origTop = rect.top;
      reloadBtn.style.cursor = 'grabbing';
      pinBtn.style.cursor = 'grabbing';
      e.preventDefault();

      function onMouseMove(moveEvent) {
        if (!isDragging) return;
        var dx = moveEvent.clientX - startX;
        var dy = moveEvent.clientY - startY;
        if (Math.abs(dx) > 3 || Math.abs(dy) > 3) {
          hasMoved = true;
        }
        var newLeft = Math.max(0, Math.min(window.innerWidth - bar.offsetWidth, origLeft + dx));
        var newTop = Math.max(0, Math.min(window.innerHeight - bar.offsetHeight, origTop + dy));
        bar.style.left = newLeft + 'px';
        bar.style.top = newTop + 'px';
        bar.style.right = 'auto';
      }

      function onMouseUp() {
        if (!isDragging) return;
        isDragging = false;
        reloadBtn.style.cursor = 'grab';
        pinBtn.style.cursor = 'grab';
        document.removeEventListener('mousemove', onMouseMove);
        document.removeEventListener('mouseup', onMouseUp);
        if (hasMoved) {
          try {
            var rect = bar.getBoundingClientRect();
            localStorage.setItem('%s', JSON.stringify({ left: rect.left, top: rect.top }));
          } catch(err) {}
        }
      }

      document.addEventListener('mousemove', onMouseMove);
      document.addEventListener('mouseup', onMouseUp);
    });

    reloadBtn.addEventListener('click', function(e) {
      e.stopPropagation();
      e.preventDefault();
      if (hasMoved) { hasMoved = false; return; }
      location.reload();
    });

    pinBtn.addEventListener('click', function(e) {
      e.stopPropagation();
      e.preventDefault();
      if (hasMoved) { hasMoved = false; return; }
      if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage('toggle-topmost');
      }
    });

    document.documentElement.appendChild(bar);

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
    document.addEventListener('DOMContentLoaded', initToolbar);
  } else {
    initToolbar();
  }
})();
`, storageKey, defaultTop, defaultLeft, storageKey)
}

// RunToolWebview: 치지직 팝업 툴(채팅창, 리모컨 등) 공통 실행 함수
func RunToolWebview(cfg ToolWebviewConfig) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")

	// 1. 단일 인스턴스 Mutex 검사
	mutexNamePtr, _ := syscall.UTF16PtrFromString(cfg.MutexName)
	mutexHandle, _, errCall := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	errno, isErrno := errCall.(syscall.Errno)
	if isErrno && errno == 183 { // ERROR_ALREADY_EXISTS
		classNamePtr, _ := syscall.UTF16PtrFromString(cfg.ClassName)
		existingHwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(classNamePtr)), 0)
		if existingHwnd != 0 {
			user32.NewProc("ShowWindow").Call(existingHwnd, 9 /* SW_RESTORE */)
			ForceForegroundWindow(existingHwnd, false)
		}
		if mutexHandle != 0 {
			kernel32.NewProc("CloseHandle").Call(mutexHandle)
		}
		return
	}
	defer func() {
		if mutexHandle != 0 {
			kernel32.NewProc("CloseHandle").Call(mutexHandle)
		}
	}()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString(cfg.ClassName)
	windowTitle, _ := syscall.UTF16PtrFromString(cfg.Title)
	hIcon := LoadAppIcon()

	hBrush, _, _ := procCreateSolidBrush.Call(0x00110E0B) // #0B0E11 다크 브러시
	state := loadToolWindowState(cfg.StateFileName, cfg.DefaultWidth, cfg.DefaultHeight)
	isTopmost := state.Topmost

	var activeHwnd uintptr
	var activeChromium *edge.Chromium

	wndProc := func(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
		switch msg {
		case WM_SIZE_WV:
			if activeChromium != nil {
				activeChromium.Resize()
			}
			return 0
		case WM_DESTROY_WV:
			var rect struct{ Left, Top, Right, Bottom int32 }
			procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
			w := int(rect.Right - rect.Left)
			h := int(rect.Bottom - rect.Top)
			if w >= 300 && h >= 400 {
				state.X = int(rect.Left)
				state.Y = int(rect.Top)
				state.Width = w
				state.Height = h
				state.Topmost = isTopmost
				saveToolWindowState(cfg.StateFileName, state)
			}
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	}

	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     syscall.Handle(hInst),
		HIcon:         hIcon,
		HCursor:       syscall.Handle(0),
		HbrBackground: syscall.Handle(hBrush),
		LpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	screenW, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	if state.X <= 0 || state.Y <= 0 || (screenW > 0 && state.X >= int(screenW)-100) || (screenH > 0 && state.Y >= int(screenH)-100) {
		if screenW > 0 && screenH > 0 {
			state.X = (int(screenW) - state.Width) / 2
			state.Y = (int(screenH) - state.Height) / 2
		} else {
			state.X = 150
			state.Y = 150
		}
	}

	var exStyle uintptr = WS_EX_APPWINDOW_VAL
	if isTopmost {
		exStyle |= WS_EX_TOPMOST_VAL
	}
	dwStyle := uintptr(WS_OVERLAPPEDWINDOW_VAL)

	hwnd, _, _ := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		dwStyle,
		uintptr(state.X), uintptr(state.Y), uintptr(state.Width), uintptr(state.Height),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		LogError("[%s] [%s] 창 생성 실패", cfg.Title, ErrSysFileIoFailed)
		return
	}
	activeHwnd = hwnd

	if hIcon != 0 {
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), uintptr(hIcon))
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), uintptr(hIcon))
	}

	applyDarkTheme(hwnd)

	chromium := edge.NewChromium()
	chromium.DataPath = cfg.ProfileDir

	browserArgs := []string{
		"--disable-features=CalculateNativeWinOcclusion",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	}
	if !GetEnableGPU() {
		browserArgs = append(browserArgs, "--disable-gpu")
	}
	chromium.AdditionalBrowserArgs = browserArgs
	activeChromium = chromium

	// 웹뷰 IPC 메시지 핸들러
	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		switch message {
		case "toggle-topmost":
			isTopmost = !isTopmost
			var insertAfter uintptr = HWND_NOTOPMOST_VAL
			if isTopmost {
				insertAfter = HWND_TOPMOST_VAL
			}
			user32.NewProc("SetWindowPos").Call(
				activeHwnd, insertAfter, 0, 0, 0, 0,
				uintptr(SWP_NOMOVE_VAL|SWP_NOSIZE_VAL),
			)
			chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%v);", isTopmost))
		case "get-topmost-state":
			chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%v);", isTopmost))
		default:
			if strings.HasPrefix(message, "open-external:") {
				targetUrl := strings.TrimPrefix(message, "open-external:")
				if targetUrl != "" {
					OpenBrowser(targetUrl)
				}
			}
		}
	}

	// 공통 링크 가드 및 플로팅 툴바 주입
	chromium.Init(ExternalLinkGuardScript)
	chromium.Init(makeFloatingToolbarScript(cfg.StoragePinKey, cfg.InitialTop, cfg.InitialLeft))

	if !chromium.Embed(hwnd) {
		LogError("[%s] [%s] WebView2 임베딩 실패", cfg.Title, ErrSysWebviewRuntime)
		procDestroyWindow.Call(hwnd)
		return
	}

	// 창 표시 및 WebView2 바인딩
	procShowWindow.Call(hwnd, 5 /* SW_SHOW */)
	procUpdateWindow.Call(hwnd)
	ForceForegroundWindow(hwnd, isTopmost)

	if cfg.CookieInjector != nil {
		cfg.CookieInjector(chromium)
	}
	chromium.Navigate(cfg.TargetURL)

	var msg struct {
		Hwnd    syscall.Handle
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      POINT
	}
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// -----------------------------------------------------------------------------
// 4. 플로팅 실시간 채팅창 (Floating Chat Window)
// -----------------------------------------------------------------------------

// RunChatWebview: 치지직 라이브 팝업 채팅창 실행
func RunChatWebview(channelId string) {
	directChatUrl := fmt.Sprintf("https://chzzk.naver.com/live/%s/chat", channelId)
	targetUrl := "https://nid.naver.com/nidlogin.login?url=" + url.QueryEscape(directChatUrl)

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")
	_ = os.MkdirAll(profileDir, 0755)

	RunToolWebview(ToolWebviewConfig{
		Title:         "치지직 채팅창",
		ClassName:     "ChzzkChatWindowClass",
		MutexName:     `Local\ChzzkChatWindowMutex`,
		StateFileName: "chat_window.json",
		StoragePinKey: "chzzk_chat_pin_pos",
		DefaultWidth:  440,
		DefaultHeight: 720,
		InitialTop:    "10",
		InitialLeft:   "14",
		TargetURL:     targetUrl,
		ProfileDir:    profileDir,
		CookieInjector: func(chromium *edge.Chromium) {
			cfg := LoadConfig()
			if cfg.NidAut != "" && cfg.NidSes != "" {
				if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
					InjectRemoteCookies(cm, cfg.NidAut, cfg.NidSes)
				}
			}
		},
	})
}

// BringChatWindowToFront: 이미 켜져 있는 채팅창을 최상단으로 복원
func BringChatWindowToFront() bool {
	user32 := syscall.NewLazyDLL("user32.dll")
	classNamePtr, _ := syscall.UTF16PtrFromString("ChzzkChatWindowClass")
	hwnd, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(classNamePtr)), 0)
	if hwnd != 0 {
		user32.NewProc("ShowWindow").Call(hwnd, 9 /* SW_RESTORE */)
		ForceForegroundWindow(hwnd, false)
		return true
	}
	return false
}

// -----------------------------------------------------------------------------
// 5. 플로팅 공식 스튜디오 리모컨 창 (Floating Remote Window)
// -----------------------------------------------------------------------------

type RemoteWindowState = ToolWindowState

func loadRemoteWindowState() RemoteWindowState {
	return loadToolWindowState("remote_window.json", 720, 880)
}

func saveRemoteWindowState(state RemoteWindowState) {
	saveToolWindowState("remote_window.json", state)
}

// RunRemoteWebview: 치지직 공식 리모컨 웹뷰 창 실행
func RunRemoteWebview(channelId string) {
	targetUrl := fmt.Sprintf("https://studio.chzzk.naver.com/%s/remotecontrol", channelId)

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")
	_ = os.MkdirAll(profileDir, 0755)

	RunToolWebview(ToolWebviewConfig{
		Title:         "치지직 리모컨",
		ClassName:     "ChzzkRemoteWindowClass",
		MutexName:     `Local\ChzzkRemoteWindowMutex`,
		StateFileName: "remote_window.json",
		StoragePinKey: "chzzk_remote_pin_pos",
		DefaultWidth:  720,
		DefaultHeight: 880,
		InitialTop:    "10",
		InitialLeft:   "14",
		TargetURL:     targetUrl,
		ProfileDir:    profileDir,
		CookieInjector: func(chromium *edge.Chromium) {
			cfg := LoadConfig()
			if cfg.NidAut != "" && cfg.NidSes != "" {
				if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
					InjectRemoteCookies(cm, cfg.NidAut, cfg.NidSes)
				}
			}
		},
	})
}

// InjectRemoteCookies: 리모컨/채팅 웹뷰에 네이버 인증 쿠키를 주입
func InjectRemoteCookies(cookieManager *edge.ICoreWebView2CookieManager, aut, ses string) {
	if cookieManager == nil || aut == "" || ses == "" {
		return
	}
	targetDomain := ".naver.com"
	if cookie, err := cookieManager.CreateCookie("NID_AUT", aut, targetDomain, "/"); err == nil && cookie != nil {
		_ = cookie.PutIsSecure(true)
		_ = cookie.PutIsHttpOnly(true)
		_ = cookieManager.AddOrUpdateCookie(cookie)
		cookie.Release()
	}
	if cookie, err := cookieManager.CreateCookie("NID_SES", ses, targetDomain, "/"); err == nil && cookie != nil {
		_ = cookie.PutIsSecure(true)
		_ = cookie.PutIsHttpOnly(true)
		_ = cookieManager.AddOrUpdateCookie(cookie)
		cookie.Release()
	}
}
