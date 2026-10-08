package main

import (
	_ "embed"
	"bytes"
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
	"golang.org/x/sys/windows"
)

//go:embed ui/chzzk-obs-dock.html
var embeddedHTML []byte

//go:embed ui/obs-stats-dock.html
var embeddedStatsHTML []byte

//go:embed icon.ico
var embeddedIcon []byte

//go:embed docs/cookie_guide_1.png
var embeddedGuide1 []byte

//go:embed docs/cookie_guide_2.png
var embeddedGuide2 []byte

//go:embed scripts/chzzk_dock_launcher.lua
var embeddedLauncherScript []byte

// ============================================================
//  CHZZK OBS Dock Server v0.6.1-Beta (Modular Architecture)
// ============================================================
const (
	APP_VERSION       = "v0.6.1-Beta"
	DEFAULT_HTTP_PORT = 8081
	USER_AGENT        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	activeHttpPort     = DEFAULT_HTTP_PORT
	isFallbackPort     = false
	portFallbackReason = ""
	webviewProcess      *exec.Cmd
	webviewWaitCh       chan struct{}
	webviewLock         sync.Mutex
	subProcessesLock    sync.Mutex
	subProcesses        []*exec.Cmd
	trayInstance        *core.PureWinTrayIcon
	httpClient          = &http.Client{Timeout: 10 * time.Second}
	chzzkApiBaseURL     = "https://api.chzzk.naver.com"
	naverGameApiBaseURL = "https://comm-api.game.naver.com"
)

// trackSubProcess: 서브프로세스 생명주기 추적 및 정상 종료 시 핸들 자동 정리
func trackSubProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	subProcessesLock.Lock()
	subProcesses = append(subProcesses, cmd)
	subProcessesLock.Unlock()

	go func() {
		_ = cmd.Wait()
		subProcessesLock.Lock()
		defer subProcessesLock.Unlock()
		for i, c := range subProcesses {
			if c == cmd {
				subProcesses = append(subProcesses[:i], subProcesses[i+1:]...)
				break
			}
		}
	}()
}

// killAllSubProcesses: 독 서버 종료 시 모든 자식 프로세스 일괄 안전 종료
func killAllSubProcesses() {
	subProcessesLock.Lock()
	defer subProcessesLock.Unlock()
	for _, cmd := range subProcesses {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	subProcesses = nil
}


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
	renderedHTMLCache []byte
	renderedHTMLOnce  sync.Once
	globalMutexHandle uintptr
	restartExecutor   = restartSelf
)

func getRenderedHTML() []byte {
	// 로컬 개발 환경에서 ui/chzzk-obs-dock.html 변경 시 즉각 반영 (핫 리로드)
	if data, err := os.ReadFile(filepath.Join("ui", "chzzk-obs-dock.html")); err == nil && len(data) > 0 {
		return bytes.ReplaceAll(data, []byte("{{APP_VERSION}}"), []byte(APP_VERSION))
	}
	if exePath, err := os.Executable(); err == nil {
		localHtml := filepath.Join(filepath.Dir(exePath), "ui", "chzzk-obs-dock.html")
		if data, err := os.ReadFile(localHtml); err == nil && len(data) > 0 {
			return bytes.ReplaceAll(data, []byte("{{APP_VERSION}}"), []byte(APP_VERSION))
		}
	}
	renderedHTMLOnce.Do(func() {
		renderedHTMLCache = bytes.ReplaceAll(embeddedHTML, []byte("{{APP_VERSION}}"), []byte(APP_VERSION))
	})
	return renderedHTMLCache
}

func getRenderedStatsHTML() []byte {
	// 로컬 개발 환경에서 ui/obs-stats-dock.html 변경 시 즉각 반영 (핫 리로드)
	if data, err := os.ReadFile(filepath.Join("ui", "obs-stats-dock.html")); err == nil && len(data) > 0 {
		return data
	}
	if exePath, err := os.Executable(); err == nil {
		localHtml := filepath.Join(filepath.Dir(exePath), "ui", "obs-stats-dock.html")
		if data, err := os.ReadFile(localHtml); err == nil && len(data) > 0 {
			return data
		}
	}
	return embeddedStatsHTML
}

// handleDiskTotal: GET /api/disk-total?drive=C (또는 D)
func handleDiskTotal(w http.ResponseWriter, r *http.Request) {
	drive := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("drive")))
	if drive == "" || len(drive) > 1 || drive[0] < 'A' || drive[0] > 'Z' {
		drive = "C"
	}

	rootPath, err := windows.UTF16PtrFromString(drive + `:\`)
	if err != nil {
		sendJSON(w, map[string]string{"error": "invalid drive"}, http.StatusBadRequest)
		return
	}

	var totalBytes uint64
	if err := windows.GetDiskFreeSpaceEx(rootPath, nil, &totalBytes, nil); err != nil {
		sendJSON(w, map[string]string{"error": "failed to get disk size"}, http.StatusInternalServerError)
		return
	}

	sendJSON(w, map[string]interface{}{
		"totalBytes": totalBytes,
	}, http.StatusOK)
}

// handleOpenFolder: GET /api/open-folder?path=D:\Recordings
func handleOpenFolder(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path != "" {
		cleanPath := filepath.Clean(path)
		go func() {
			_ = exec.Command("explorer.exe", cleanPath).Start()
		}()
	}
	sendJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func getTrayTooltip() string {
	return fmt.Sprintf("ChzzkDock-%s, port:%d", APP_VERSION, activeHttpPort)
}

var (
	user32DLL                   = syscall.NewLazyDLL("user32.dll")
	procAllowSetForegroundWindow = user32DLL.NewProc("AllowSetForegroundWindow")
)

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
	return strings.Contains(target, "/auto-complete/") || strings.Contains(target, "/service/") || strings.Contains(target, "/polling/")
}

// fetchSessionUserStatus: 특정 세션 토큰으로 치지직 사용자 상태 조회
func fetchSessionUserStatus(aut, ses string) (channelID, channelName, profileImage string, ok bool) {
	if aut == "" && ses == "" {
		return "", "", "", false
	}
	req, err := http.NewRequest("GET", naverGameApiBaseURL+"/nng_main/v1/user/getUserStatus", nil)
	if err != nil {
		return "", "", "", false
	}
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", aut, ses))
	req.Header.Set("Origin", "https://chzzk.naver.com")
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", "", false
	}
	defer resp.Body.Close()
	var res struct {
		Content struct {
			LoggedIn        bool   `json:"loggedIn"`
			UserIdHash      string `json:"userIdHash"`
			Nickname        string `json:"nickname"`
			ProfileImageUrl string `json:"profileImageUrl"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || !res.Content.LoggedIn {
		return "", "", "", false
	}
	return res.Content.UserIdHash, res.Content.Nickname, res.Content.ProfileImageUrl, true
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
	} else if strings.HasPrefix(path, "/manage/") || strings.HasPrefix(path, "/service/") || strings.HasPrefix(path, "/polling/") {
		targetURL = chzzkApiBaseURL + path
	} else {
		targetURL = chzzkApiBaseURL + "/manage/v1" + path
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

	// 200~299 정상 응답은 에러 로그를 남기지 않으며, 400 이상 비정상 응답만 도메인 에러 코드로 기록
	if resp.StatusCode >= 400 {
		var errCode string
		switch resp.StatusCode {
		case 401:
			errCode = core.ErrApiUnauthorized
		case 403:
			errCode = core.ErrApiPermissionWait
		case 500, 502, 503, 504:
			errCode = core.ErrApiServerDown
		default:
			errCode = core.ErrApiOtherError
		}
		core.LogError("[Proxy %s %s] [%s] 치지직 API 오류 (%d): %s", method, targetURL, errCode, resp.StatusCode, string(respBytes))
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



// ============================================================
//  시스템 트레이 및 앱 생명주기 관리
// ============================================================
func exitApp() {
	killAllSubProcesses()
	core.DestroyDockWindow()
	if trayInstance != nil {
		trayInstance.Stop()
	}
	os.Exit(0)
}

func restartSelf() {
	exePath, err := os.Executable()
	if err != nil {
		core.LogError("[Main] 재시작 실패 (os.Executable): %v", err)
		return
	}

	var cleanArgs []string
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "--port" || os.Args[i] == "-p") && i+1 < len(os.Args) {
			i++
			continue
		}
		cleanArgs = append(cleanArgs, os.Args[i])
	}

	killAllSubProcesses()
	core.DestroyDockWindow()
	if trayInstance != nil {
		trayInstance.Stop()
	}

	if globalMutexHandle != 0 {
		kernel32DLL := syscall.NewLazyDLL("kernel32.dll")
		kernel32DLL.NewProc("CloseHandle").Call(globalMutexHandle)
		globalMutexHandle = 0
	}

	cmd := exec.Command(exePath, cleanArgs...)
	if err := cmd.Start(); err != nil {
		core.LogError("[Main] 새 프로세스 실행 실패: %v", err)
	}

	os.Exit(0)
}

func getWatchdogLabel(sec int) string {
	switch sec {
	case -1:
		return "사용 안 함 (자동 종료 비활성화)"
	case 0:
		return "즉시 종료"
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
	case -1:
		return "사용 안 함"
	case 0:
		return "즉시 종료"
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
	st.WatchdogDisabled = (sec == -1)
	_ = core.SaveSettings(st)
	if trayInstance != nil {
		trayInstance.UpdateTooltip(getTrayTooltip())
		if sec == -1 {
			trayInstance.ShowNotification("CHZZK OBS Dock", "OBS 자동 종료가 비활성화되었습니다. (상시 실행 유지)")
		} else if sec == 0 {
			trayInstance.ShowNotification("CHZZK OBS Dock", "OBS 종료 시 즉시 종료되도록 설정되었습니다.")
		} else {
			trayInstance.ShowNotification("CHZZK OBS Dock", fmt.Sprintf("OBS 종료 시 %s 뒤 함께 종료되도록 설정되었습니다.", getWatchdogLabel(sec)))
		}
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
	tray := core.NewPureWinTrayIcon(
		getTrayTooltip(),
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
		{
			Label: "웹뷰 GPU 하드웨어 가속",
			CheckFn: func() bool {
				return core.GetEnableGPU()
			},
			Callback: func() {
				newVal := !core.GetEnableGPU()
				_ = core.SetEnableGPUSetting(newVal)
				if trayInstance != nil {
					statusStr := "활성화"
					if !newVal {
						statusStr = "비활성화 (CPU 렌더링)"
					}
					trayInstance.ShowNotification("CHZZK OBS Dock", fmt.Sprintf("웹뷰 GPU 가속이 %s되었습니다. (창을 다시 열 때 적용)", statusStr))
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
					Label: "사용 안 함 (자동 종료 비활성화)",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == -1 },
					Callback: func() { updateWatchdogTimeoutFromTray(-1) },
				},
				{
					Label: "즉시 종료 (0초)",
					CheckFn: func() bool { return core.GetWatchdogTimeoutSec() == 0 },
					Callback: func() { updateWatchdogTimeoutFromTray(0) },
				},
				{IsSeparator: true},
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
						isPreset := (sec == -1 || sec == 0 || sec == 10 || sec == 30 || sec == 60 || sec == 300 || sec == 600)
						if !isPreset {
							return fmt.Sprintf("직접 입력 (%d초 뒤 종료)", sec)
						}
						return "직접 입력 (독 UI 설정)"
					},
					CheckFn: func() bool {
						sec := core.GetWatchdogTimeoutSec()
						return !(sec == -1 || sec == 0 || sec == 10 || sec == 30 || sec == 60 || sec == 300 || sec == 600)
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
			Label: "📋 진단 리포트 클립보드 복사",
			Callback: func() {
				_ = core.CopyLogsToClipboard()
			},
		},
		{
			Label: "로그 확인하기 (메모장)",
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

func activateExistingInstance(user32 *syscall.LazyDLL) {
	className := "ChzzkDockWindowClass"
	classNamePtr, _ := syscall.UTF16PtrFromString(className)

	procFindWindowW := user32.NewProc("FindWindowW")
	procPostMessageW := user32.NewProc("PostMessageW")
	procShowWindow := user32.NewProc("ShowWindow")

	var existingHwnd uintptr
	for i := 0; i < 15; i++ {
		existingHwnd, _, _ = procFindWindowW.Call(uintptr(unsafe.Pointer(classNamePtr)), 0)
		if existingHwnd != 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	if existingHwnd != 0 {
		showMsgId := core.GetShowDockMessageId()
		if showMsgId != 0 {
			procPostMessageW.Call(existingHwnd, uintptr(showMsgId), 0, 0)
		}
		procShowWindow.Call(existingHwnd, 9 /* SW_RESTORE */)
		core.ForceForegroundWindow(existingHwnd, false)
	}

	time.Sleep(50 * time.Millisecond)
}

// ============================================================
//  E단계: 서버 기동 서브루틴 및 압축된 Main 진입점
// ============================================================

func handleSubcommands() bool {
	if len(os.Args) <= 1 {
		return false
	}
	cmd := os.Args[1]
	switch cmd {
	case "--login", "-l":
		core.RunLoginWebview()
		os.Exit(0)
	case "--remote", "-r":
		channelId := ""
		if len(os.Args) > 2 {
			channelId = strings.TrimSpace(os.Args[2])
		}
		core.RunRemoteWebview(channelId)
		os.Exit(0)
	case "--chat", "-c":
		channelId := ""
		if len(os.Args) > 2 {
			channelId = strings.TrimSpace(os.Args[2])
		}
		core.RunChatWebview(channelId)
		os.Exit(0)
	case "--install-script":
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
	return false
}

func isSilentMode() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--silent" || arg == "--background" || arg == "-s" {
			return true
		}
	}
	return false
}

func acquireSingleInstanceMutex(silentMode bool, user32 *syscall.LazyDLL) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	mutexName := `Local\ChzzkDock`
	mutexNamePtr, _ := syscall.UTF16PtrFromString(mutexName)
	mutexHandle, _, errCall := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	globalMutexHandle = mutexHandle

	errno, isErrno := errCall.(syscall.Errno)
	if isErrno && errno == 183 { // ERROR_ALREADY_EXISTS
		if silentMode {
			releaseSingleInstanceMutex()
			os.Exit(0)
		}
		core.LogInfo("[Main] 치지직 독 인스턴스가 이미 실행 중입니다. 기존 인스턴스 독 화면을 최상단으로 활성화합니다.")
		activateExistingInstance(user32)
		releaseSingleInstanceMutex()
		os.Exit(0)
	}
}

func releaseSingleInstanceMutex() {
	if globalMutexHandle != 0 {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		kernel32.NewProc("CloseHandle").Call(globalMutexHandle)
		globalMutexHandle = 0
	}
}

func initWatchdog(silentMode bool) {
	core.OnShutdownCallback = killAllSubProcesses
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
}

func triggerStartupSessionCheck() {
	go func() {
		cfg := core.LoadConfig()
		if cfg.NidAut != "" {
			core.CheckAndRefreshSessionOnStartup(cfg.NidAut, cfg.NidSes)
		}
	}()
}

func parseCliPort() int {
	for i := 1; i < len(os.Args); i++ {
		if (os.Args[i] == "--port" || os.Args[i] == "-p") && i+1 < len(os.Args) {
			var p int
			if _, err := fmt.Sscanf(os.Args[i+1], "%d", &p); err == nil && p >= 1024 && p <= 65535 {
				return p
			}
		}
	}
	return 0
}

func bindServerListener(cliPort int, silentMode bool, user32 *syscall.LazyDLL) net.Listener {
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

		if strings.EqualFold(procName, "chzzk-dock.exe") || strings.EqualFold(procName, "chzzk-dock") || strings.EqualFold(procName, "chzzk-obs-dock.exe") {
			core.LogInfo("[Main] 포트(%d)를 점유 중인 프로세스가 이미 chzzk-dock (PID: %d)입니다. 기존 창을 활성화합니다.", targetPort, pid)
			if silentMode {
				os.Exit(0)
			}
			activateExistingInstance(user32)
			os.Exit(0)
		}

		errCode := core.ErrBenDefaultPortCollision
		if targetPort != DEFAULT_HTTP_PORT {
			errCode = core.ErrBenCustomPortCollision
		}
		core.LogWarn("[Main] [%s] 지정 포트(%d)가 타 프로그램 '%s'(PID: %d)에 의해 사용 중입니다 (%v). 안전한 대체 포트를 자동 할당합니다.", errCode, targetPort, procName, pid, err)

		fallbackPort, fallbackListener, fbErr := core.FindSafeFallbackPort()
		if fbErr != nil {
			core.LogError("[Main] [%s] 대체 포트 할당 실패: %v", core.ErrBenPortScanFailed, fbErr)
			showMessageBox("CHZZK OBS Dock - 안내", "포트 충돌 후 대체 가능한 안전 포트를 찾지 못했습니다.\n네트워크 환경을 확인한 후 다시 실행해 주세요.")
			os.Exit(1)
		}

		listener = fallbackListener
		activeHttpPort = fallbackPort
		isFallbackPort = true
		portFallbackReason = fmt.Sprintf("지정 포트(%d)가 '%s'(PID: %d)에 의해 사용 중이어서 안전한 임시 포트(%d)로 자동 전환되었습니다.", targetPort, procName, pid, fallbackPort)

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

	return listener
}

func startHttpServer(listener net.Listener) {
	server := &http.Server{
		Handler:      buildRouter(),
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
}

// ============================================================
//  Main Entrypoint (간결한 시퀀스)
// ============================================================
func main() {
	core.SetAppVersion(APP_VERSION)

	if handleSubcommands() {
		return
	}

	silentMode := isSilentMode()
	user32 := syscall.NewLazyDLL("user32.dll")

	acquireSingleInstanceMutex(silentMode, user32)
	defer releaseSingleInstanceMutex()

	initWatchdog(silentMode)
	triggerStartupSessionCheck()

	cliPort := parseCliPort()
	listener := bindServerListener(cliPort, silentMode, user32)

	startHttpServer(listener)
	time.Sleep(150 * time.Millisecond)

	runTray(silentMode)
}






