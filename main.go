package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"chzzk-obs-dock/core"
)

//go:embed chzzk-obs-dock.html
var embeddedHTML []byte

//go:embed icon.ico
var embeddedIcon []byte

// ============================================================
//  CHZZK OBS Dock Server v0.4.1 (Modular Architecture)
// ============================================================
const (
	APP_VERSION = "v0.4.1"
	HTTP_PORT   = 8081
	USER_AGENT  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	webviewProcess *exec.Cmd
	webviewLock    sync.Mutex
	trayInstance   *core.PureWinTrayIcon
	httpClient     = &http.Client{Timeout: 10 * time.Second}
)

func sendBytes(w http.ResponseWriter, body []byte, status int, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// OpenURL: 시스템 기본 브라우저로 입력된 URL을 엽니다.
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		// Windows cmd.exe의 start 명령은 첫 번째 따옴표 인자를 창 제목으로 취급하므로
		// 반드시 빈 타이틀("")을 첫 번째 인자로 전달해야 URL이 실행됩니다.
		escaped := strings.ReplaceAll(url, "&", "^&")
		cmd := exec.Command("cmd", "/c", "start", "", escaped)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := cmd.Start(); err == nil {
			return nil
		}
		// 폴백: PowerShell Start-Process
		psCmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", fmt.Sprintf("Start-Process '%s'", strings.ReplaceAll(url, "'", "''")))
		psCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return psCmd.Start()
	case "darwin":
		// macOS: open 주소
		cmd := exec.Command("open", url)
		return cmd.Start()
	case "linux":
		// Linux: xdg-open 주소
		cmd := exec.Command("xdg-open", url)
		return cmd.Start()
	default:
		return fmt.Errorf("지원하지 않는 운영체제입니다: %s", runtime.GOOS)
	}
}

// setCORSHeaders: [SEC-01] CORS Origin 엄격한 화이트리스트 제한
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowedOrigins := map[string]bool{
		fmt.Sprintf("http://localhost:%d", HTTP_PORT): true,
		fmt.Sprintf("http://127.0.0.1:%d", HTTP_PORT): true,
	}
	if allowedOrigins[origin] {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
	}
}

// isPublicEndpoint: 쿠키 값 없이 작동 가능한 공개 API 경로인지 판별
func isPublicEndpoint(path, customURL string) bool {
	target := path
	if customURL != "" {
		target = customURL
	}
	return strings.Contains(target, "/auto-complete/") || strings.Contains(target, "/service/")
}

// proxyUnofficialRequest: 치지직 비공식 API 프록시 (Rate Limiter 및 세션 헤더 포함)
func proxyUnofficialRequest(w http.ResponseWriter, r *http.Request, method, path string, body []byte, customURL string) {
	isPublic := isPublicEndpoint(path, customURL)

	cfg := core.LoadConfig()
	nidAut := cfg.NidAut
	nidSes := cfg.NidSes

	// 쿠키가 필요한 엔드포인트인 경우에만 로그인 쿠키 검증
	if !isPublic && (nidAut == "" || nidSes == "") {
		sendJSON(w, map[string]interface{}{
			"code":    401,
			"message": "네이버 로그인 쿠키가 설정되지 않았습니다. 독 설정에서 로그인하세요.",
		}, http.StatusUnauthorized)
		return
	}

	var targetURL string
	if customURL != "" {
		targetURL = customURL
	} else if strings.HasPrefix(path, "/manage/") || strings.HasPrefix(path, "/service/") {
		targetURL = "https://api.chzzk.naver.com" + path
	} else {
		targetURL = "https://api.chzzk.naver.com/manage/v1" + path
	}

	// [LEG-01] 3초 Rate Limiting 인메모리 캐시 조회
	if cached, found := core.GetCachedApiResponse(method, targetURL); found {
		sendBytes(w, cached.Body, cached.Status, cached.ContentType)
		return
	}

	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = strings.NewReader(string(body))
	}

	req, err := http.NewRequest(method, targetURL, bodyReader)
	if err != nil {
		sendJSON(w, map[string]interface{}{"code": 502, "message": "프록시 요청 생성 실패"}, http.StatusBadGateway)
		return
	}

	req.Header.Set("User-Agent", USER_AGENT)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	// 쿠키 값 없이 작동 가능한 엔드포인트는 Cookie, Origin, Referer 등 불필요한 헤더를 첨부하지 않음
	if !isPublic && nidAut != "" && nidSes != "" {
		req.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", nidAut, nidSes))
		req.Header.Set("Origin", "https://chzzk.naver.com")
		req.Header.Set("Referer", "https://chzzk.naver.com/")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		sendJSON(w, map[string]interface{}{"code": 502, "message": "프록시 요청 실패"}, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		sendJSON(w, map[string]interface{}{"code": 502, "message": "프록시 응답 읽기 실패"}, http.StatusBadGateway)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	core.SetCachedApiResponse(method, targetURL, respBytes, resp.StatusCode, contentType)
	sendBytes(w, respBytes, resp.StatusCode, contentType)
}

func proxyDispatch(w http.ResponseWriter, r *http.Request, method string) bool {
	path := r.URL.Path
	if strings.HasPrefix(path, "/unofficial/") {
		target := strings.TrimPrefix(path, "/unofficial")
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		proxyUnofficialRequest(w, r, method, target, body, "")
		return true
	}
	return false
}

// HttpDockHandler: 메인 HTTP 라우터 핸들러
func HttpDockHandler(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w, r)

	// OPTIONS 프리플라이트 요청 처리
	if r.Method == http.MethodOptions {
		if !core.CheckSecurity(w, r) {
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	path := r.URL.Path

	switch r.Method {
	case http.MethodGet:
		// API 엔드포인트 라우팅
		apiPaths := map[string]bool{
			"/config":          true,
			"/login-webview":   true,
			"/login-wait":      true,
			"/unofficial-user": true,
			"/open-browser":    true,
		}

		if apiPaths[path] || strings.HasPrefix(path, "/unofficial/") {
			if !core.CheckApiAuth(w, r) {
				return
			}

			if path == "/config" {
				sendJSON(w, core.LoadConfig(), http.StatusOK)
				return
			}

			if path == "/login-webview" {
				webviewLock.Lock()
				if webviewProcess != nil && webviewProcess.ProcessState == nil {
					webviewLock.Unlock()
					sendJSON(w, map[string]interface{}{
						"status":  "already_open",
						"message": "이미 네이버 로그인 창이 열려 있습니다.",
					}, http.StatusOK)
					return
				}

				exePath, err := os.Executable()
				if err != nil {
					webviewLock.Unlock()
					sendJSON(w, map[string]interface{}{
						"status":  "error",
						"message": "실행 파일 경로를 찾을 수 없습니다.",
					}, http.StatusInternalServerError)
					return
				}

				cmd := exec.Command(exePath, "--login")
				if err := cmd.Start(); err != nil {
					webviewLock.Unlock()
					sendJSON(w, map[string]interface{}{
						"status":  "error",
						"message": "로그인 웹뷰를 시작할 수 없습니다.",
					}, http.StatusInternalServerError)
					return
				}
				webviewProcess = cmd
				webviewLock.Unlock()

				sendJSON(w, map[string]interface{}{
					"status":  "started",
					"message": "네이버 로그인 웹뷰 창이 열렸습니다.",
				}, http.StatusOK)
				return
			}

			if path == "/login-wait" {
				if webviewProcess != nil {
					done := make(chan error, 1)
					go func() {
						done <- webviewProcess.Wait()
					}()

					select {
					case <-done:
					case <-time.After(180 * time.Second):
					}
				}

				cfg := core.LoadConfig()
				if cfg.NidAut != "" && cfg.NidSes != "" {
					sendJSON(w, map[string]interface{}{
						"status": "completed",
						"config": cfg,
					}, http.StatusOK)
				} else {
					sendJSON(w, map[string]interface{}{
						"status":  "closed",
						"message": "로그인 창이 닫혔습니다.",
					}, http.StatusOK)
				}
				return
			}

			if path == "/unofficial-user" {
				proxyUnofficialRequest(w, r, "GET", "", nil, "https://comm-api.game.naver.com/nng_main/v1/user/getUserStatus")
				return
			}

			if path == "/open-browser" {
				rawURL := r.URL.Query().Get("url")
				if rawURL == "" || (!strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://")) {
					sendJSON(w, map[string]interface{}{"status": "error", "message": "잘못된 URL입니다."}, http.StatusBadRequest)
					return
				}
				if err := OpenURL(rawURL); err != nil {
					sendJSON(w, map[string]interface{}{"status": "error", "message": "브라우저 실행 실패: " + err.Error()}, http.StatusInternalServerError)
					return
				}
				sendJSON(w, map[string]interface{}{"status": "ok"}, http.StatusOK)
				return
			}

			if proxyDispatch(w, r, "GET") {
				return
			}
		}

		// OBS 독 정적 HTML 페이지 서빙
		if path == "/" || path == "/index.html" || path == "/chzzk-obs-dock.html" {
			// 로컬 디스크 파일 우선 확인, 없으면 내장 에셋 서빙
			if localHTML, err := os.ReadFile("chzzk-obs-dock.html"); err == nil {
				sendBytes(w, localHTML, http.StatusOK, "text/html; charset=utf-8")
				return
			}
			sendBytes(w, embeddedHTML, http.StatusOK, "text/html; charset=utf-8")
			return
		}

		sendJSON(w, map[string]interface{}{"code": 404, "message": "Not Found"}, http.StatusNotFound)

	case http.MethodPost:
		if !core.CheckApiAuth(w, r) {
			return
		}

		if path == "/save-config" {
			var bodyMap map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
				sendJSON(w, map[string]interface{}{"code": 400, "message": "설정 저장에 실패했습니다."}, http.StatusBadRequest)
				return
			}
			saved := core.SaveConfig(bodyMap)
			sendJSON(w, map[string]interface{}{
				"code":    200,
				"message": "성공적으로 저장되었습니다.",
				"config":  saved,
			}, http.StatusOK)
			return
		}

		if path == "/logout" {
			core.ClearConfig()
			// WebView2 세션 프로필 디렉토리 초기화 (다음 로그인 시 깨끗한 로그인 화면 보장)
			appData := os.Getenv("LOCALAPPDATA")
			if appData == "" {
				appData = os.Getenv("USERPROFILE")
			}
			profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")
			_ = os.RemoveAll(profileDir)

			sendJSON(w, map[string]interface{}{
				"code":    200,
				"message": "성공적으로 로그아웃되었습니다.",
			}, http.StatusOK)
			return
		}

		if proxyDispatch(w, r, "POST") {
			return
		}
		sendJSON(w, map[string]interface{}{"code": 404, "message": "Not Found"}, http.StatusNotFound)

	case http.MethodPut:
		if !core.CheckApiAuth(w, r) || !proxyDispatch(w, r, "PUT") {
			sendJSON(w, map[string]interface{}{"code": 404, "message": "Not Found"}, http.StatusNotFound)
		}

	case http.MethodPatch:
		if !core.CheckApiAuth(w, r) || !proxyDispatch(w, r, "PATCH") {
			sendJSON(w, map[string]interface{}{"code": 404, "message": "Not Found"}, http.StatusNotFound)
		}

	case http.MethodDelete:
		if !core.CheckApiAuth(w, r) || !proxyDispatch(w, r, "DELETE") {
			sendJSON(w, map[string]interface{}{"code": 404, "message": "Not Found"}, http.StatusNotFound)
		}

	default:
		sendJSON(w, map[string]interface{}{"code": 405, "message": "Method Not Allowed"}, http.StatusMethodNotAllowed)
	}
}

// ============================================================
//  시스템 트레이 및 앱 생명주기 관리
// ============================================================
func exitApp() {
	if trayInstance != nil {
		trayInstance.Stop()
	}
	os.Exit(0)
}

func runTray() {
	tray := core.NewPureWinTrayIcon(
		fmt.Sprintf("CHZZK OBS Dock Server (%s)", APP_VERSION),
		"icon.ico",
		embeddedIcon,
	)

	tray.StartNotificationTitle = "CHZZK OBS Dock"
	tray.StartNotificationMsg = fmt.Sprintf("치지직 OBS 독 서버가 시작되었습니다.\n독 URL: http://localhost:%d", HTTP_PORT)

	tray.MenuItems = []core.MenuItem{
		{
			Label: fmt.Sprintf("CHZZK Dock %s", APP_VERSION),
			Callback: func() {
				core.CopyDockUrl(HTTP_PORT)
			},
		},
		{IsSeparator: true},
		{
			Label: "시작 프로그램 등록",
			Callback: func() {
				core.ToggleStartup(!core.IsStartupEnabled())
			},
			CheckFn: core.IsStartupEnabled,
		},
		{IsSeparator: true},
		{
			Label:    "서버 종료",
			Callback: exitApp,
		},
	}

	trayInstance = tray
	tray.Run()
}

func showMessageBox(title, msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	procMessageBoxW := user32.NewProc("MessageBoxW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	msgPtr, _ := syscall.UTF16PtrFromString(msg)
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(msgPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x10)
}

// ============================================================
//  Main Entrypoint
// ============================================================
func main() {
	// --login 서브커맨드 감지 시 로그인 웹뷰 팝업 창 전담 모드로 실행
	if len(os.Args) > 1 && (os.Args[1] == "--login" || os.Args[1] == "-l" || os.Args[1] == "webview_login.py") {
		core.RunLoginWebview()
		os.Exit(0)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", HTTP_PORT)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		msg := fmt.Sprintf("CHZZK Dock 서버가 이미 실행 중이거나 포트(%d)가 사용 중입니다.\n\n작업 표시줄 트레이 아이콘이나 기존 실행 중인 프로그램을 확인해 주세요.", HTTP_PORT)
		fmt.Printf("\n[오류] %s\n\n", msg)
		showMessageBox("CHZZK OBS Dock - 실행 오류", msg)
		os.Exit(1)
	}

	server := &http.Server{
		Handler:      http.HandlerFunc(HttpDockHandler),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  CHZZK OBS Dock Server (%s)\n", APP_VERSION)
	fmt.Printf("  HTTP (통합 방송 독) : http://localhost:%d\n", HTTP_PORT)
	fmt.Println(strings.Repeat("=", 60))

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[HTTP Server Error] %v\n", err)
		}
	}()

	runTray()
}
