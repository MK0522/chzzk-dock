package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

	procShowWindow       = user32.NewProc("ShowWindow")
	procUpdateWindow     = user32.NewProc("UpdateWindow")
	procSetTimer         = user32.NewProc("SetTimer")
	procKillTimer        = user32.NewProc("KillTimer")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
)

const (
	WM_SIZE_WV    = 0x0005
	WM_DESTROY_WV = 0x0002
	WM_TIMER_WV   = 0x0113
	TIMER_ID_WV   = 2001
)

var (
	activeChromium *edge.Chromium
	cookieCaptured = false
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
	case WM_SIZE_WV:
		if activeChromium != nil {
			activeChromium.Resize()
		}
		return 0

	case WM_TIMER_WV:
		if wParam == TIMER_ID_WV && activeChromium != nil && !cookieCaptured {
			cm, err := activeChromium.GetCookieManager()
			if err == nil && cm != nil {
				_ = callGetCookies(cm, "https://chzzk.naver.com", func(listPtr uintptr, err error) {
					if err != nil || listPtr == 0 {
						return
					}
					aut, ses, found := inspectCookies(listPtr)
					if found && !cookieCaptured {
						cookieCaptured = true
						SaveConfig(map[string]interface{}{
							"nid_aut": aut,
							"nid_ses": ses,
						})
						fmt.Println("[Webview Login] 세션 쿠키 추출 완료 (NID_AUT, NID_SES) -> 자격 증명 관리자에 저장됨.")
						procKillTimer.Call(uintptr(hwnd), TIMER_ID_WV)
						procDestroyWindow.Call(uintptr(hwnd))
					}
				})
			}
		}
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

	// [단일 인스턴스 보장]
	mutexNamePtr, _ := syscall.UTF16PtrFromString(`Local\ChzzkObsDock_Login_Mutex`)
	mutexHandle, _, _ := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	lastErr, _, _ := kernel32.NewProc("GetLastError").Call()

	if lastErr == 183 { // ERROR_ALREADY_EXISTS
		fmt.Println("[Webview] 이미 로그인 창이 실행 중입니다. 중복 실행을 건너뜁니다.")
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
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview2_profile")
	_ = os.MkdirAll(profileDir, 0755)

	className, _ := syscall.UTF16PtrFromString("ChzzkLoginWindowClass")
	windowTitle, _ := syscall.UTF16PtrFromString("네이버 로그인 - CHZZK OBS Dock")

	hInst, _, _ := procGetModuleHandleW.Call(0)

	wc := WNDCLASSW{
		Style:         0x0002 | 0x0001, // CS_HREDRAW | CS_VREDRAW
		LpfnWndProc:   syscall.NewCallback(loginWndProc),
		HInstance:     syscall.Handle(hInst),
		HCursor:       syscall.Handle(0),
		LpszClassName: className,
	}
	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	// 480 x 680 중앙 배치 윈도우 생성
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowTitle)),
		0x00CF0000, // WS_OVERLAPPEDWINDOW
		150, 150, 480, 680,
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		fmt.Println("[Webview Error] 로그인 윈도우 생성 실패")
		return
	}

	chromium := edge.NewChromium()
	chromium.DataPath = profileDir
	activeChromium = chromium

	if !chromium.Embed(hwnd) {
		fmt.Println("[Webview Error] WebView2 임베딩 실패 (WebView2 Runtime이 설치되어 있는지 확인하세요)")
		procDestroyWindow.Call(hwnd)
		return
	}

	procShowWindow.Call(hwnd, 5) // SW_SHOW
	procUpdateWindow.Call(hwnd)

	chromium.Resize()
	chromium.Navigate(LOGIN_URL)

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

	time.Sleep(300 * time.Millisecond)
}

