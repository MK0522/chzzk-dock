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
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procBringWindowToTop = user32.NewProc("BringWindowToTop")

	dwmapiDLL                 = syscall.NewLazyDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapiDLL.NewProc("DwmSetWindowAttribute")
)

const (
	HWND_TOPMOST_VAL   = ^uintptr(0) // -1
	HWND_NOTOPMOST_VAL = ^uintptr(1) // -2
	SWP_NOSIZE_VAL     = 0x0001
	SWP_NOMOVE_VAL     = 0x0002

	DWMWA_USE_IMMERSIVE_DARK_MODE_OLD = 19
	DWMWA_USE_IMMERSIVE_DARK_MODE     = 20
	DWMWA_CAPTION_COLOR               = 35

	SM_CXSCREEN = 0
	SM_CYSCREEN = 1

	WM_CLOSE_VAL = 0x0010
)

var (
	activeRemoteChromium *edge.Chromium
	remoteHwnd           uintptr
	isRemoteTopmost      = true
	currentChannelId     = ""
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
		Width:   700,
		Height:  760,
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

	if state.Width < 500 || state.Height < 550 {
		state.Width = 700
		state.Height = 760
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
}

func setWindowTopmost(hwnd uintptr, topmost bool) {
	target := HWND_NOTOPMOST_VAL
	if topmost {
		target = HWND_TOPMOST_VAL
	}
	procSetWindowPos.Call(hwnd, target, 0, 0, 0, 0, SWP_NOMOVE_VAL|SWP_NOSIZE_VAL)
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
	case WM_SIZE_WV:
		if activeRemoteChromium != nil {
			activeRemoteChromium.Resize()
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

    var bar = document.createElement('div');
    bar.id = 'chzzk-remote-toolbar';
    bar.style.cssText = 'position:fixed; top:0; left:0; right:0; height:32px; background:#0B0E11; border-bottom:1px solid #1E2738; z-index:999999; display:flex; align-items:center; justify-content:space-between; padding:0 10px; box-sizing:border-box; font-family:Pretendard,-apple-system,BlinkMacSystemFont,sans-serif; user-select:none; -webkit-user-select:none; box-shadow:0 2px 10px rgba(0,0,0,0.6);';

    bar.innerHTML = '<div style="display:flex; align-items:center; gap:6px;">' +
      '<span style="display:inline-block; width:7px; height:7px; border-radius:50%; background:#00FFA3; box-shadow:0 0 8px rgba(0,255,163,0.8);"></span>' +
      '<span style="font-size:11.5px; font-weight:700; color:#F1F5F9; letter-spacing:-0.2px;">치지직 리모컨</span>' +
      '</div>' +
      '<div style="display:flex; align-items:center; gap:5px;">' +
      '<button id="chzzk-btn-pin" style="background:#161F2E; border:1px solid #243044; color:#A8B3C7; font-size:11px; font-weight:600; padding:2px 8px; border-radius:5px; cursor:pointer; display:flex; align-items:center; gap:3.5px; transition:all 0.15s;" title="항상 위 고정 토글">' +
      '<span id="pin-icon" style="font-size:10px;">📌</span><span id="pin-text">항상 위</span>' +
      '</button>' +
      '<button id="chzzk-btn-reload" style="background:#161F2E; border:1px solid #243044; color:#A8B3C7; font-size:11px; padding:2px 6px; border-radius:5px; cursor:pointer; display:flex; align-items:center; transition:all 0.15s;" title="새로고침">' +
      '🔄' +
      '</button>' +
      '</div>';

    document.documentElement.appendChild(bar);

    var style = document.createElement('style');
    style.innerHTML = 'body { padding-top: 32px !important; box-sizing: border-box !important; }';
    document.documentElement.appendChild(style);

    var pinBtn = document.getElementById('chzzk-btn-pin');
    pinBtn.onclick = function() {
      if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage('toggle-topmost');
      }
    };

    var reloadBtn = document.getElementById('chzzk-btn-reload');
    reloadBtn.onclick = function() {
      location.reload();
    };

    window.__updateTopmostUI = function(isTopmost) {
      var btn = document.getElementById('chzzk-btn-pin');
      var txt = document.getElementById('pin-text');
      if (!btn) return;
      if (isTopmost) {
        btn.style.background = 'rgba(0, 255, 163, 0.15)';
        btn.style.borderColor = '#00FFA3';
        btn.style.color = '#00FFA3';
        btn.style.boxShadow = '0 0 8px rgba(0, 255, 163, 0.35)';
        if (txt) txt.innerText = '고정됨';
      } else {
        btn.style.background = '#161F2E';
        btn.style.borderColor = '#243044';
        btn.style.color = '#A8B3C7';
        btn.style.boxShadow = 'none';
        if (txt) txt.innerText = '항상 위';
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
		fmt.Println("[Remote Webview] 이미 리모컨 창이 실행 중입니다. 기존 창을 맨 앞으로 가져옵니다.")
		existingHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
		if existingHwnd != 0 {
			procShowWindow.Call(existingHwnd, 9) // SW_RESTORE
			setWindowTopmost(existingHwnd, true)
			procSetForegroundWindow.Call(existingHwnd)
			procBringWindowToTop.Call(existingHwnd)
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

	currentChannelId = channelId

	// 저장된 창 위치 및 크기 복원
	state := loadRemoteWindowState()
	isRemoteTopmost = state.Topmost

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview2_profile")
	_ = os.MkdirAll(profileDir, 0755)

	windowTitle, _ := syscall.UTF16PtrFromString("치지직 리모컨 - CHZZK OBS Dock")
	hInst, _, _ := procGetModuleHandleW.Call(0)

	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(remoteWndProc),
		HInstance:     syscall.Handle(hInst),
		HCursor:       syscall.Handle(0),
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

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		0x00CF0000, // WS_OVERLAPPEDWINDOW
		uintptr(state.X), uintptr(state.Y), uintptr(state.Width), uintptr(state.Height),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		fmt.Println("[Remote Webview Error] 리모컨 윈도우 생성 실패")
		return
	}
	remoteHwnd = hwnd

	// DWM 다크 테마 및 프레임 일체화 적용
	applyDarkTheme(hwnd)

	// 저장된 Topmost 상태 적용
	if isRemoteTopmost {
		setWindowTopmost(hwnd, true)
	}

	chromium := edge.NewChromium()
	chromium.DataPath = profileDir
	activeRemoteChromium = chromium

	if !chromium.Embed(hwnd) {
		fmt.Println("[Remote Webview Error] WebView2 임베딩 실패 (WebView2 Runtime 확인 필요)")
		procDestroyWindow.Call(hwnd)
		return
	}

	// 상단 커스텀 툴바 주입 (Embed 완료 후 e.webview가 생성된 시점에 호출)
	chromium.Init(remoteToolbarScript)

	// WebMessage 이벤트 핸들러 (항상 위 토글 등)
	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		switch message {
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

	// [쿠키 사전 주입: 0초 무로그인 인증]
	cm, err := chromium.GetCookieManager()
	if err == nil && cm != nil {
		cfg := LoadConfig()
		domains := []string{"naver.com", "chzzk.naver.com", "studio.chzzk.naver.com"}
		for _, dom := range domains {
			if cfg.NidAut != "" {
				if cookie, err := cm.CreateCookie("NID_AUT", cfg.NidAut, dom, "/"); err == nil && cookie != nil {
					_ = cm.AddOrUpdateCookie(cookie)
					cookie.Release()
				}
			}
			if cfg.NidSes != "" {
				if cookie, err := cm.CreateCookie("NID_SES", cfg.NidSes, dom, "/"); err == nil && cookie != nil {
					_ = cm.AddOrUpdateCookie(cookie)
					cookie.Release()
				}
			}
		}
	}

	setWindowTopmost(hwnd, true)
	procShowWindow.Call(hwnd, 5) // SW_SHOW
	procUpdateWindow.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
	procBringWindowToTop.Call(hwnd)

	chromium.Resize()

	// 공식 리모컨 페이지 URL 로드
	remoteURL := "https://studio.chzzk.naver.com"
	if channelId != "" {
		remoteURL = fmt.Sprintf("https://studio.chzzk.naver.com/%s/remotecontrol", channelId)
	}
	chromium.Navigate(remoteURL)

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
