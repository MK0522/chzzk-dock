package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	// URL 인코딩
	// %3A :
	// %2F /

	// ExternalLinkGuardScript: 네이버 및 로컬 호스트 외 외부 도메인 클릭 시 기본 브라우저로 위임하는 스크립트
	ExternalLinkGuardScript = `
	(function() {
		function isInternalHost(hostname) {
			if (!hostname) return true;
			var h = hostname.toLowerCase();
			return h === 'naver.com' || h.endsWith('.naver.com') || h === '127.0.0.1' || h === 'localhost';
		}
		document.addEventListener('click', function(e) {
			var target = e.target;
			while (target && target.tagName !== 'A') {
				target = target.parentElement;
			}
			if (!target || !target.href) return;
			var url;
			try { url = new URL(target.href); } catch(err) { return; }
			if (url.protocol !== 'http:' && url.protocol !== 'https:') return;
			if (target.target === '_blank' || !isInternalHost(url.hostname)) {
				e.preventDefault();
				e.stopPropagation();
				if (window.chrome && window.chrome.webview) {
					window.chrome.webview.postMessage('open-external:' + target.href);
				}
			}
		}, true);
		var origOpen = window.open;
		window.open = function(url, target, feat) {
			if (url) {
				try {
					var p = new URL(url, location.href);
					if (p.protocol === 'http:' || p.protocol === 'https:') {
						if (!isInternalHost(p.hostname) || target === '_blank') {
							if (window.chrome && window.chrome.webview) {
								window.chrome.webview.postMessage('open-external:' + p.href);
								return null;
							}
						}
					}
				} catch(e) {}
			}
			return origOpen.apply(this, arguments);
		};
	})();
	`
)

var (
	// [OLE32: COM 메모리 관리 도서관]
	ole32DLL = syscall.NewLazyDLL("ole32.dll")

	// procCoTaskMemFree: WebView2 C++ 엔진이 쿠키 문자열을 건네줄 때 빌려준 C++ 힙 메모리를
	// 안전하게 반납(해제)하여 메모리 누수를 영구 차단하는 함수
	procCoTaskMemFree = ole32DLL.NewProc("CoTaskMemFree")

	// [USER32: 윈도우 창 및 타이머 제어 함수들]
	// procShowWindow: 윈도우 창을 화면에 표시(SW_SHOW)하거나 숨김(SW_HIDE) 처리
	procShowWindow = user32.NewProc("ShowWindow")

	// procUpdateWindow: 창의 클라이언트 영역을 즉시 새로고침하여 잔상 없이 화면 갱신
	procUpdateWindow = user32.NewProc("UpdateWindow")

	// procSetTimer: 로그인 완료 여부를 주기적으로 감시(Polling)하기 위한 Win32 타이머 등록
	procSetTimer = user32.NewProc("SetTimer")

	// procKillTimer: 로그인이 끝나거나 창이 닫힐 때 타이머를 해제하여 CPU 낭비 방어
	procKillTimer = user32.NewProc("KillTimer")

	// procPostQuitMessage: 로그인 창이 닫힐 때 Win32 메시지 루프에 종료 신호(WM_QUIT)를 전달
	procPostQuitMessage = user32.NewProc("PostQuitMessage")

	// procPeekMessageW: 동기 쿠키 정리 배리어 대기 중 Win32/COM 메시지 펌핑에 사용
	procPeekMessageW = user32.NewProc("PeekMessageW")
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
	activeChromium               *edge.Chromium
	cookieCaptured               = false
	closeOnce                    sync.Once
	loginURL                     = LOGIN_URL
	onLoginWebviewStartedForTest func(*edge.Chromium)
)



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

func _iCoreWebView2GetCookiesCompletedHandlerInvoke(this uintptr, errorCode uintptr, result uintptr) uintptr {
	res := combridge.Resolve[iCoreWebView2GetCookiesCompletedHandler](this).Invoke(int32(errorCode), result)
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
	defer comPtr.Close()

	vtablePtr := *(*uintptr)(unsafe.Pointer(cm))
	vtable := (*[10]uintptr)(unsafe.Pointer(vtablePtr))
	getCookiesProc := vtable[5]

	hr, _, _ := syscall.SyscallN(
		getCookiesProc,
		uintptr(unsafe.Pointer(cm)),
		uintptr(unsafe.Pointer(uriUTF16)),
		comPtr.Ref(),
	)
	runtime.KeepAlive(uriUTF16)
	runtime.KeepAlive(comPtr)
	if hr != 0 {
		return syscall.Errno(hr)
	}
	return nil
}

// purgeSessionCookies deletes NID_AUT and NID_SES across naver/chzzk domains while preserving 2FA tokens like NID_JST.
func purgeSessionCookies(cm *edge.ICoreWebView2CookieManager) {
	if cm == nil {
		return
	}
	targets := []string{
		"https://naver.com",
		"https://nid.naver.com",
		"https://chzzk.naver.com",
		"https://studio.chzzk.naver.com",
	}
	for _, target := range targets {
		_ = cm.DeleteCookies("NID_AUT", target)
		_ = cm.DeleteCookies("NID_SES", target)
	}
}

// forEachCookieInList iterates over an ICoreWebView2CookieList [in] pointer without releasing listPtr itself,
// releasing each individual cookie COM object after invoking fn.
func forEachCookieInList(listPtr uintptr, fn func(name, val string, cookiePtr uintptr)) {
	if listPtr == 0 {
		return
	}
	vtablePtr := *(*uintptr)(unsafe.Pointer(listPtr))
	vtable := (*[10]uintptr)(unsafe.Pointer(vtablePtr))

	var count uint32
	hr, _, _ := syscall.SyscallN(vtable[3], listPtr, uintptr(unsafe.Pointer(&count)))
	if hr != 0 || count == 0 {
		return
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

		fn(cName, cVal, cookiePtr)

		// Release individual cookie object retrieved via GetValueAtIndex
		syscall.SyscallN(cookieVtable[2], cookiePtr)
	}
}

// deleteSessionCookiesInList deletes NID_AUT and NID_SES cookie objects from listPtr while preserving 2FA tokens (e.g., NID_JST).
func deleteSessionCookiesInList(cm *edge.ICoreWebView2CookieManager, listPtr uintptr) int {
	if cm == nil || listPtr == 0 {
		return 0
	}
	deleted := 0
	forEachCookieInList(listPtr, func(cName, _ string, cookiePtr uintptr) {
		if cName == "NID_AUT" || cName == "NID_SES" {
			if err := cm.DeleteCookie((*edge.ICoreWebView2Cookie)(unsafe.Pointer(cookiePtr))); err == nil {
				deleted++
			}
		}
	})
	return deleted
}

// purgeSessionCookiesSync clears NID_AUT/NID_SES both by URI and by direct cookie object deletion,
// pumping the UI message loop until a verification GetCookies barrier completes or timeout elapses.
func purgeSessionCookiesSync(cm *edge.ICoreWebView2CookieManager, timeout time.Duration) {
	if cm == nil {
		return
	}
	purgeSessionCookies(cm)

	deleteTargets := []string{
		"https://chzzk.naver.com",
		"https://nid.naver.com",
		"https://studio.chzzk.naver.com",
	}
	verifyTargets := []string{
		"https://chzzk.naver.com",
		"https://nid.naver.com",
	}

	done := false
	phase2Pending := len(verifyTargets)
	onVerify := func(listPtr uintptr, err error) {
		if err == nil && listPtr != 0 {
			deleteSessionCookiesInList(cm, listPtr)
		}
		phase2Pending--
		if phase2Pending <= 0 {
			done = true
		}
	}

	startVerify := func() {
		for _, target := range verifyTargets {
			if err := callGetCookies(cm, target, onVerify); err != nil {
				phase2Pending--
			}
		}
		if phase2Pending <= 0 {
			done = true
		}
	}

	phase1Pending := len(deleteTargets)
	onDeleteList := func(listPtr uintptr, err error) {
		if err == nil && listPtr != 0 {
			deleteSessionCookiesInList(cm, listPtr)
		}
		phase1Pending--
		if phase1Pending <= 0 {
			startVerify()
		}
	}

	for _, target := range deleteTargets {
		if err := callGetCookies(cm, target, onDeleteList); err != nil {
			phase1Pending--
		}
	}
	if phase1Pending <= 0 {
		startVerify()
	}

	const (
		pmRemove = 0x0001
		wmQuit   = 0x0012
	)
	var m MSG
	deadline := time.Now().Add(timeout)
	for !done && time.Now().Before(deadline) {
		drained := false
		for {
			ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if ret == 0 {
				break
			}
			drained = true
			if m.Message == wmQuit {
				procPostQuitMessage.Call(m.WParam)
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		if !done && !drained {
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func inspectCookies(listPtr uintptr) (aut, ses string, found bool) {
	forEachCookieInList(listPtr, func(cName, cVal string, _ uintptr) {
		switch cName {
		case "NID_AUT":
			aut = cVal
		case "NID_SES":
			ses = cVal
		}
	})

	if aut != "" && ses != "" {
		return aut, ses, true
	}

	return aut, ses, false
}

const autoCheckKeepScript = `(function() {
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
})();`

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
			activeChromium.Eval(autoCheckKeepScript)

			cm, err := activeChromium.GetCookieManager()
			if err == nil && cm != nil {
				defer cm.Release()
				handleList := func(listPtr uintptr, err error) {
					if err != nil || listPtr == 0 || cookieCaptured {
						return
					}
					// [HOTFIX-06-01] ICoreWebView2CookieList*는 Invoke의 [in] 파라미터로 WebView2 런타임이 소유하므로
					// 절대 IUnknown::Release를 호출하지 않는다 (호출 시 이중 해제로 0xC0000005 크래시 발생).
					aut, ses, found := inspectCookies(listPtr)
					if found && !cookieCaptured {
						cookieCaptured = true
						procKillTimer.Call(uintptr(hwnd), TIMER_ID_WV)

						postClose := func() {
							closeOnce.Do(func() {
								procPostMessageW.Call(uintptr(hwnd), WM_CLOSE_WV, 0, 0)
							})
						}

						saved := SaveConfig(map[string]interface{}{
							"nid_aut": aut,
							"nid_ses": ses,
						})
						if saved.NidAut == "" || saved.NidSes == "" {
							LogError("[Webview Login] 추출한 세션 쿠키 저장 실패 (Windows 자격 증명 관리자 오류)")
							fmt.Fprintf(os.Stderr, "[Webview Login] 추출한 세션 쿠키 저장 실패 (Windows 자격 증명 관리자 오류)\n")
							postClose()
							return
						}
						LogInfo("[Webview Login] 세션 쿠키 추출 완료 (NID_AUT, NID_SES) -> 자격 증명 관리자에 저장됨.")

						// [시점 A] 백엔드 저장 완료 후 웹뷰 프로필에서 세션 쿠키만 선별 파기하고(2차 인증 NID_JST 보존),
						// 삭제 IPC가 반영된 것을 GetCookies 배리어로 확인한 뒤 창을 닫아 다음 로그인 시 잔류 쿠키 재추출을 방지.
						cm2, cmErr := activeChromium.GetCookieManager()
						if cmErr != nil || cm2 == nil {
							postClose()
							return
						}
						var releaseCm2Once sync.Once
						releaseCm2 := func() {
							releaseCm2Once.Do(func() {
								cm2.Release()
							})
						}
						deleteSessionCookiesInList(cm2, listPtr)
						purgeSessionCookies(cm2)

						fallbackTimer := time.AfterFunc(1500*time.Millisecond, postClose)
						cleanupPending := 2
						onCleanupBarrier := func(bListPtr uintptr, bErr error) {
							if bErr == nil && bListPtr != 0 {
								deleteSessionCookiesInList(cm2, bListPtr)
							}
							cleanupPending--
							if cleanupPending <= 0 {
								fallbackTimer.Stop()
								releaseCm2()
								postClose()
							}
						}
						if err1 := callGetCookies(cm2, "https://nid.naver.com", onCleanupBarrier); err1 != nil {
							cleanupPending--
						}
						if err2 := callGetCookies(cm2, "https://chzzk.naver.com", onCleanupBarrier); err2 != nil {
							cleanupPending--
						}
						if cleanupPending <= 0 {
							fallbackTimer.Stop()
							releaseCm2()
							postClose()
						}
					}
				}
				_ = callGetCookies(cm, "https://chzzk.naver.com", handleList)
				if !cookieCaptured {
					_ = callGetCookies(cm, "https://nid.naver.com", handleList)
				}
			}
		}
		return 0

	case 0x0011 /* WM_QUERYENDSESSION */:
		return 1

	case 0x0016 /* WM_ENDSESSION */:
		if wParam != 0 {
			procKillTimer.Call(uintptr(hwnd), TIMER_ID_WV)
			procPostQuitMessage.Call(0)
			os.Exit(0)
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
	mutexHandle, _, errCall := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	errno, isErrno := errCall.(syscall.Errno)

	className, _ := syscall.UTF16PtrFromString("ChzzkLoginWindowClass")

	// 183: ERROR_ALREADY_EXISTS, 5: ERROR_ACCESS_DENIED
	// [Ponytail/YAGNI] 새 의존성 없이 기존 IsProcessRunning으로 2차 검증하여 안전하게 중복 판단
	if isErrno && (errno == 183 || (errno == 5 && IsProcessRunning("chzzk-dock.exe"))) {
		LogWarn("[Webview] [%s] 이미 로그인 창이 실행 중입니다. 중복 실행을 건너뜁니다.", ErrAuthDuplicateLogin)
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
	closeOnce = sync.Once{}

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	// 치지직 로그인 및 공식 리모컨 창이 공유하는 일원화된 브라우저 프로필
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")

	_ = os.MkdirAll(profileDir, 0755)

	windowTitle, _ := syscall.UTF16PtrFromString("네이버 로그인 - CHZZK OBS Dock")
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hIcon := LoadAppIcon()

	// 눈부신 화이트 플래시(White Flash) 방지: 치지직 다크 테마 배경 브러시 (#18181B)
	darkBrush, _, _ := procCreateSolidBrush.Call(uintptr(0x001B1818))

	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001,                   // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(loginWndProc), // 윈도우 메시지 처리 함수 콜백 등록
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
		LogError("[Webview Error] [%s] 로그인 윈도우 생성 실패", ErrSysFileIoFailed)
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
	chromium.AdditionalBrowserArgs = GetSharedBrowserArgs()
	activeChromium = chromium

	var embedFailed bool
	var embedErr error
	chromium.SetErrorCallback(func(err error) {
		LogError("[Login Webview] WebView2 초기화 에러 콜백: %v", err)
		embedFailed = true
		embedErr = err
		procPostMessageW.Call(hwnd, WM_CLOSE_WV, 0, 0)
	})

	// 프로세스 실패(크래시) 감지 콜백
	chromium.ProcessFailedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
		LogError("[Webview Login] [%s] WebView2 렌더러 프로세스 장애 발생. 다시 시도해 주세요.", ErrSysWebviewRuntime)
	}

	// 페이지 로딩 완료 감지 콜백
	chromium.NavigationCompletedCallback = func(sender *edge.ICoreWebView2, args *edge.ICoreWebView2NavigationCompletedEventArgs) {
		src, _ := sender.GetSource()
		LogInfo("[Webview Login] 페이지 로드 완료: %s", src)
	}

	if !chromium.Embed(hwnd) || embedFailed {
		LogError("[Webview Error] [%s] WebView2 임베딩 실패 (에러: %v)", ErrSysWebviewRuntime, embedErr)
		user32 := syscall.NewLazyDLL("user32.dll")
		titlePtr, _ := syscall.UTF16PtrFromString("네이버 로그인 - CHZZK OBS Dock")
		errMsg := fmt.Sprintf("네이버 로그인 웹뷰 초기화에 실패했습니다.\n\n오류: %v\n\n잠시 후 다시 시도해 주세요.", embedErr)
		if embedErr == nil {
			errMsg = "네이버 로그인 웹뷰 초기화에 실패했습니다.\n\n잠시 후 다시 시도해 주세요."
		}
		msgPtr, _ := syscall.UTF16PtrFromString(errMsg)
		user32.NewProc("MessageBoxW").Call(hwnd, uintptr(unsafe.Pointer(msgPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x10)
		procDestroyWindow.Call(hwnd)
		os.Exit(1)
	}

	// [시점 B] 웹뷰 임베딩 직후 이전 세션 쿠키 선별 삭제 및 반영 완료 대기 (기기 기억 토큰 NID_JST 등은 보존)
	if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
		purgeSessionCookiesSync(cm, 2*time.Second)
		cm.Release()
	}

	chromium.MessageCallback = func(message string, sender *edge.ICoreWebView2, args *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		if strings.HasPrefix(message, "open-external:") {
			targetURL := strings.TrimPrefix(message, "open-external:")
			if GetExternalBrowserGuard() {
				OpenBrowser(targetURL)
			}
		}
	}

	// [로그인 상태 유지 자동화] NID_AUT 장기 쿠키 발급 및 외부 링크 기본 브라우저 열기 가드
	keepLoginScript := `(function() {
		function autoCheckKeep() { ` + autoCheckKeepScript + ` }
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', autoCheckKeep);
		} else {
			autoCheckKeep();
		}
		var attempts = 0;
		var timer = setInterval(function() {
			autoCheckKeep();
			if (++attempts > 30) clearInterval(timer);
		}, 100);
	})();`
	chromium.Init(keepLoginScript)
	chromium.Init(ExternalLinkGuardScript)

	LogInfo("[Webview Login] 로그인 페이지로 이동: %s", loginURL)
	chromium.Navigate(loginURL)

	// 컨트롤러 가시성 보장 및 포커스 부여 후 부드럽게 윈도우 활성화
	_ = chromium.Show()
	chromium.Focus()
	chromium.Resize()
	ForceForegroundWindow(hwnd, true)

	// 1초마다 로그인 완료 쿠키 감지 타이머 가동
	procSetTimer.Call(hwnd, TIMER_ID_WV, 1000, 0)
	if onLoginWebviewStartedForTest != nil {
		onLoginWebviewStartedForTest(chromium)
	}

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
