package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/combridge"
	"github.com/wailsapp/go-webview2/pkg/edge"
)

// ==============================================================================
// [SECURITY & AUDIT NOTE: Naver Login & Cookie Extraction]
// - 목적: 네이버 치지직 방송 제어(제목, 카테고리, 채팅 설정 등)를 위한 세션 연동
// - 아키텍처: Windows 공식 WebView2 런타임 및 ICoreWebView2CookieManager 정식 인터페이스 사용
// - 안전성 보장:
//   * 별도의 디버깅 포트나 WebSocket CDP 등 악성 의심 패턴 원천 배제
//   * Microsoft 공식 Evergreen WebView2 COM 인터페이스로 안전하게 로그인 및 쿠키 취득
//   * 추출된 세션 쿠키는 Windows 자격 증명 관리자(Windows Vault)에 암호화 보관
// ==============================================================================

const (
	LOGIN_URL = "https://nid.naver.com/nidlogin.login?url=https%3A%2F%2Fchzzk.naver.com%2F"
)

var (
	ole32DLL          = syscall.NewLazyDLL("ole32.dll")
	procCoTaskMemFree = ole32DLL.NewProc("CoTaskMemFree")

	procShowWindow      = user32.NewProc("ShowWindow")
	procUpdateWindow    = user32.NewProc("UpdateWindow")
	procSetTimer        = user32.NewProc("SetTimer")
	procKillTimer       = user32.NewProc("KillTimer")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
)

const (
	WM_MOVE_WV    = 0x0003
	WM_SIZE_WV    = 0x0005
	WM_CLOSE_WV   = 0x0010
	WM_DESTROY_WV = 0x0002
	WM_TIMER_WV   = 0x0113
	TIMER_ID_WV   = 2001
)

var (
	activeChromium *edge.Chromium
	cookieCaptured = false
)

// ClearWebViewSession: 브라우저 정적 자원(HTML/CSS/JS 및 V8 컴파일 캐시)은 보존하면서,
// 계정 전환 및 자동 로그인을 차단하기 위해 세션/쿠키/로컬 스토리지 데이터만 선별적으로 안전하게 제거합니다.
func ClearWebViewSession(profileDir string) {
	if profileDir == "" {
		return
	}
	defaultDir := filepath.Join(profileDir, "EBWebView", "Default")
	if _, err := os.Stat(defaultDir); os.IsNotExist(err) {
		return
	}

	patterns := []string{
		filepath.Join(defaultDir, "Network", "Cookies*"),
		filepath.Join(defaultDir, "Cookies*"),
		filepath.Join(defaultDir, "Local Storage"),
		filepath.Join(defaultDir, "Session Storage"),
		filepath.Join(defaultDir, "IndexedDB"),
		filepath.Join(defaultDir, "Web Data*"),
		filepath.Join(defaultDir, "Login Data*"),
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			for _, m := range matches {
				_ = os.RemoveAll(m)
			}
		} else {
			_ = os.RemoveAll(pattern)
		}
	}
	LogInfo("[Webview] 브라우저 정적 캐시 보존 및 세션/쿠키 데이터 선별 초기화 완료")
}

// COM vtable definitions for ICoreWebView2GetCookiesCompletedHandler
type ICoreWebView2GetCookiesCompletedHandler interface {
	Invoke(errorCode int32, result uintptr) int32
}

type iCoreWebView2GetCookiesCompletedHandler interface {
	combridge.IUnknown
	ICoreWebView2GetCookiesCompletedHandler
}

type cookiesCompletedHandler struct {
	onDone func(uintptr, error)
}

func (h *cookiesCompletedHandler) Invoke(errorCode int32, result uintptr) int32 {
	if errorCode != 0 {
		h.onDone(0, fmt.Errorf("GetCookies failed: 0x%X", uint32(errorCode)))
		return 0
	}
	h.onDone(result, nil)
	return 0
}

func init() {
	combridge.RegisterVTable[combridge.IUnknown, iCoreWebView2GetCookiesCompletedHandler](
		"{5a4f5069-5c15-47c3-8646-f4de1c116670}",
		_iCoreWebView2GetCookiesCompletedHandlerInvoke,
	)
}

func _iCoreWebView2GetCookiesCompletedHandlerInvoke(this uintptr, errorCode int32, result uintptr) uintptr {
	res := combridge.Resolve[iCoreWebView2GetCookiesCompletedHandler](this).Invoke(errorCode, result)
	return uintptr(res)
}

// callGetCookies executes ICoreWebView2CookieManager::GetCookies via COM vtable
func callGetCookies(cm *edge.ICoreWebView2CookieManager, uri string, onDone func(uintptr, error)) error {
	uriUTF16, err := syscall.UTF16PtrFromString(uri)
	if err != nil {
		return err
	}

	handlerObj := &cookiesCompletedHandler{onDone: onDone}
	comPtr := combridge.New[iCoreWebView2GetCookiesCompletedHandler](handlerObj)
	_ = comPtr // Keep alive for the asynchronous callback

	vtablePtr := *(*uintptr)(unsafe.Pointer(cm))
	vtable := (*[10]uintptr)(unsafe.Pointer(vtablePtr))
	getCookiesProc := vtable[5]

	hr, _, _ := syscall.SyscallN(
		getCookiesProc,
		uintptr(unsafe.Pointer(cm)),
		uintptr(unsafe.Pointer(uriUTF16)),
		comPtr.Ref(),
	)
	if hr != 0 {
		return syscall.Errno(hr)
	}
	return nil
}

// inspectCookies parses the ICoreWebView2CookieList and searches for NID_AUT and NID_SES
func inspectCookies(listPtr uintptr) (aut, ses string, found bool) {
	if listPtr == 0 {
		return "", "", false
	}
	vtablePtr := *(*uintptr)(unsafe.Pointer(listPtr))
	vtable := (*[5]uintptr)(unsafe.Pointer(vtablePtr))

	var count uint32
	hr, _, _ := syscall.SyscallN(vtable[3], listPtr, uintptr(unsafe.Pointer(&count)))
	if hr != 0 || count == 0 {
		return "", "", false
	}

	for i := uint32(0); i < count; i++ {
		var cookiePtr uintptr
		hr, _, _ := syscall.SyscallN(vtable[4], listPtr, uintptr(i), uintptr(unsafe.Pointer(&cookiePtr)))
		if hr != 0 || cookiePtr == 0 {
			continue
		}

		cookieVtablePtr := *(*uintptr)(unsafe.Pointer(cookiePtr))
		cookieVtable := (*[15]uintptr)(unsafe.Pointer(cookieVtablePtr))

		var namePtr, valPtr *uint16
		syscall.SyscallN(cookieVtable[3], cookiePtr, uintptr(unsafe.Pointer(&namePtr)))
		syscall.SyscallN(cookieVtable[4], cookiePtr, uintptr(unsafe.Pointer(&valPtr)))

		cName := ""
		cVal := ""
		if namePtr != nil {
			cName = syscall.UTF16ToString((*[4096]uint16)(unsafe.Pointer(namePtr))[:])
			procCoTaskMemFree.Call(uintptr(unsafe.Pointer(namePtr)))
		}
		if valPtr != nil {
			cVal = syscall.UTF16ToString((*[4096]uint16)(unsafe.Pointer(valPtr))[:])
			procCoTaskMemFree.Call(uintptr(unsafe.Pointer(valPtr)))
		}

		if cName == "NID_AUT" {
			aut = cVal
		} else if cName == "NID_SES" {
			ses = cVal
		}

		// Release individual cookie object
		syscall.SyscallN(cookieVtable[2], cookiePtr)
	}

	if aut != "" && ses != "" {
		return aut, ses, true
	}
	return aut, ses, false
}

func loginWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_MOVE_WV:
		if activeChromium != nil {
			_ = activeChromium.NotifyParentWindowPositionChanged()
		}
		return 0

	case WM_SIZE_WV:
		if activeChromium != nil {
			activeChromium.Resize()
		}
		return 0

	case WM_TIMER_WV:
		if wParam == TIMER_ID_WV && activeChromium != nil && !cookieCaptured {
			// '로그인 상태 유지' 체크박스(#loginStay) 자동 활성화 보장
			activeChromium.Eval(`(function() {
				try {
					var stay = document.getElementById("loginStay") || 
					           document.getElementById("keep") || 
					           document.querySelector('input[name="nvlong"]');
					if (stay && !stay.checked) {
						stay.click();
						if (!stay.checked) {
							stay.checked = true;
							stay.value = "on";
							stay.setAttribute("aria-checked", "true");
							stay.dispatchEvent(new Event("change", { bubbles: true }));
						}
					}
				} catch(e) {}
			})();`)

			cm, err := activeChromium.GetCookieManager()
			if err == nil && cm != nil {
				handleList := func(listPtr uintptr, err error) {
					if err != nil || listPtr == 0 || cookieCaptured {
						return
					}
					aut, ses, found := inspectCookies(listPtr)
					if found && !cookieCaptured {
						cookieCaptured = true
						SaveConfig(map[string]interface{}{
							"nid_aut": aut,
							"nid_ses": ses,
						})
						LogInfo("[Webview Login] 세션 쿠키 추출 완료 (NID_AUT, NID_SES) -> 자격 증명 관리자에 저장됨.")
						procKillTimer.Call(uintptr(hwnd), TIMER_ID_WV)
						// 안전하게 창 닫기: 메인 스레드에 WM_CLOSE 포스팅
						procPostMessageW.Call(uintptr(hwnd), WM_CLOSE_WV, 0, 0)
					}
				}
				_ = callGetCookies(cm, "https://chzzk.naver.com", handleList)
				if !cookieCaptured {
					_ = callGetCookies(cm, "https://nid.naver.com", handleList)
				}
			}
		}
		return 0

	case WM_CLOSE_WV:
		procKillTimer.Call(uintptr(hwnd), TIMER_ID_WV)
		procDestroyWindow.Call(uintptr(hwnd))
		return 0

	case WM_DESTROY_WV:
		procKillTimer.Call(uintptr(hwnd), TIMER_ID_WV)
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return ret
}

// RunLoginWebview: Windows 공식 WebView2 런타임 및 ICoreWebView2CookieManager 기반 로그인 창 실행
func RunLoginWebview() {
	runtime.LockOSThread()
	LogInfo("[Webview Login] 네이버 로그인 웹뷰 시작 요청됨")

	// [단일 인스턴스 보장]
	mutexNamePtr, _ := syscall.UTF16PtrFromString(`Local\ChzzkObsDock_Login_Mutex`)
	mutexHandle, _, _ := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	lastErr, _, _ := kernel32.NewProc("GetLastError").Call()

	className, _ := syscall.UTF16PtrFromString("ChzzkLoginWindowClass")

	if lastErr == 183 { // ERROR_ALREADY_EXISTS
		LogWarn("[Webview] 이미 로그인 창이 실행 중입니다. 중복 실행을 건너뜁니다.")
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

	cookieCaptured = false

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	// 치지직 로그인 및 공식 리모컨 창이 공유하는 일원화된 브라우저 프로필
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")

	cfg := LoadConfig()
	if cfg.NidAut == "" {
		// 로그아웃 상태이거나 신규 로그인 시, 이전 계정 캐시로 인한 원치 않는 자동 로그인을 차단하되,
		// 정적 리소스(JS/CSS/이미지/V8 캐시)는 보존하여 첫 실행 콜드 스타트 지연을 원천 방지합니다.
		ClearWebViewSession(profileDir)
	}
	_ = os.MkdirAll(profileDir, 0755)

	windowTitle, _ := syscall.UTF16PtrFromString("네이버 로그인 - CHZZK OBS Dock")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hIcon := LoadAppIcon()

	// 눈부신 화이트 플래시(White Flash) 방지: 치지직 다크 테마 배경 브러시 (#18181B)
	darkBrush, _, _ := procCreateSolidBrush.Call(uintptr(0x001B1818))

	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(loginWndProc),
		HInstance:     syscall.Handle(hInst),
		HIcon:         hIcon,
		HCursor:       syscall.Handle(0),
		HbrBackground: syscall.Handle(darkBrush),
		LpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	// 화면 중앙 좌표 계산
	winW, winH := 480, 700
	winX, winY := 150, 150
	screenW, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
	screenH, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
	if screenW > 0 && screenH > 0 {
		winX = (int(screenW) - winW) / 2
		winY = (int(screenH) - winH) / 2
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0x00000008, // WS_EX_TOPMOST
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		0x00CF0000, // WS_OVERLAPPEDWINDOW
		uintptr(winX), uintptr(winY), uintptr(winW), uintptr(winH),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		LogError("[Webview Error] 로그인 윈도우 생성 실패")
		return
	}

	// 타이틀바 및 작업표시줄 아이콘 설정
	if hIcon != 0 {
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_SMALL), uintptr(hIcon))
		procSendMessageW.Call(hwnd, uintptr(WM_SETICON), uintptr(ICON_BIG), uintptr(hIcon))
	}

	applyDarkTheme(hwnd)

	chromium := edge.NewChromium()
	chromium.DataPath = profileDir

	// [OBS 후킹 및 GPU 가속 충돌 방지 핵심 인자]
	chromium.AdditionalBrowserArgs = []string{
		"--disable-gpu",
		"--disable-features=CalculateNativeWinOcclusion",
	}
	activeChromium = chromium

	// 프로세스 실패(크래시) 감지 콜백
	chromium.ProcessFailedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		LogError("[Webview Login] WebView2 렌더러 프로세스 장애 발생. 다시 시도해 주세요.")
	}

	// 페이지 로딩 완료 감지 콜백
	chromium.NavigationCompletedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		src, _ := sender.GetSource()
		LogInfo("[Webview Login] 페이지 로드 완료: %s", src)
		// 네이버 로그인 페이지 접속 시 '로그인 상태 유지' 체크박스 활성화 보장
		if strings.Contains(src, "nidlogin.login") {
			chromium.Eval(`(function() {
				try {
					var stay = document.getElementById("loginStay") || 
					           document.getElementById("keep") || 
					           document.querySelector('input[name="nvlong"]');
					if (stay && !stay.checked) {
						stay.click();
						if (!stay.checked) {
							stay.checked = true;
							stay.value = "on";
							stay.setAttribute("aria-checked", "true");
							stay.dispatchEvent(new Event("change", { bubbles: true }));
						}
					}
				} catch(e) {}
			})();`)
		}
	}

	if !chromium.Embed(hwnd) {
		LogError("[Webview Error] WebView2 임베딩 실패 (Microsoft Edge WebView2 Runtime이 설치되어 있는지 확인하세요)")
		procDestroyWindow.Call(hwnd)
		return
	}

	// [로그인 상태 유지 자동화] NID_AUT 장기 쿠키 발급을 위한 로그인 상태 유지 체크박스(#loginStay) 자동 활성화 스크립트 등록
	keepLoginScript := `
	(function() {
		function autoCheckKeep() {
			try {
				var stay = document.getElementById("loginStay") || 
				           document.getElementById("keep") || 
				           document.querySelector('input[name="nvlong"]');
				if (stay && !stay.checked) {
					stay.click();
					if (!stay.checked) {
						stay.checked = true;
						stay.value = "on";
						stay.setAttribute("aria-checked", "true");
						stay.dispatchEvent(new Event("change", { bubbles: true }));
					}
				}
			} catch(e) {}
		}
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', autoCheckKeep);
		} else {
			autoCheckKeep();
		}
		var attempts = 0;
		var timer = setInterval(function() {
			autoCheckKeep();
			attempts++;
			if (attempts > 30) clearInterval(timer);
		}, 100);
	})();
	`
	chromium.Init(keepLoginScript)

	LogInfo("[Webview Login] 로그인 페이지로 이동: %s", LOGIN_URL)
	chromium.Navigate(LOGIN_URL)

	// 컨트롤러 가시성 보장 및 포커스 부여 후 부드럽게 윈도우 활성화
	_ = chromium.Show()
	chromium.Focus()
	chromium.Resize()
	ForceForegroundWindow(hwnd, true)

	// 1초마다 로그인 완료 쿠키 감지 타이머 가동
	procSetTimer.Call(hwnd, TIMER_ID_WV, 1000, 0)

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
	LogInfo("[Webview Login] 로그인 윈도우 루프 종료")
}

