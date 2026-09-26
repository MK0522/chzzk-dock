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

const chatOverlayScript = `
(function() {
  if (window.__chzzkChatOverlayInjected) return;
  window.__chzzkChatOverlayInjected = true;

  function initOverlay() {
    if (document.getElementById('chzzk-floating-toolbar')) return;

    var bar = document.createElement('div');
    bar.id = 'chzzk-floating-toolbar';

    // 이전 저장 위치 복원 (localStorage: 채팅창 전용 키)
    var savedPos = null;
    try {
      var raw = localStorage.getItem('chzzk_chat_pin_pos');
      if (raw) savedPos = JSON.parse(raw);
    } catch(e) {}

    // 기본 위치: 좌측 상단 (치지직 우측 상단 공식 메뉴 가림 방지)
    var initialTop = (savedPos && typeof savedPos.top === 'number') ? savedPos.top + 'px' : '10px';
    var initialLeft = (savedPos && typeof savedPos.left === 'number') ? savedPos.left + 'px' : '14px';

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
        'transition: background 0.15s, border-color 0.15s, color 0.15s, box-shadow 0.15s !important',
        'box-shadow: 0 4px 10px rgba(0, 0, 0, 0.35) !important'
      ].join(';');

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
      return btn;
    }

    var reloadBtn = createBtn('chzzk-floating-reload-btn', '🔄', '새로고침 (드래그하여 위치 이동)');
    var pinBtn = createBtn('chzzk-floating-pin-btn', '📌', '항상 위에 고정 (드래그하여 위치 이동)');

    bar.appendChild(reloadBtn);
    bar.appendChild(pinBtn);

    var isDragging = false;
    var hasMoved = false;
    var startX = 0, startY = 0;
    var origLeft = 0, origTop = 0;

    // --- 툴바 드래그 이동 핸들러 ---
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

      var maxLeft = window.innerWidth - bar.offsetWidth;
      var maxTop = window.innerHeight - bar.offsetHeight;
      newLeft = Math.max(0, Math.min(maxLeft, newLeft));
      newTop = Math.max(0, Math.min(maxTop, newTop));

      bar.style.left = newLeft + 'px';
      bar.style.top = newTop + 'px';
      bar.style.right = 'auto';
    });

    document.addEventListener('mouseup', function(e) {
      if (!isDragging) return;
      isDragging = false;
      reloadBtn.style.cursor = 'grab';
      pinBtn.style.cursor = 'grab';

      if (hasMoved) {
        try {
          var rect = bar.getBoundingClientRect();
          localStorage.setItem('chzzk_chat_pin_pos', JSON.stringify({
            left: rect.left,
            top: rect.top
          }));
        } catch(err) {}
      }
    });

    // 개별 버튼 클릭 핸들러 (드래그 이동 시 실행 방지)
    reloadBtn.addEventListener('click', function(e) {
      e.stopPropagation();
      e.preventDefault();
      if (hasMoved) {
        hasMoved = false;
        return;
      }
      location.reload();
    });

    pinBtn.addEventListener('click', function(e) {
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

    document.documentElement.appendChild(bar);

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

  // [외부 링크 가드] Toast UI 생성 및 표시
  window.__showToast = function(msg) {
    var toast = document.getElementById('chzzk-dock-toast');
    if (!toast) {
      toast = document.createElement('div');
      toast.id = 'chzzk-dock-toast';
      toast.style.cssText = [
        'position: fixed !important',
        'bottom: 18px !important',
        'left: 50% !important',
        'transform: translateX(-50%) translateY(20px) !important',
        'background: rgba(15, 23, 42, 0.95) !important',
        'border: 1.5px solid #00FFA3 !important',
        'color: #F8FAFC !important',
        'padding: 8px 16px !important',
        'border-radius: 8px !important',
        'font-size: 12px !important',
        'font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif !important',
        'font-weight: 600 !important',
        'box-shadow: 0 8px 24px rgba(0, 0, 0, 0.6) !important',
        'z-index: 10000000 !important',
        'pointer-events: none !important',
        'transition: opacity 0.25s ease, transform 0.25s ease !important',
        'opacity: 0 !important',
        'white-space: nowrap !important'
      ].join(';');
      document.documentElement.appendChild(toast);
    }
    toast.textContent = msg;
    toast.style.opacity = '1';
    toast.style.transform = 'translateX(-50%) translateY(0)';
    if (window.__toastTimer) clearTimeout(window.__toastTimer);
    window.__toastTimer = setTimeout(function() {
      toast.style.opacity = '0';
      toast.style.transform = 'translateX(-50%) translateY(20px)';
    }, 2500);
  };

  // [외부 링크 가드] 네이버 외 외부 도메인 판별
  function isInternalHost(hostname) {
    if (!hostname) return true;
    var h = hostname.toLowerCase();
    return h === 'naver.com' || h.endsWith('.naver.com');
  }

  // [외부 링크 가드] <a> 클릭 이벤트 캡처 가로채기
  document.addEventListener('click', function(e) {
    var target = e.target;
    while (target && target.tagName !== 'A') {
      target = target.parentElement;
    }
    if (!target || !target.href) return;

    var url;
    try {
      url = new URL(target.href);
    } catch(err) {
      return;
    }

    if (url.protocol !== 'http:' && url.protocol !== 'https:') return;

    var isBlank = (target.target === '_blank');
    var isExternal = !isInternalHost(url.hostname);

    if (isBlank || isExternal) {
      e.preventDefault();
      e.stopPropagation();
      if (window.chrome && window.chrome.webview) {
        window.chrome.webview.postMessage('open-external:' + target.href);
      }
    }
  }, true);

  // [외부 링크 가드] window.open 가로채기
  var originalWindowOpen = window.open;
  window.open = function(url, target, features) {
    if (url) {
      try {
        var parsed = new URL(url, location.href);
        if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
          if (!isInternalHost(parsed.hostname) || target === '_blank') {
            if (window.chrome && window.chrome.webview) {
              window.chrome.webview.postMessage('open-external:' + parsed.href);
              return null;
            }
          }
        }
      } catch(e) {}
    }
    return originalWindowOpen.apply(this, arguments);
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initOverlay);
  } else {
    initOverlay();
  }
})();
`

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

// InjectChatCookies: WebView2 CookieManager에 .naver.com 단일 도메인으로 최신 쿠키 주입
func InjectChatCookies(cm *edge.ICoreWebView2CookieManager, aut, ses string) {
	if cm == nil || aut == "" || ses == "" {
		return
	}
	dom := ".naver.com"
	if cookie, err := cm.CreateCookie("NID_AUT", aut, dom, "/"); err == nil && cookie != nil {
		_ = cookie.PutIsSecure(true)
		_ = cookie.PutIsHttpOnly(true)
		_ = cm.AddOrUpdateCookie(cookie)
		cookie.Release()
	}
	if cookie, err := cm.CreateCookie("NID_SES", ses, dom, "/"); err == nil && cookie != nil {
		_ = cookie.PutIsSecure(true)
		_ = cookie.PutIsHttpOnly(true)
		_ = cm.AddOrUpdateCookie(cookie)
		cookie.Release()
	}
	LogInfo("[Chat Webview] 네이버 인증 쿠키(.naver.com)를 WebView2에 주입 완료.")
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
		chromium.Eval(chatOverlayScript)
		chromium.Eval(fmt.Sprintf("if (window.__updateTopmostUI) window.__updateTopmostUI(%t);", isChatTopmost))

		// [Outbound Sync] 웹뷰 브라우저 내부에서 네이버가 새 세션을 갱신한 경우 시스템 금고에 자동 역동기화
		if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
			_ = callGetCookies(cm, "https://chzzk.naver.com", func(listPtr uintptr, err error) {
				if err != nil || listPtr == 0 {
					return
				}
				aut, ses, found := inspectCookies(listPtr)
				if found && aut != "" && ses != "" {
					latestCfg := LoadConfig()
					if latestCfg.NidSes != ses || latestCfg.NidAut != aut {
						SaveConfig(map[string]interface{}{
							"nid_aut": aut,
							"nid_ses": ses,
						})
						LogInfo("[Chat Webview] 웹뷰 브라우저에서 갱신된 최신 네이버 세션 쿠키를 시스템 금고에 자동 역저장 완료.")
					}
				}
			})
		}
	}

	if !chromium.Embed(hwnd) {
		LogError("[Chat Webview Error] WebView2 임베딩 실패 (WebView2 Runtime 확인 필요)")
		procDestroyWindow.Call(hwnd)
		return
	}

	chromium.SetBackgroundColour(0x0B, 0x0E, 0x11, 255)
	chromium.Init(chatOverlayScript)

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

	// [Inbound Sync] Windows 시스템 금고의 최신 세션 쿠키를 .naver.com 단일 도메인으로 상시 주입
	cfg := LoadConfig()
	if cfg.NidAut != "" && cfg.NidSes != "" {
		if cm, err := chromium.GetCookieManager(); err == nil && cm != nil {
			InjectChatCookies(cm, cfg.NidAut, cfg.NidSes)
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
