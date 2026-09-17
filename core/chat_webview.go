package core

import (
	"encoding/json"
	"fmt"
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
// [CHZZK OBS DOCK - Chat Webview Window]
// - 치지직 실시간 라이브 채팅창 (chzzk.naver.com/live/{channelId}/chat)
// - Win32 DWM 다크 테마 일체화
// - 상단 커스텀 플로팅 툴바 (📌 항상 위 토글, 🔄 새로고침)
// - 마지막 창 위치 / 크기 / Always-on-top 영속적 기억 (%LOCALAPPDATA%\ChzzkObsDock\chat_window.json)
// - webview_profile 공유로 네이버 세션 0초 자동 로그인 유지
// - 단일 인스턴스 Mutex 및 중복 실행 시 기존 창 포커스
// ==============================================================================

type ChatWindowState struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Width   int  `json:"width"`
	Height  int  `json:"height"`
	Topmost bool `json:"topmost"`
}

var (
	activeChatChromium *edge.Chromium
	isChatTopmost      = false
	chatHwnd           uintptr
)

func getChatWindowStatePath() string {
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	return filepath.Join(appData, "ChzzkObsDock", "chat_window.json")
}

func loadChatWindowState() ChatWindowState {
	def := ChatWindowState{
		X:       0,
		Y:       0,
		Width:   380,
		Height:  720,
		Topmost: true, // 채팅창은 기본 Always-on-top 권장
	}

	path := getChatWindowStatePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}

	var state ChatWindowState
	if err := json.Unmarshal(data, &state); err != nil {
		return def
	}

	if state.Width < 280 || state.Height < 400 {
		state.Width = 380
		state.Height = 720
	}
	return state
}

func saveChatWindowState(state ChatWindowState) {
	path := getChatWindowStatePath()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func updateChatWindowTitle(hwnd uintptr, topmost bool) {
	title := "치지직 채팅 - CHZZK OBS Dock"
	if topmost {
		title = "📌 치지직 채팅 - CHZZK OBS Dock"
	}
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(titlePtr)))
}

func setChatWindowTopmost(hwnd uintptr, topmost bool) {
	ForceForegroundWindow(hwnd, topmost)
	updateSystemMenuTopmost(hwnd, topmost)
	updateChatWindowTitle(hwnd, topmost)
}

func toggleChatTopmost(hwnd uintptr) {
	isChatTopmost = !isChatTopmost
	setChatWindowTopmost(hwnd, isChatTopmost)
	st := captureCurrentChatWindowState(hwnd)
	st.Topmost = isChatTopmost
	saveChatWindowState(st)
	if activeChatChromium != nil {
		activeChatChromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isChatTopmost))
	}
}

func captureCurrentChatWindowState(hwnd uintptr) ChatWindowState {
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

	if w < 280 || h < 400 {
		w = 380
		h = 720
	}

	return ChatWindowState{
		X:       x,
		Y:       y,
		Width:   w,
		Height:  h,
		Topmost: isChatTopmost,
	}
}

func chatWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_MOVE_WV:
		if activeChatChromium != nil {
			_ = activeChatChromium.NotifyParentWindowPositionChanged()
		}
		return 0

	case WM_SIZE_WV:
		if activeChatChromium != nil {
			activeChatChromium.Resize()
		}
		return 0

	case WM_EXITSIZEMOVE_VAL:
		state := captureCurrentChatWindowState(uintptr(hwnd))
		saveChatWindowState(state)
		return 0

	case WM_SYSCOMMAND_VAL:
		if (wParam & 0xFFF0) == IDM_REMOTE_TOPMOST {
			toggleChatTopmost(uintptr(hwnd))
			return 0
		}

	case WM_CLOSE_VAL, WM_DESTROY_WV:
		state := captureCurrentChatWindowState(uintptr(hwnd))
		saveChatWindowState(state)
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// RunChatWebview: 치지직 공식 채팅창 독립 WebView2 플로팅 윈도우 구동
func RunChatWebview(channelId string) {
	runtime.LockOSThread()
	LogInfo("[Chat Webview] 치지직 채팅창 시작 요청됨 (Channel ID: %s)", channelId)

	// [단일 인스턴스 보장 및 중복 실행 시 기존 창 포커스]
	mutexNamePtr, _ := syscall.UTF16PtrFromString(`Local\ChzzkObsDock_Chat_Mutex`)
	mutexHandle, _, _ := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	if syscall.GetLastError() == syscall.ERROR_ALREADY_EXISTS {
		LogWarn("[Chat Webview] 이미 실행 중인 채팅창이 감지되었습니다. 기존 창을 전면으로 활성화합니다.")
		classPtr, _ := syscall.UTF16PtrFromString("ChzzkChatWebviewClass")
		existingHwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(classPtr)), 0)
		if existingHwnd != 0 {
			ForceForegroundWindow(existingHwnd, isChatTopmost)
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

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")
	_ = os.MkdirAll(profileDir, 0755)

	className, _ := syscall.UTF16PtrFromString("ChzzkChatWebviewClass")
	windowTitle, _ := syscall.UTF16PtrFromString("치지직 채팅 - CHZZK OBS Dock")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hIcon := LoadAppIcon()

	darkBrush, _, _ := procCreateSolidBrush.Call(uintptr(0x001B1818))

	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(chatWndProc),
		HInstance:     syscall.Handle(hInst),
		HIcon:         hIcon,
		HCursor:       syscall.Handle(0),
		HbrBackground: syscall.Handle(darkBrush),
		LpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	state := loadChatWindowState()
	isChatTopmost = state.Topmost

	winW := state.Width
	winH := state.Height
	winX := state.X
	winY := state.Y

	screenW, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	if winX <= 0 || winY <= 0 || winX >= int(screenW)-100 || winY >= int(screenH)-100 {
		if screenW > 0 && screenH > 0 {
			winX = int(screenW) - winW - 30
			winY = (int(screenH) - winH) / 2
		} else {
			winX = 100
			winY = 100
		}
	}

	windowStyle := uint32(WS_OVERLAPPEDWINDOW_VAL)
	exStyle := uint32(WS_EX_APPWINDOW_VAL)
	if isChatTopmost {
		exStyle |= WS_EX_TOPMOST_VAL
	}

	var r RECT_WIN
	r.Left = int32(winX)
	r.Top = int32(winY)
	r.Right = int32(winX + winW)
	r.Bottom = int32(winY + winH)
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), uintptr(windowStyle), 0, uintptr(exStyle))

	realW := r.Right - r.Left
	realH := r.Bottom - r.Top

	hwnd, _, _ := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		uintptr(windowStyle),
		uintptr(winX),
		uintptr(winY),
		uintptr(realW),
		uintptr(realH),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		LogError("[Chat Webview] 창 생성 실패 (CreateWindowExW return 0)")
		return
	}

	chatHwnd = hwnd

	if hIcon != 0 {
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), uintptr(hIcon))
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), uintptr(hIcon))
	}

	hSysMenu, _, _ := procGetSystemMenu.Call(hwnd, 0)
	if hSysMenu != 0 {
		procAppendMenuW.Call(hSysMenu, MF_SEPARATOR, 0, 0)
		menuText, _ := syscall.UTF16PtrFromString("📌 항상 위에 고정")
		procAppendMenuW.Call(hSysMenu, MF_STRING, IDM_REMOTE_TOPMOST, uintptr(unsafe.Pointer(menuText)))
	}

	applyDarkTheme(hwnd)
	setChatWindowTopmost(hwnd, isChatTopmost)

	chromium := edge.NewChromium()
	chromium.DataPath = profileDir

	chatBrowserArgs := []string{
		"--disable-features=CalculateNativeWinOcclusion",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
		"--disk-cache-size=268435456",
	}
	if !GetEnableGPU() {
		chatBrowserArgs = append(chatBrowserArgs, "--disable-gpu")
	}
	chromium.AdditionalBrowserArgs = chatBrowserArgs
	activeChatChromium = chromium

	chromium.ProcessFailedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		LogError("[Chat Webview] WebView2 렌더러 프로세스 장애 발생.")
	}

	chromium.NavigationCompletedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		src, _ := sender.GetSource()
		LogInfo("[Chat Webview] 채팅 페이지 로드 완료: %s", src)
		// 상단 플로팅 툴바(📌 항상 위, 🔄 새로고침) 주입
		chromium.Eval(remoteOverlayScript)
		chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isChatTopmost))
	}

	if !chromium.Embed(hwnd) {
		LogError("[Chat Webview Error] WebView2 임베딩 실패 (WebView2 Runtime 확인 필요)")
		procDestroyWindow.Call(hwnd)
		return
	}

	chromium.SetBackgroundColour(0x0B, 0x0E, 0x11, 255)
	chromium.Init(remoteOverlayScript)

	// WebMessage 이벤트 핸들러 (항상 위 토글, 상태 동기화, 외부 링크 브라우저 핸드오프)
	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		switch {
		case message == "toggle-topmost":
			toggleChatTopmost(hwnd)
		case message == "get-topmost-state":
			chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isChatTopmost))
		case strings.HasPrefix(message, "open-external:"):
			targetURL := strings.TrimPrefix(message, "open-external:")
			if GetExternalBrowserGuard() {
				browserName := GetDefaultBrowserName()
				toastMsg := fmt.Sprintf("🔗 외부 링크를 %s(으)로 엽니다.", browserName)
				escapedMsg, _ := json.Marshal(toastMsg)
				chromium.Eval(fmt.Sprintf("if (window.__showToast) window.__showToast(%s);", string(escapedMsg)))
				OpenBrowser(targetURL)
			} else {
				chromium.Navigate(targetURL)
			}
		}
	}

	// 수동 쿠키 주입 (auth_method == "manual" 상태일 때만 1회 주입)
	cfg := LoadConfig()
	if cfg.AuthMethod == "manual" && cfg.NidAut != "" && cfg.NidSes != "" {
		if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
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
			SaveConfig(map[string]interface{}{
				"auth_method": "webview",
			})
			LogInfo("[Chat Webview] 수동 설정 쿠키를 webview_profile에 1회 동기화 완료.")
		}
	}

	targetURL := "https://chzzk.naver.com"
	if channelId != "" {
		targetURL = fmt.Sprintf("https://chzzk.naver.com/live/%s/chat", channelId)
	}

	LogInfo("[Chat Webview] 채팅 URL 이동: %s", targetURL)
	chromium.Navigate(targetURL)

	_ = chromium.Show()
	chromium.Focus()
	chromium.Resize()

	ForceForegroundWindow(hwnd, isChatTopmost)

	var m MSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	time.Sleep(100 * time.Millisecond)
}
