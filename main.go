package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"chzzk-dock/core"
	"golang.org/x/sys/windows"
)

//go:embed ui/*
var embeddedUI embed.FS

//go:embed ui/main-dock/index.html
var embeddedHTML []byte

//go:embed ui/overlay-settings/manager.html
var embeddedOverlayManager []byte

//go:embed ui/stats-dock/index.html
var embeddedStatsHTML []byte

//go:embed ui/mini-dock/index.html
var embeddedMiniHTML []byte

//go:embed docs/cookie_guide_1.png
var embeddedGuide1 []byte

//go:embed docs/cookie_guide_2.png
var embeddedGuide2 []byte

//go:embed icon.ico
var embeddedAppIcon []byte

//go:embed scripts/chzzk_dock_launcher.lua
var embeddedLauncherScript []byte

//go:embed versioninfo.json
var embeddedVersionInfo []byte

var appVersionLoadErr error

func parseAppVersionFromJSON(raw []byte) (string, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	var vi struct {
		StringFileInfo struct {
			ProductVersion string `json:"ProductVersion"`
		} `json:"StringFileInfo"`
	}
	if err := json.Unmarshal(raw, &vi); err != nil {
		return "v0.0.0-dev", err
	}
	ver := strings.TrimSpace(vi.StringFileInfo.ProductVersion)
	if ver == "" {
		return "v0.0.0-dev", fmt.Errorf("ProductVersion이 비어 있음")
	}
	if !strings.HasPrefix(ver, "v") && !strings.HasPrefix(ver, "V") {
		return "v" + ver, nil
	}
	return ver, nil
}

func loadAppVersionFromEmbed() string {
	ver, err := parseAppVersionFromJSON(embeddedVersionInfo)
	appVersionLoadErr = err
	return ver
}

// ============================================================
//
//	CHZZK OBS Dock Server (Modular Architecture)
//
// ============================================================
const (
	DEFAULT_HTTP_PORT = 8081
	USER_AGENT        = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	APP_VERSION         = loadAppVersionFromEmbed()
	activeHttpPort      = DEFAULT_HTTP_PORT
	isFallbackPort      = false
	portFallbackReason  = ""
	webviewProcess      *exec.Cmd
	webviewWaitCh       chan struct{}
	webviewLock         sync.Mutex
	trayInstance        *core.PureWinTrayIcon
	httpClient          = &http.Client{Timeout: 10 * time.Second}
	chzzkApiBaseURL     = "https://api.chzzk.naver.com"
	naverGameApiBaseURL = "https://comm-api.game.naver.com"
)

func sendBytes(w http.ResponseWriter, body []byte, status int, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func sendJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

var (
	renderedHTMLCache []byte
	renderedHTMLOnce  sync.Once
	globalMutexHandle uintptr
	restartExecutor   = restartSelf
	runtimeAppVersion = APP_VERSION
)

func getRenderedHTML() []byte {
	renderedHTMLOnce.Do(func() {
		renderedHTMLCache = bytes.ReplaceAll(embeddedHTML, []byte("{{APP_VERSION}}"), []byte(runtimeAppVersion))
		renderedHTMLCache = renderOverlayManager(renderedHTMLCache)
	})
	return renderedHTMLCache
}

func renderOverlayManager(page []byte) []byte {
	return bytes.ReplaceAll(page, []byte("{{OVERLAY_MANAGER}}"), embeddedOverlayManager)
}

func getRenderedStatsHTML() []byte {
	return embeddedStatsHTML
}

func getRenderedMiniHTML() []byte {
	return embeddedMiniHTML
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
	if path == "" {
		sendJSON(w, map[string]string{"error": "path is required"}, http.StatusBadRequest)
		return
	}
	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		sendJSON(w, map[string]string{"error": "유효한 폴더 경로가 아닙니다."}, http.StatusBadRequest)
		return
	}
	go func() {
		cmd := exec.Command("explorer.exe", cleanPath)
		if err := cmd.Start(); err == nil {
			_ = cmd.Process.Release()
		}
	}()
	sendJSON(w, map[string]string{"status": "ok"}, http.StatusOK)
}

func getTrayTooltip() string {
	return fmt.Sprintf("ChzzkDock-%s, port:%d", APP_VERSION, activeHttpPort)
}

var (
	user32DLL                    = syscall.NewLazyDLL("user32.dll")
	procAllowSetForegroundWindow = user32DLL.NewProc("AllowSetForegroundWindow")
)

func getLauncherScriptData() []byte {
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

// verifySessionUserStatus: 특정 세션 토큰으로 치지직 사용자 상태 검증 및 오류 코드 반환
func verifySessionUserStatus(aut, ses string) (channelID, channelName, profileImage, errCode, detail string, ok bool) {
	return verifySessionUserStatusCtx(context.Background(), aut, ses)
}

func verifySessionUserStatusCtx(ctx context.Context, aut, ses string) (channelID, channelName, profileImage, errCode, detail string, ok bool) {
	if aut == "" && ses == "" {
		return "", "", "", core.ErrAuthSessionExpired, "세션 쿠키 없음", false
	}
	req, err := http.NewRequestWithContext(ctx, "GET", naverGameApiBaseURL+"/nng_main/v1/user/getUserStatus", nil)
	if err != nil {
		return "", "", "", core.ErrApiOtherError, fmt.Sprintf("요청 생성 실패: %v", err), false
	}
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", aut, ses))
	req.Header.Set("Origin", "https://chzzk.naver.com")
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", core.ErrApiOtherError, fmt.Sprintf("통신 실패: %v", err), false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		code := core.ErrApiOtherError
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			code = core.ErrApiUnauthorized
		case resp.StatusCode == http.StatusForbidden:
			code = core.ErrApiPermissionWait
		case resp.StatusCode >= 500:
			code = core.ErrApiServerDown
		}
		return "", "", "", code, fmt.Sprintf("HTTP %d", resp.StatusCode), false
	}
	var res struct {
		Content struct {
			LoggedIn        bool   `json:"loggedIn"`
			UserIdHash      string `json:"userIdHash"`
			Nickname        string `json:"nickname"`
			ProfileImageUrl string `json:"profileImageUrl"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", "", "", core.ErrApiOtherError, fmt.Sprintf("응답 파싱 실패: %v", err), false
	}
	if !res.Content.LoggedIn {
		return "", "", "", core.ErrAuthSessionExpired, "loggedIn=false (만료 또는 미인증 세션)", false
	}
	return res.Content.UserIdHash, res.Content.Nickname, res.Content.ProfileImageUrl, "", "", true
}

// fetchSessionUserStatus: 특정 세션 토큰으로 치지직 사용자 상태 조회
func fetchSessionUserStatus(aut, ses string) (channelID, channelName, profileImage string, ok bool) {
	channelID, channelName, profileImage, _, _, ok = verifySessionUserStatus(aut, ses)
	return
}

// ============================================================
//
//	시스템 트레이 및 앱 생명주기 관리
//
// ============================================================
func exitApp() {
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
	core.DetachFromJob(cmd)
	if err := cmd.Start(); err != nil {
		core.LogError("[Main] 새 프로세스 실행 실패: %v", err)
	}

	os.Exit(0)
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
	}
}

func runTray(silentMode bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// 1. 트레이 아이콘 인스턴스 생성 (트레이를 가장 먼저 띄워 UI 반응성을 즉시 확보)
	tray := core.NewPureWinTrayIcon(getTrayTooltip())
	trayInstance = tray

	// 2. 독 독립 윈도우(WebView2) 인스턴스 준비 (Lazy Init)
	// - silentMode가 아니고, 설정에서 '시작 시 팝업 열기'가 켜져 있을 때만 화면에 즉시 표시
	// - 팝업 열기가 꺼져 있으면 WebView2를 미리 띄우지 않아 백그라운드 메모리(~240MB) 절감
	showWindow := !silentMode && core.GetPopupOnStart()
	core.EnsureDockWindow(activeHttpPort, APP_VERSION, showWindow)

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
			Label: "오버레이 레이어 관리",
			Callback: func() {
				core.OpenBrowser(fmt.Sprintf("http://localhost:%d/ui/overlay-settings/index.html", activeHttpPort))
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
					Label:    "사용 안 함 (자동 종료 비활성화)",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == -1 },
					Callback: func() { updateWatchdogTimeoutFromTray(-1) },
				},
				{
					Label:    "즉시 종료 (0초)",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == 0 },
					Callback: func() { updateWatchdogTimeoutFromTray(0) },
				},
				{IsSeparator: true},
				{
					Label:    "10초 뒤 종료",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == 10 },
					Callback: func() { updateWatchdogTimeoutFromTray(10) },
				},
				{
					Label:    "30초 뒤 종료",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == 30 },
					Callback: func() { updateWatchdogTimeoutFromTray(30) },
				},
				{
					Label:    "1분 뒤 종료 (기본값)",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == 60 },
					Callback: func() { updateWatchdogTimeoutFromTray(60) },
				},
				{
					Label:    "5분 뒤 종료",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == 300 },
					Callback: func() { updateWatchdogTimeoutFromTray(300) },
				},
				{
					Label:    "10분 뒤 종료",
					CheckFn:  func() bool { return core.GetWatchdogTimeoutSec() == 600 },
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
			// [Ponytail/YAGNI] 동기식 ShowWindow/ForceForegroundWindow 호출로 인한 타 프로세스 UI 블로킹을 배제하고 비동기 PostMessageW만 전송
			procPostMessageW.Call(existingHwnd, uintptr(showMsgId), 0, 0)
		}
	}

	time.Sleep(30 * time.Millisecond)
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
		core.StartParentWatchdog()
		core.RunLoginWebview()
		os.Exit(0)
	case "--remote", "-r":
		core.StartParentWatchdog()
		channelId := ""
		if len(os.Args) > 2 {
			channelId = strings.TrimSpace(os.Args[2])
		}
		core.RunRemoteWebview(channelId)
		os.Exit(0)
	case "--chat", "-c", "--chat-overlay":
		core.StartParentWatchdog()
		channelId := ""
		if len(os.Args) > 2 {
			channelId = strings.TrimSpace(os.Args[2])
		}
		if os.Args[1] == "--chat-overlay" {
			if len(os.Args) > 3 {
				if err := core.RunOverlayLayer(channelId, os.Args[3]); err != nil {
					core.LogError("[Chat Overlay] %v", err)
				}
			} else {
				core.RunChatOverlay(channelId)
			}
		} else {
			core.RunChatWebview(channelId)
		}
		os.Exit(0)
	case "--install-script":
		targetDir := ""
		var cleanupFiles []string
		for i := 2; i < len(os.Args); i++ {
			arg := os.Args[i]
			if arg == "--cleanup" && i+1 < len(os.Args) {
				cleanupFiles = append(cleanupFiles, strings.Trim(os.Args[i+1], `"`))
				i++
				continue
			}
			if targetDir == "" {
				targetDir = strings.Trim(arg, `"`)
			}
		}
		if targetDir == "" {
			targetDir, _ = core.DetectObsScriptsDir()
			if targetDir == "" {
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
		for _, oldFile := range cleanupFiles {
			if oldFile != "" && !strings.EqualFold(filepath.Clean(oldFile), filepath.Clean(targetFile)) {
				_ = os.Remove(oldFile)
			}
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

func isObsLaunchMode() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--obs-launch" || arg == "--from-obs" {
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
	// 183: ERROR_ALREADY_EXISTS, 5: ERROR_ACCESS_DENIED (관리자 OBS 실행 인스턴스를 일반 사용자가 감지한 경우)
	// [Ponytail/YAGNI] 새 라이브러리 추가 없이 기존 core.IsProcessRunning으로 2차 검증하여 유령 핸들로 인한 오종료 방지
	if isErrno && (errno == 183 || (errno == 5 && core.IsProcessRunning("chzzk-dock.exe"))) {
		if silentMode || isObsLaunchMode() {
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
	enableWatchdog := true
	for _, arg := range os.Args[1:] {
		if arg == "--no-watchdog" || arg == "--standalone" {
			enableWatchdog = false
			break
		}
	}
	if enableWatchdog {
		core.StartObsWatchdog(silentMode, isObsLaunchMode())
	}
}

func triggerStartupSessionCheck() {
	go func() {
		cfg := core.LoadConfig()
		if cfg.NidAut != "" {
			core.CheckAndRefreshSessionOnStartup(cfg.NidAut, cfg.NidSes)
			if cid, _, _, ok := fetchSessionUserStatus(cfg.NidAut, cfg.NidSes); ok && cid != "" {
				SetCachedActiveChannelId(cid)
			}
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

		reasonHeader := fmt.Sprintf("기본 포트(%d)가 다른 프로그램에 의해 사용 중이어서\n", targetPort)
		occupantLine := fmt.Sprintf("• 점유 프로그램: %s (PID: %d)\n", procName, pid)
		if pid == 0 {
			reasonHeader = fmt.Sprintf("기본 포트(%d)가 Windows 시스템 예약 포트여서\n", targetPort)
			occupantLine = "• 원인: Windows 시스템 예약 포트와 충돌\n"
		}
		notifyMsg := fmt.Sprintf(
			"%s"+
				"안전한 임시 포트(%d)로 서버가 시작되었습니다.\n\n"+
				"%s"+
				"• 현재 독 주소: http://localhost:%d\n\n"+
				"OBS 브라우저 독 또는 웹 브라우저에서 위 새 주소로 접속해 주세요.\n"+
				"(독 설정 화면에서 원하시는 포트로 변경할 수 있습니다.)",
			reasonHeader, fallbackPort, occupantLine, fallbackPort,
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
//
//	Main Entrypoint (간결한 시퀀스)
//
// ============================================================
func main() {
	core.SetAppVersion(APP_VERSION)
	if appVersionLoadErr != nil {
		core.LogWarn("[Version] versioninfo.json 파싱 실패로 기본 버전(%s) 사용: %v", APP_VERSION, appVersionLoadErr)
	}
	core.SetEmbeddedIcon(embeddedAppIcon)
	core.OnShutdownCallback = exitApp
	if handleSubcommands() {
		return
	}

	core.SetDockShutdownCallback(exitApp)
	core.InitProcessJobObject()
	core.InitializeOverlayMode()

	silentMode := isSilentMode()
	user32 := syscall.NewLazyDLL("user32.dll")

	acquireSingleInstanceMutex(silentMode, user32)
	defer releaseSingleInstanceMutex()

	initWatchdog(silentMode)
	go core.CleanupLegacyArtifactsIfNeeded(APP_VERSION)
	triggerStartupSessionCheck()

	cliPort := parseCliPort()
	listener := bindServerListener(cliPort, silentMode, user32)

	startHttpServer(listener)
	go func() {
		layers, err := core.LoadOverlayLayers()
		if err != nil {
			core.LogWarn("[Chat Overlay] restore: %v", err)
			return
		}
		for _, layer := range layers {
			if !layer.Enabled {
				continue
			}
			if layer.ID == "chzzk" && layer.URL == "" && layer.RestoreChannel == "" {
				continue
			}
			if err := startToolWebview("--chat-overlay", layer.RestoreChannel, layer.ID); err != nil {
				core.LogWarn("[Chat Overlay] restore: %v", err)
			}
		}
	}()
	time.Sleep(150 * time.Millisecond)

	runTray(silentMode)
}

