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

//go:embed docs/cookie_guide_1.png
var embeddedGuide1 []byte

//go:embed docs/cookie_guide_2.png
var embeddedGuide2 []byte

//go:embed scripts/chzzk_dock_launcher.lua
var embeddedLauncherScript []byte

// ============================================================
//  CHZZK OBS Dock Server v0.5.2 (Modular Architecture)
// ============================================================
const (
	APP_VERSION       = "v0.5.2"
	DEFAULT_HTTP_PORT = 8081
	USER_AGENT        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	activeHttpPort     = DEFAULT_HTTP_PORT
	isFallbackPort     = false
	portFallbackReason = ""
	webviewProcess     *exec.Cmd
	webviewLock        sync.Mutex
	trayInstance       *core.PureWinTrayIcon
	httpClient         = &http.Client{Timeout: 10 * time.Second}
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

var (
	shell32DLL                  = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW           = shell32DLL.NewProc("ShellExecuteW")
	user32DLL                   = syscall.NewLazyDLL("user32.dll")
	procAllowSetForegroundWindow = user32DLL.NewProc("AllowSetForegroundWindow")
)

// OpenURL: 시스템 기본 브라우저로 입력된 URL을 엽니다.
func OpenURL(url string) error {
	switch runtime.GOOS {
	case "windows":
		// [SYS-501] cmd /c start 및 powershell 호출 제거
		// Win32 공식 ShellExecuteW API를 직접 바인딩하여 백신 오탐을 원천 차단하고 즉시 실행합니다.
		opPtr, err := syscall.UTF16PtrFromString("open")
		if err != nil {
			return err
		}
		urlPtr, err := syscall.UTF16PtrFromString(url)
		if err != nil {
			return err
		}
		ret, _, err := procShellExecuteW.Call(
			0,
			uintptr(unsafe.Pointer(opPtr)),
			uintptr(unsafe.Pointer(urlPtr)),
			0,
			0,
			1, // SW_SHOWNORMAL
		)
		// ShellExecuteW는 성공 시 32보다 큰 인스턴스 핸들을 반환합니다.
		if ret <= 32 {
			return fmt.Errorf("ShellExecuteW failed (code %d): %w", ret, err)
		}
		return nil
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

func getLauncherScriptData() []byte {
	if localScript, err := os.ReadFile("scripts/chzzk_dock_launcher.lua"); err == nil {
		return localScript
	}
	if localScript, err := os.ReadFile("chzzk_dock_launcher.lua"); err == nil {
		return localScript
	}
	return embeddedLauncherScript
}

// setCORSHeaders: [SEC-01] CORS Origin 엄격한 화이트리스트 제한
func setCORSHeaders(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	allowedOrigins := map[string]bool{
		fmt.Sprintf("http://localhost:%d", activeHttpPort): true,
		fmt.Sprintf("http://127.0.0.1:%d", activeHttpPort): true,
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

	// 쿠키가 필요한 엔드포인트인데 NID_AUT만 있고 NID_SES가 누락된 경우 즉시 자동 갱신 시도
	if !isPublic && nidAut != "" && nidSes == "" {
		if _, newSes, err := core.RefreshNaverSession(nidAut); err == nil && newSes != "" {
			cfg = core.LoadConfig()
			nidAut = cfg.NidAut
			nidSes = cfg.NidSes
		}
	}

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

	// 401 Unauthorized이거나 /unofficial-user에서 loggedIn: false가 반환된 경우, NID_AUT로 1회 무중단 자동 갱신 및 재시도
	isUserStatusLoggedOut := false
	if !isPublic && resp.StatusCode == http.StatusOK && strings.Contains(targetURL, "getUserStatus") {
		var statusResp struct {
			Content struct {
				LoggedIn bool `json:"loggedIn"`
			} `json:"content"`
		}
		if err := json.Unmarshal(respBytes, &statusResp); err == nil && !statusResp.Content.LoggedIn {
			isUserStatusLoggedOut = true
		}
	}

	if !isPublic && nidAut != "" && (resp.StatusCode == http.StatusUnauthorized || isUserStatusLoggedOut) {
		core.LogWarn("[Auth] 네이버 세션 만료 감지 (HTTP %d, loggedOut=%v). NID_AUT 기반 자동 갱신 시도...", resp.StatusCode, isUserStatusLoggedOut)
		if _, newSes, err := core.RefreshNaverSession(nidAut); err == nil && newSes != "" {
			cfg = core.LoadConfig()
			nidAut = cfg.NidAut
			nidSes = cfg.NidSes

			var retryBodyReader io.Reader
			if len(body) > 0 {
				retryBodyReader = strings.NewReader(string(body))
			}
			retryReq, rErr := http.NewRequest(method, targetURL, retryBodyReader)
			if rErr == nil {
				retryReq.Header.Set("User-Agent", USER_AGENT)
				if len(body) > 0 {
					retryReq.Header.Set("Content-Type", "application/json")
				}
				retryReq.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", nidAut, nidSes))
				retryReq.Header.Set("Origin", "https://chzzk.naver.com")
				retryReq.Header.Set("Referer", "https://chzzk.naver.com/")

				if retryResp, doErr := httpClient.Do(retryReq); doErr == nil {
					defer retryResp.Body.Close()
					if retryBytes, readErr := io.ReadAll(retryResp.Body); readErr == nil {
						core.LogInfo("[Auth] 세션 자동 갱신 후 재요청 성공 (HTTP %d)", retryResp.StatusCode)
						respBytes = retryBytes
						resp.StatusCode = retryResp.StatusCode
						contentType = retryResp.Header.Get("Content-Type")
						if contentType == "" {
							contentType = "application/json"
						}
					}
				}
			}
		} else {
			core.LogWarn("[Auth] 네이버 세션 자동 갱신 실패: %v", err)
			core.InvalidateConfigCache()
		}
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
			"/config":            true,
			"/login-webview":     true,
			"/login-wait":        true,
			"/unofficial-user":   true,
			"/open-browser":      true,
			"/obs-script-status": true,
			"/remote-webview":      true,
			"/watchdog-timeout":    true,
			"/port-status":         true,
			"/startup-popup-status": true,
		}

		if apiPaths[path] || strings.HasPrefix(path, "/unofficial/") {
			if !core.CheckApiAuth(w, r) {
				return
			}

			if path == "/obs-script-status" {
				scriptsDir, detected := core.DetectObsScriptsDir()
				installed := core.IsScriptInstalled(scriptsDir)
				sendJSON(w, map[string]interface{}{
					"code":      200,
					"detected":  detected,
					"path":      scriptsDir,
					"installed": installed,
				}, http.StatusOK)
				return
			}

			if path == "/config" {
				core.InvalidateConfigCache()
				cfg := core.LoadConfig()
				autMask := ""
				sesMask := ""
				if cfg.NidAut != "" {
					autMask = "••••••••••••••••••••••••••••••••"
				}
				if cfg.NidSes != "" {
					sesMask = "••••••••••••••••••••••••••••••••"
				}
				sendJSON(w, map[string]string{
					"nid_aut": autMask,
					"nid_ses": sesMask,
				}, http.StatusOK)
				return
			}

			if path == "/login-webview" {
				core.LogInfo("[HTTP] /login-webview 요청 수신")
				webviewLock.Lock()
				if webviewProcess != nil && webviewProcess.ProcessState == nil {
					webviewLock.Unlock()
					core.LogWarn("[HTTP] /login-webview: 이미 네이버 로그인 창이 열려 있습니다.")
					sendJSON(w, map[string]interface{}{
						"status":  "already_open",
						"message": "이미 네이버 로그인 창이 열려 있습니다.",
					}, http.StatusOK)
					return
				}

				exePath, err := os.Executable()
				if err != nil {
					webviewLock.Unlock()
					core.LogError("[HTTP] /login-webview: 실행 파일 경로 확인 실패: %v", err)
					sendJSON(w, map[string]interface{}{
						"status":  "error",
						"message": "실행 파일 경로를 찾을 수 없습니다.",
					}, http.StatusInternalServerError)
					return
				}

				cmd := exec.Command(exePath, "--login")
				if err := cmd.Start(); err != nil {
					webviewLock.Unlock()
					core.LogError("[HTTP] /login-webview: 로그인 서브프로세스 시작 실패: %v", err)
					sendJSON(w, map[string]interface{}{
						"status":  "error",
						"message": "로그인 웹뷰를 시작할 수 없습니다.",
					}, http.StatusInternalServerError)
					return
				}
				procAllowSetForegroundWindow.Call(uintptr(cmd.Process.Pid))
				webviewProcess = cmd
				webviewLock.Unlock()
				core.LogInfo("[HTTP] /login-webview: 로그인 서브프로세스 시작 완료 (PID: %d)", cmd.Process.Pid)

				sendJSON(w, map[string]interface{}{
					"status":  "started",
					"message": "네이버 로그인 웹뷰 창이 열렸습니다.",
				}, http.StatusOK)
				return
			}

			if path == "/login-wait" {
				core.LogInfo("[HTTP] /login-wait 대기 시작")
				if webviewProcess != nil {
					done := make(chan error, 1)
					go func() {
						done <- webviewProcess.Wait()
					}()

					select {
					case <-done:
						core.LogInfo("[HTTP] /login-wait: 로그인 프로세스 종료 감지")
					case <-time.After(180 * time.Second):
						core.LogWarn("[HTTP] /login-wait: 180초 대기 타임아웃")
					}
				}

				core.InvalidateConfigCache()
				cfg := core.LoadConfig()
				if cfg.NidAut != "" && cfg.NidSes != "" {
					core.LogInfo("[HTTP] /login-wait: 네이버 로그인 세션 쿠키 연동 성공")
					sendJSON(w, map[string]interface{}{
						"status": "completed",
						"config": map[string]string{
							"nid_aut": "••••••••••••••••••••••••••••••••",
							"nid_ses": "••••••••••••••••••••••••••••••••",
						},
					}, http.StatusOK)
				} else {
					core.LogWarn("[HTTP] /login-wait: 쿠키 미취득 상태로 창 닫힘")
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

			if path == "/remote-webview" {
				channelId := r.URL.Query().Get("channelId")
				exePath, err := os.Executable()
				if err != nil {
					sendJSON(w, map[string]interface{}{
						"status":  "error",
						"message": "실행 파일 경로를 찾을 수 없습니다.",
					}, http.StatusInternalServerError)
					return
				}
				args := []string{"--remote"}
				if channelId != "" {
					args = append(args, channelId)
				}
				cmd := exec.Command(exePath, args...)
				if err := cmd.Start(); err != nil {
					sendJSON(w, map[string]interface{}{
						"status":  "error",
						"message": "리모컨 창을 시작할 수 없습니다: " + err.Error(),
					}, http.StatusInternalServerError)
					return
				}
				procAllowSetForegroundWindow.Call(uintptr(cmd.Process.Pid))
				sendJSON(w, map[string]interface{}{
					"status":  "started",
					"message": "치지직 리모컨 창이 열렸습니다.",
				}, http.StatusOK)
				return
			}

			if path == "/watchdog-timeout" {
				timeoutSec := core.GetWatchdogTimeoutSec()
				sendJSON(w, map[string]interface{}{
					"code":        200,
					"timeout_sec": timeoutSec,
				}, http.StatusOK)
				return
			}

			if path == "/port-status" {
				sendJSON(w, map[string]interface{}{
					"code":            200,
					"active_port":     activeHttpPort,
					"configured_port": core.GetConfiguredPort(),
					"is_fallback":     isFallbackPort,
					"fallback_reason": portFallbackReason,
					"default_port":    DEFAULT_HTTP_PORT,
				}, http.StatusOK)
				return
			}

			if path == "/startup-popup-status" {
				sendJSON(w, map[string]interface{}{
					"code":           200,
					"popup_on_start": core.GetPopupOnStart(),
				}, http.StatusOK)
				return
			}

			if proxyDispatch(w, r, "GET") {
				return
			}
		}

		// 쿠키 확인 가이드 이미지 서빙 (새창 열기 및 독 내 임베드 지원)
		if path == "/guide-image/1" {
			if localImg, err := os.ReadFile("docs/cookie_guide_1.png"); err == nil {
				sendBytes(w, localImg, http.StatusOK, "image/png")
				return
			}
			sendBytes(w, embeddedGuide1, http.StatusOK, "image/png")
			return
		}
		if path == "/guide-image/2" {
			if localImg, err := os.ReadFile("docs/cookie_guide_2.png"); err == nil {
				sendBytes(w, localImg, http.StatusOK, "image/png")
				return
			}
			sendBytes(w, embeddedGuide2, http.StatusOK, "image/png")
			return
		}

		// OBS 자동 실행 Lua 스크립트 서빙
		if path == "/obs-script" || path == "/obs-launcher.lua" || path == "/chzzk_dock_launcher.lua" {
			sendBytes(w, getLauncherScriptData(), http.StatusOK, "text/plain; charset=utf-8")
			return
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

		if path == "/browse-obs-folder" {
			folder, err := core.BrowseForObsFolder("OBS Studio 설치 폴더(obs-studio) 또는 scripts 폴더를 선택하세요")
			if err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    500,
					"message": "폴더 선택 중 오류가 발생했습니다: " + err.Error(),
				}, http.StatusInternalServerError)
				return
			}
			if folder == "" {
				sendJSON(w, map[string]interface{}{
					"code":      200,
					"cancelled": true,
				}, http.StatusOK)
				return
			}
			resolved := core.ResolveObsScriptsDir(folder)
			sendJSON(w, map[string]interface{}{
				"code":          200,
				"cancelled":     false,
				"selected_path": folder,
				"resolved_path": resolved,
			}, http.StatusOK)
			return
		}

		if path == "/install-obs-script" {
			var reqData struct {
				CustomDir string `json:"custom_dir"`
			}
			if r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(&reqData)
			}

			scriptData := getLauncherScriptData()
			installedPath, err := core.InstallLauncherScriptToObs(reqData.CustomDir, scriptData)
			if err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    500,
					"message": "OBS 스크립트 설치 실패: " + err.Error(),
				}, http.StatusInternalServerError)
				return
			}
			sendJSON(w, map[string]interface{}{
				"code":    200,
				"message": "OBS 스크립트 폴더에 성공적으로 추가되었습니다.",
				"path":    installedPath,
			}, http.StatusOK)
			return
		}

		if path == "/export-script" {
			scriptData := getLauncherScriptData()
			createdPath, err := core.ExportLauncherScript(scriptData)
			if err != nil {
				sendJSON(w, map[string]interface{}{"code": 500, "message": err.Error()}, http.StatusInternalServerError)
				return
			}
			sendJSON(w, map[string]interface{}{
				"code":    200,
				"message": "스크립트 파일이 생성되고 코드가 클립보드에 복사되었습니다.",
				"path":    createdPath,
			}, http.StatusOK)
			return
		}

		if path == "/save-config" {
			var bodyMap map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&bodyMap); err != nil {
				sendJSON(w, map[string]interface{}{"code": 400, "message": "설정 저장에 실패했습니다."}, http.StatusBadRequest)
				return
			}
			aut, _ := bodyMap["nid_aut"].(string)
			ses, _ := bodyMap["nid_ses"].(string)
			if strings.Contains(aut, "•") || strings.Contains(aut, "*") || strings.Contains(ses, "•") || strings.Contains(ses, "*") {
				sendJSON(w, map[string]interface{}{"code": 400, "message": "더미 마스킹 값이 아닌 실제 쿠키 값을 입력하세요."}, http.StatusBadRequest)
				return
			}
			saved := core.SaveConfig(bodyMap)
			_ = saved
			sendJSON(w, map[string]interface{}{
				"code":    200,
				"message": "성공적으로 저장되었습니다.",
				"config": map[string]string{
					"nid_aut": "••••••••••••••••••••••••••••••••",
					"nid_ses": "••••••••••••••••••••••••••••••••",
				},
			}, http.StatusOK)
			return
		}

		if path == "/logout" {
			core.ClearConfig()
			// 자격 증명 금고(Windows Credential Manager) 및 메모리 캐시 즉시 파기.
			// 단, 네이버 2단계 인증 '이 기기 기억하기' 및 브라우저 편의 상태 유지를 위해
			// webview2_profile 폴더는 보존합니다.

			sendJSON(w, map[string]interface{}{
				"code":    200,
				"message": "성공적으로 로그아웃되었습니다.",
			}, http.StatusOK)
			return
		}

		if path == "/save-startup-popup" {
			var reqData struct {
				PopupOnStart bool `json:"popup_on_start"`
			}
			if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    400,
					"message": "잘못된 요청 형식입니다.",
				}, http.StatusBadRequest)
				return
			}
			if err := core.SetPopupOnStartSetting(reqData.PopupOnStart); err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    500,
					"message": "설정 저장에 실패했습니다.",
				}, http.StatusInternalServerError)
				return
			}
			core.LogInfo("[Settings] 시작 시 독 팝업창 자동 열기 설정이 %v(으)로 저장되었습니다.", reqData.PopupOnStart)
			sendJSON(w, map[string]interface{}{
				"code":           200,
				"popup_on_start": reqData.PopupOnStart,
				"message":        "시작 팝업 설정이 저장되었습니다.",
			}, http.StatusOK)
			return
		}

		if path == "/watchdog-timeout" {
			var reqData struct {
				TimeoutSec int `json:"timeout_sec"`
			}
			if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    400,
					"message": "잘못된 요청 형식입니다.",
				}, http.StatusBadRequest)
				return
			}
			if reqData.TimeoutSec < 5 || reqData.TimeoutSec > 86400 {
				sendJSON(w, map[string]interface{}{
					"code":    400,
					"message": "대기 시간은 5초에서 86400초(24시간) 사이여야 합니다.",
				}, http.StatusBadRequest)
				return
			}
			core.SetWatchdogTimeoutSec(reqData.TimeoutSec)
			st := core.LoadSettings()
			st.WatchdogTimeoutSec = reqData.TimeoutSec
			_ = core.SaveSettings(st)
			if trayInstance != nil {
				portLabel := fmt.Sprintf("포트: %d", activeHttpPort)
				if isFallbackPort {
					portLabel = fmt.Sprintf("대체 포트: %d (점유 충돌)", activeHttpPort)
				}
				trayInstance.UpdateTooltip(fmt.Sprintf("CHZZK OBS Dock Server (%s) - %s, OBS와 함께 종료: %s", APP_VERSION, portLabel, getWatchdogTrayLabel(reqData.TimeoutSec)))
			}
			sendJSON(w, map[string]interface{}{
				"code":        200,
				"message":     "대기 시간이 성공적으로 변경되었습니다.",
				"timeout_sec": reqData.TimeoutSec,
			}, http.StatusOK)
			return
		}

		if path == "/check-port" {
			var reqData struct {
				Port int `json:"port"`
			}
			if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    400,
					"message": "잘못된 요청 형식입니다.",
				}, http.StatusBadRequest)
				return
			}

			if reqData.Port < 1024 || reqData.Port > 65535 {
				appErr := core.NewAppError(
					core.ErrNetInvalidPortRange,
					"포트 번호 범위 오류",
					fmt.Sprintf("포트 번호는 1024 ~ 65535 사이여야 합니다 (입력값: %d).", reqData.Port),
					fmt.Sprintf("Invalid port range: %d", reqData.Port),
					nil,
				)
				core.LogWarn(appErr.DevFormat())
				sendJSON(w, map[string]interface{}{
					"code":       400,
					"error_code": appErr.Code,
					"available":  false,
					"port":       reqData.Port,
					"message":    appErr.UserMsg,
				}, http.StatusBadRequest)
				return
			}

			// 현재 활성화된 독 서버의 포트와 동일하다면 사용 중인 독 포트로 안내
			if reqData.Port == activeHttpPort {
				sendJSON(w, map[string]interface{}{
					"code":      200,
					"available": true,
					"port":      reqData.Port,
					"message":   fmt.Sprintf("현재 CHZZK OBS Dock에서 정상 작동 중인 포트(%d)입니다.", reqData.Port),
				}, http.StatusOK)
				return
			}

			available, pid, procName, err := core.CheckPortAvailable(reqData.Port)
			if !available {
				displayName := procName
				if displayName == "" {
					displayName = "알 수 없는 프로그램"
				}
				errMsg := fmt.Sprintf("포트 %d번은 이미 다른 프로그램('%s', PID %d)에서 사용 중입니다.", reqData.Port, displayName, pid)
				if pid == 0 {
					errMsg = fmt.Sprintf("포트 %d번은 이미 다른 프로그램에서 사용 중입니다.", reqData.Port)
				}
				if err != nil {
					core.LogWarn("[PortCheck] Port %d unavailable: %v (PID: %d, Proc: %s)", reqData.Port, err, pid, procName)
				}
				sendJSON(w, map[string]interface{}{
					"code":        409,
					"available":   false,
					"port":        reqData.Port,
					"occupied_by": displayName,
					"pid":         pid,
					"message":     errMsg,
				}, http.StatusOK)
				return
			}

			sendJSON(w, map[string]interface{}{
				"code":      200,
				"available": true,
				"port":      reqData.Port,
				"message":   fmt.Sprintf("포트 %d번은 사용 가능합니다.", reqData.Port),
			}, http.StatusOK)
			return
		}

		if path == "/save-port" {
			var reqData struct {
				Port int `json:"port"`
			}
			if err := json.NewDecoder(r.Body).Decode(&reqData); err != nil {
				sendJSON(w, map[string]interface{}{
					"code":    400,
					"message": "잘못된 요청 형식입니다.",
				}, http.StatusBadRequest)
				return
			}

			if reqData.Port < 1024 || reqData.Port > 65535 {
				sendJSON(w, map[string]interface{}{
					"code":       400,
					"error_code": core.ErrNetInvalidPortRange,
					"message":    fmt.Sprintf("포트 번호는 1024 ~ 65535 사이여야 합니다 (입력값: %d).", reqData.Port),
				}, http.StatusBadRequest)
				return
			}

			if err := core.SaveConfiguredPort(reqData.Port); err != nil {
				appErr := core.NewAppError(
					core.ErrSysSettingsIoFailed,
					"설정 저장 실패",
					"포트 설정을 저장하는 중 오류가 발생했습니다.",
					fmt.Sprintf("Failed to save port %d to settings.json", reqData.Port),
					err,
				)
				core.LogError(appErr.DevFormat())
				sendJSON(w, map[string]interface{}{
					"code":       500,
					"error_code": appErr.Code,
					"message":    appErr.UserMsg,
				}, http.StatusInternalServerError)
				return
			}

			core.LogInfo("[Settings] 기본 HTTP 서버 포트 설정이 %d번으로 저장되었습니다.", reqData.Port)
			sendJSON(w, map[string]interface{}{
				"code":             200,
				"port":             reqData.Port,
				"restart_required": reqData.Port != activeHttpPort,
				"message":          fmt.Sprintf("기본 포트가 %d번으로 설정되었습니다.\n프로그램을 재시작하면 새 포트로 작동합니다.", reqData.Port),
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
	core.DestroyDockWindow()
	if trayInstance != nil {
		trayInstance.Stop()
	}
	os.Exit(0)
}

func getWatchdogLabel(sec int) string {
	switch sec {
	case 10:
		return "10초"
	case 30:
		return "30초"
	case 60:
		return "1분"
	case 300:
		return "5분"
	case 600:
		return "10분"
	default:
		return fmt.Sprintf("%d초", sec)
	}
}

func getWatchdogTrayLabel(sec int) string {
	switch sec {
	case 10:
		return "10초 뒤 종료"
	case 30:
		return "30초 뒤 종료"
	case 60:
		return "1분 뒤 종료 (기본값)"
	case 300:
		return "5분 뒤 종료"
	case 600:
		return "10분 뒤 종료"
	default:
		return fmt.Sprintf("%d초 뒤 종료 (직접 입력)", sec)
	}
}

func updateWatchdogTimeoutFromTray(sec int) {
	core.SetWatchdogTimeoutSec(sec)
	st := core.LoadSettings()
	st.WatchdogTimeoutSec = sec
	_ = core.SaveSettings(st)
	if trayInstance != nil {
		portLabel := fmt.Sprintf("포트: %d", activeHttpPort)
		if isFallbackPort {
			portLabel = fmt.Sprintf("대체 포트: %d (점유 충돌)", activeHttpPort)
		}
		trayInstance.UpdateTooltip(fmt.Sprintf("CHZZK OBS Dock Server (%s) - %s, OBS와 함께 종료: %s", APP_VERSION, portLabel, getWatchdogTrayLabel(sec)))
		trayInstance.ShowNotification("CHZZK OBS Dock", fmt.Sprintf("OBS 종료 시 %s 뒤 함께 종료되도록 설정되었습니다.", getWatchdogLabel(sec)))
	}
}

func runTray(silentMode bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 1. 독 독립 윈도우(WebView2) 생성
	// - silentMode(OBS 연동 등)가 아니고, 설정에서 '시작 시 팝업 열기'가 켜져 있을 때만 화면에 즉시 표시
	showWindow := !silentMode && core.GetPopupOnStart()
	core.InitDockWindow(activeHttpPort, APP_VERSION, showWindow)

	// 2. 트레이 아이콘 설정
	sec := core.GetWatchdogTimeoutSec()
	portLabel := fmt.Sprintf("포트: %d", activeHttpPort)
	if isFallbackPort {
		portLabel = fmt.Sprintf("대체 포트: %d (점유 충돌)", activeHttpPort)
	}

	tray := core.NewPureWinTrayIcon(
		fmt.Sprintf("CHZZK OBS Dock Server (%s) - %s, OBS와 함께 종료: %s", APP_VERSION, portLabel, getWatchdogTrayLabel(sec)),
		"icon.ico",
		embeddedIcon,
	)

	tray.StartNotificationTitle = "CHZZK OBS Dock"
	if isFallbackPort {
		tray.StartNotificationMsg = fmt.Sprintf("기본 포트 충돌로 임시 포트(%d)로 시작되었습니다.\n독 URL: http://localhost:%d", activeHttpPort, activeHttpPort)
	} else {
		tray.StartNotificationMsg = fmt.Sprintf("치지직 OBS 독 서버가 시작되었습니다.\n독 URL: http://localhost:%d", activeHttpPort)
	}

	tray.MenuItems = []core.MenuItem{
		{
			Label: "🖥️ CHZZK 독 팝업창 열기",
			Callback: func() {
				core.ShowDockWindow()
			},
		},
		{
			Label: fmt.Sprintf("CHZZK Dock %s (주소 복사)", APP_VERSION),
			Callback: func() {
				core.CopyDockUrl(activeHttpPort)
			},
		},
		{IsSeparator: true},
		{
			Label: "시작 시 독 팝업창 자동 열기",
			CheckFn: func() bool {
				return core.GetPopupOnStart()
			},
			Callback: func() {
				newVal := !core.GetPopupOnStart()
				_ = core.SetPopupOnStartSetting(newVal)
				if trayInstance != nil {
					statusStr := "활성화"
					if !newVal {
						statusStr = "비활성화 (트레이 시작)"
					}
					trayInstance.ShowNotification("CHZZK OBS Dock", fmt.Sprintf("시작 시 독 팝업창 자동 열기가 %s되었습니다.", statusStr))
				}
			},
		},
		{IsSeparator: true},
		{
			DynamicLabel: func() string {
				sec := core.GetWatchdogTimeoutSec()
				return fmt.Sprintf("OBS와 함께 종료: %s", getWatchdogTrayLabel(sec))
			},
			SubItems: []core.MenuItem{
				{
					Label: "10초 뒤 종료",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == 10 },
					Callback: func() { updateWatchdogTimeoutFromTray(10) },
				},
				{
					Label: "30초 뒤 종료",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == 30 },
					Callback: func() { updateWatchdogTimeoutFromTray(30) },
				},
				{
					Label: "1분 뒤 종료 (기본값)",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == 60 },
					Callback: func() { updateWatchdogTimeoutFromTray(60) },
				},
				{
					Label: "5분 뒤 종료",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == 300 },
					Callback: func() { updateWatchdogTimeoutFromTray(300) },
				},
				{
					Label: "10분 뒤 종료",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == 600 },
					Callback: func() { updateWatchdogTimeoutFromTray(600) },
				},
				{IsSeparator: true},
				{
					DynamicLabel: func() string {
						sec := core.GetWatchdogTimeoutSec()
						isPreset := (sec == 10 || sec == 30 || sec == 60 || sec == 300 || sec == 600)
						if !isPreset {
							return fmt.Sprintf("직접 입력 (%d초 뒤 종료)", sec)
						}
						return "직접 입력 (독 UI 설정)"
					},
					CheckFn: func() bool {
						sec := core.GetWatchdogTimeoutSec()
						return !(sec == 10 || sec == 30 || sec == 60 || sec == 300 || sec == 600)
					},
					DisabledFn: func() bool {
						return true
					},
					Callback: nil,
				},
			},
		},
		{IsSeparator: true},
		{
			Label: "로그 확인하기",
			Callback: func() {
				_ = core.ViewLogsInNotepad()
			},
		},
		{
			Label: "로그 저장 (.txt)",
			Callback: func() {
				_, _ = core.SaveLogsWithDialog()
			},
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
	if len(os.Args) > 1 && (os.Args[1] == "--login" || os.Args[1] == "-l") {
		core.RunLoginWebview()
		os.Exit(0)
	}

	// --remote 서브커맨드 감지 시 치지직 공식 리모컨 미니창 실행
	if len(os.Args) > 1 && (os.Args[1] == "--remote" || os.Args[1] == "-r") {
		channelId := ""
		if len(os.Args) > 2 {
			channelId = strings.TrimSpace(os.Args[2])
		}
		core.RunRemoteWebview(channelId)
		os.Exit(0)
	}

	// --install-script 서브커맨드 감지 시 (UAC 관리자 권한 자식 프로세스 모드)
	if len(os.Args) > 1 && os.Args[1] == "--install-script" {
		targetDir := ""
		if len(os.Args) > 2 {
			targetDir = strings.Trim(os.Args[2], `"`)
		}
		if targetDir == "" {
			var err error
			targetDir, err = core.FindObsScriptsDir()
			if err != nil {
				os.Exit(1)
			}
		}
		_ = os.MkdirAll(targetDir, 0755)
		targetFile := filepath.Join(targetDir, "chzzk_dock_launcher.lua")
		scriptData := getLauncherScriptData()
		scriptData = core.PrepareLauncherScriptWithExePath(scriptData)
		if err := os.WriteFile(targetFile, scriptData, 0644); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// [SILENT / HEADLESS] 백그라운드 무화면 실행 모드 확인 (OBS 연동 스크립트 등)
	silentMode := false
	for _, arg := range os.Args[1:] {
		if arg == "--silent" || arg == "--background" || arg == "-s" {
			silentMode = true
			break
		}
	}

	// [단일 인스턴스 보장] 버전별 고유 Mutex를 통한 다중 버전 공존 및 중복 실행 감지
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	verKey := core.GetVersionKey(APP_VERSION)
	mutexName := fmt.Sprintf(`Local\ChzzkObsDock_%s_Server_Mutex`, verKey)
	mutexNamePtr, _ := syscall.UTF16PtrFromString(mutexName)
	mutexHandle, _, _ := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	lastErr, _, _ := kernel32.NewProc("GetLastError").Call()
	if lastErr == 183 { // ERROR_ALREADY_EXISTS
		// 백그라운드/스크립트 모드로 중복 실행된 경우 조용히 종료
		if silentMode {
			if mutexHandle != 0 {
				kernel32.NewProc("CloseHandle").Call(mutexHandle)
			}
			os.Exit(0)
		}

		core.LogInfo("[Main] 치지직 독 (%s) 인스턴스가 이미 실행 중입니다. 사용자에게 독 화면 표시 여부를 확인합니다.", APP_VERSION)

		displayVer := APP_VERSION
		if !strings.HasPrefix(displayVer, "v") {
			displayVer = "v" + displayVer
		}

		// Win32 MessageBoxW 확인 다이얼로그 (MB_YESNO | MB_ICONQUESTION | MB_TOPMOST)
		titlePtr, _ := syscall.UTF16PtrFromString("CHZZK OBS Dock 알림")
		msgText := fmt.Sprintf(
			"이미 CHZZK OBS Dock (%s) 프로그램이 백그라운드에서 실행 중입니다.\n\n현재 실행 중인 방송 독 화면을 여시겠습니까?",
			displayVer,
		)
		msgPtr, _ := syscall.UTF16PtrFromString(msgText)
		procMessageBoxW := user32.NewProc("MessageBoxW")
		ret, _, _ := procMessageBoxW.Call(
			0,
			uintptr(unsafe.Pointer(msgPtr)),
			uintptr(unsafe.Pointer(titlePtr)),
			uintptr(0x00000004|0x00000020|0x00040000), // MB_YESNO | MB_ICONQUESTION | MB_TOPMOST
		)

		if ret == 6 { // IDYES: 사용자가 [예]를 선택한 경우에만 기존 창 표시
			className := fmt.Sprintf("ChzzkDockWindowClass_%s", verKey)
			classNamePtr, _ := syscall.UTF16PtrFromString(className)

			procFindWindowW := user32.NewProc("FindWindowW")
			procPostMessageW := user32.NewProc("PostMessageW")
			procShowWindow := user32.NewProc("ShowWindow")

			var existingHwnd uintptr
			for i := 0; i < 20; i++ {
				existingHwnd, _, _ = procFindWindowW.Call(uintptr(unsafe.Pointer(classNamePtr)), 0)
				if existingHwnd != 0 {
					break
				}
				time.Sleep(50 * time.Millisecond)
			}

			if existingHwnd != 0 {
				showMsgId := core.GetShowDockMessageId(verKey)
				if showMsgId != 0 {
					procPostMessageW.Call(existingHwnd, uintptr(showMsgId), 0, 0)
				}
				procShowWindow.Call(existingHwnd, 9 /* SW_RESTORE */)
				core.ForceForegroundWindow(existingHwnd, false)
			}
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

	// [WATCHDOG] OBS 프로세스 감시 및 자동 자폭 활성화 (설정 기반, 기본 1분 유예 시간)
	enableWatchdog := true
	for _, arg := range os.Args[1:] {
		if arg == "--no-watchdog" || arg == "--standalone" {
			enableWatchdog = false
			break
		}
	}

	if enableWatchdog {
		core.StartObsWatchdog(silentMode)
	}

	// [세션 자동 복구] 프로그램 시작 시 저장된 NID_AUT 기반 네이버 세션 유효성 자동 검증 및 무중단 갱신
	go func() {
		cfg := core.LoadConfig()
		if cfg.NidAut != "" {
			core.CheckAndRefreshSessionOnStartup(cfg.NidAut, cfg.NidSes)
		}
	}()

	// CLI 포트 지정 지원 (--port <num> 또는 -p <num>)
	cliPort := 0
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "--port" || os.Args[i] == "-p") && i+1 < len(os.Args) {
			var p int
			if _, err := fmt.Sscanf(os.Args[i+1], "%d", &p); err == nil && p >= 1024 && p <= 65535 {
				cliPort = p
			}
		}
	}

	targetPort := DEFAULT_HTTP_PORT
	if cliPort > 0 {
		targetPort = cliPort
	} else {
		targetPort = core.GetConfiguredPort()
		if targetPort < 1024 || targetPort > 65535 {
			targetPort = DEFAULT_HTTP_PORT
		}
	}

	addr := fmt.Sprintf("127.0.0.1:%d", targetPort)
	listener, err := net.Listen("tcp", addr)
	activeHttpPort = targetPort

	if err != nil {
		pid, procName := core.FindProcessUsingPort(targetPort)
		if procName == "" {
			procName = "알 수 없는 프로그램"
		}
		core.LogWarn("[Main] 지정 포트(%d)가 '%s'(PID: %d)에 의해 사용 중입니다 (%v). 안전한 대체 포트를 자동 할당합니다.", targetPort, procName, pid, err)

		// 안전한 대체 포트(49152 ~ 65535) 자동 탐색 및 바인딩
		fallbackPort, fallbackListener, fbErr := core.FindSafeFallbackPort()
		if fbErr != nil {
			core.LogError("[Main] 대체 포트 할당 실패: %v", fbErr)
			showMessageBox("CHZZK OBS Dock - 안내", "포트 충돌 후 대체 가능한 안전 포트를 찾지 못했습니다.\n네트워크 환경을 확인한 후 다시 실행해 주세요.")
			os.Exit(1)
		}

		listener = fallbackListener
		activeHttpPort = fallbackPort
		isFallbackPort = true
		portFallbackReason = fmt.Sprintf("지정 포트(%d)가 '%s'(PID: %d)에 의해 사용 중이어서 안전한 임시 포트(%d)로 자동 전환되었습니다.", targetPort, procName, pid, fallbackPort)

		core.LogInfo("[Main] 안전한 대체 포트 %d번으로 서버를 시작합니다.", fallbackPort)

		occupantLine := fmt.Sprintf("• 점유 프로그램: %s (PID: %d)\n", procName, pid)
		if pid == 0 {
			occupantLine = "• 점유 프로그램: 다른 프로그램에서 사용 중\n"
		}

		notifyMsg := fmt.Sprintf(
			"기본 포트(%d)가 다른 프로그램에 의해 사용 중이어서\n"+
				"안전한 임시 포트(%d)로 서버가 시작되었습니다.\n\n"+
				"%s"+
				"• 현재 독 주소: http://localhost:%d\n\n"+
				"OBS 브라우저 독 또는 웹 브라우저에서 위 새 주소로 접속해 주세요.\n"+
				"(독 설정 화면에서 원하시는 포트로 변경할 수 있습니다.)",
			targetPort, fallbackPort, occupantLine, fallbackPort,
		)
		go showMessageBox("CHZZK OBS Dock - 안내", notifyMsg)
	}

	server := &http.Server{
		Handler:      http.HandlerFunc(HttpDockHandler),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	core.LogInfo("CHZZK OBS Dock Server (%s) 시작됨 (포트: %d, Fallback: %v)", APP_VERSION, activeHttpPort, isFallbackPort)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  CHZZK OBS Dock Server (%s)\n", APP_VERSION)
	fmt.Printf("  HTTP (통합 방송 독) : http://localhost:%d\n", activeHttpPort)
	fmt.Println(strings.Repeat("=", 60))

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("[HTTP Server Error] %v\n", err)
		}
	}()

	// 로컬 HTTP 서버 준비를 위한 초단위 이하(150ms) 확실한 지연시간 부여
	time.Sleep(150 * time.Millisecond)

	runTray(silentMode)
}
