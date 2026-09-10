package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

// ==============================================================================
// [CHZZK OBS DOCK - Standalone Dock WebView Window]
// - exe 실행 시 웹뷰 화면으로 방송 독 UI(http://127.0.0.1:<port>) 즉시 표시
// - 창 닫기(X 버튼 / 화면 닫기) 시 프로세스 종료 없이 백그라운드/트레이로 숨김 처리
// - 중복 실행 시 버전별 단일 인스턴스 인식 및 기존 창 포커스/복원
// - Win32 DWM 다크 테마 일체화 및 창 위치/크기 영속성 보장
// ==============================================================================

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
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")

	activeDockChromium *edge.Chromium
	dockHwnd           uintptr
	dockRegisteredMsg  uint32
	dockVersionKey     string
)

// GetVersionKey: "0.5.1" -> "0_5_1" 등 Win32 Mutex/Class에 안전한 식별자로 변환
func GetVersionKey(version string) string {
	clean := strings.ReplaceAll(version, ".", "_")
	clean = strings.ReplaceAll(clean, "-", "_")
	clean = strings.ReplaceAll(clean, " ", "_")
	return clean
}

// GetShowDockMessageId: 버전별 고유 등록 윈도우 메시지 ID 조회 (RegisterWindowMessageW)
func GetShowDockMessageId(verKey string) uint32 {
	msgNamePtr, _ := syscall.UTF16PtrFromString(fmt.Sprintf("ChzzkDock_ShowUI_%s", verKey))
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
	procShowWindow.Call(dockHwnd, 9 /* SW_RESTORE */)
	procUpdateWindow.Call(dockHwnd)
	if activeDockChromium != nil {
		_ = activeDockChromium.Show()
		activeDockChromium.Resize()
		activeDockChromium.Focus()
	}
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
	verKey := GetVersionKey(version)
	dockVersionKey = verKey
	dockRegisteredMsg = GetShowDockMessageId(verKey)

	classNameStr := fmt.Sprintf("ChzzkDockWindowClass_%s", verKey)
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
		LogError("[Dock Window Error] 독 윈도우 생성 실패")
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

	chromium.AdditionalBrowserArgs = []string{
		"--disable-features=CalculateNativeWinOcclusion",
		"--disable-background-timer-throttling",
		"--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding",
	}
	activeDockChromium = chromium

	chromium.ProcessFailedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		LogError("[Dock Window] WebView2 렌더러 프로세스 장애 발생")
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
		LogWarn("[Dock Window] WebView2 임베딩 실패 (WebView2 Runtime 미설치 가능성)")
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
