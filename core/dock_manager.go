package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// ==============================================================================
// [CHZZK OBS DOCK - Dock Manager (Settings & Updater Engine)]
// - 설정 관리: settings.json 입출력, 프리셋 관리, 기본 브라우저 감지
// - 업데이트 관리: GitHub Releases 최신 버전 비동기 감지, 스트리밍 다운로드 및 무인 설치
// ==============================================================================

// -----------------------------------------------------------------------------
// 1. 설정 모델 및 함수 (Settings)
// -----------------------------------------------------------------------------

// AppSettings: 애플리케이션의 비보안/일반 설정 모델
type AppSettings struct {
	SchemaVersion        int               `json:"schema_version,omitempty"`        // 설정 스키마 버전 (기본값: 1)
	WatchdogTimeoutSec   int               `json:"watchdog_timeout_sec"`             // OBS 미감지 시 자체 종료 대기 시간 (초, 0: 사용 안 함)
	WatchdogDisabled     bool              `json:"watchdog_disabled,omitempty"`     // OBS 자동 종료 비활성화 여부
	HttpPort             int               `json:"http_port"`                        // 기본 HTTP 서버 포트 (기본값: 8081)
	PopupOnStart         *bool             `json:"popup_on_start,omitempty"`         // 프로그램 시작 시 웹뷰 팝업창 자동 열기 (기본값: false)
	NotifyOnShutdown     *bool             `json:"notify_on_shutdown,omitempty"`     // OBS 종료/미감지로 자동 종료 시 Windows 알림 표시 (기본값: true)
	EnableGPU            *bool             `json:"enable_gpu,omitempty"`             // 웹뷰 창 GPU 하드웨어 가속 여부 (기본값: true)
	ExternalBrowserGuard *bool             `json:"external_browser_guard,omitempty"` // 네이버 외 외부 링크 클릭 시 기본 브라우저로 열기 (기본값: true)
	RemoteTesterUnlocked bool              `json:"remote_tester_unlocked,omitempty"` // 리모컨 테스터 인증 승인 여부
	BroadcastPresets     []BroadcastPreset `json:"broadcast_presets,omitempty"`     // 방송 정보 프리셋 목록 (최대 10개)
}

// BroadcastPreset: 방송 정보(제목, 카테고리, 태그) 프리셋 데이터 모델
type BroadcastPreset struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Title        string   `json:"title"`
	CategoryID   string   `json:"category_id"`
	CategoryType string   `json:"category_type,omitempty"`
	CategoryName string   `json:"category_name"`
	Tags         []string `json:"tags"`
}

func (s AppSettings) IsRemoteTesterUnlocked() bool {
	return s.RemoteTesterUnlocked
}

func (s *AppSettings) SetRemoteTesterUnlocked(val bool) {
	s.RemoteTesterUnlocked = val
}

func (s AppSettings) IsPopupOnStart() bool {
	if s.PopupOnStart == nil {
		return false
	}
	return *s.PopupOnStart
}

func (s *AppSettings) SetPopupOnStart(val bool) {
	s.PopupOnStart = &val
}

func (s AppSettings) IsNotifyOnShutdown() bool {
	if s.NotifyOnShutdown == nil {
		return true // 기본값: 알림 켜짐
	}
	return *s.NotifyOnShutdown
}

func (s *AppSettings) SetNotifyOnShutdown(val bool) {
	s.NotifyOnShutdown = &val
}

func (s AppSettings) IsEnableGPU() bool {
	if s.EnableGPU == nil {
		return true // 기본값: GPU 가속 활성화
	}
	return *s.EnableGPU
}

func (s *AppSettings) SetEnableGPU(val bool) {
	s.EnableGPU = &val
}

func (s AppSettings) IsExternalBrowserGuard() bool {
	if s.ExternalBrowserGuard == nil {
		return true // 기본값: 외부 브라우저 가드 활성화
	}
	return *s.ExternalBrowserGuard
}

func (s *AppSettings) SetExternalBrowserGuard(val bool) {
	s.ExternalBrowserGuard = &val
}

func GetEnableGPU() bool {
	s := LoadSettings()
	return s.IsEnableGPU()
}

func SetEnableGPU(val bool) {
	_ = SetEnableGPUSetting(val)
}

func SetEnableGPUSetting(val bool) error {
	st := LoadSettings()
	st.SetEnableGPU(val)
	return SaveSettings(st)
}

func GetExternalBrowserGuard() bool {
	s := LoadSettings()
	return s.IsExternalBrowserGuard()
}

func SetExternalBrowserGuardSetting(val bool) error {
	st := LoadSettings()
	st.SetExternalBrowserGuard(val)
	return SaveSettings(st)
}

var (
	shlwapiDLL            = syscall.NewLazyDLL("shlwapi.dll")
	procAssocQueryStringW = shlwapiDLL.NewProc("AssocQueryStringW")
)

const (
	ASSOCSTR_EXECUTABLE = 2
	ASSOCSTR_PROGID     = 20
)

// GetDefaultBrowserName: Windows 공식 Shell API(AssocQueryStringW)를 통해 기본 웹 브라우저 이름을 감지합니다.
func GetDefaultBrowserName() string {
	assocPtr, _ := syscall.UTF16PtrFromString("https")
	extraPtr, _ := syscall.UTF16PtrFromString("open")

	buf := make([]uint16, 512)
	cch := uint32(len(buf))

	// 1차: ASSOCSTR_EXECUTABLE (2)
	ret, _, _ := procAssocQueryStringW.Call(
		0,
		uintptr(ASSOCSTR_EXECUTABLE),
		uintptr(unsafe.Pointer(assocPtr)),
		uintptr(unsafe.Pointer(extraPtr)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&cch)),
	)

	var detected string
	if ret == 0 && cch > 0 {
		detected = syscall.UTF16ToString(buf[:cch])
	}

	// 2차: 실패 시 ASSOCSTR_PROGID (20) 조회
	if detected == "" {
		cch = uint32(len(buf))
		ret, _, _ = procAssocQueryStringW.Call(
			0,
			uintptr(ASSOCSTR_PROGID),
			uintptr(unsafe.Pointer(assocPtr)),
			uintptr(unsafe.Pointer(extraPtr)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&cch)),
		)
		if ret == 0 && cch > 0 {
			detected = syscall.UTF16ToString(buf[:cch])
		}
	}

	if detected == "" {
		return "기본 브라우저"
	}

	p := strings.ToLower(detected)
	switch {
	case strings.Contains(p, "whale"):
		return "네이버 웨일(Whale)"
	case strings.Contains(p, "chrome"):
		return "구글 크롬(Chrome)"
	case strings.Contains(p, "msedge") || strings.Contains(p, "edge"):
		return "마이크로소프트 엣지(Edge)"
	case strings.Contains(p, "firefox"):
		return "파이어폭스(Firefox)"
	case strings.Contains(p, "brave"):
		return "브레이브(Brave)"
	case strings.Contains(p, "opera"):
		return "오페라(Opera)"
	default:
		base := filepath.Base(detected)
		if base != "" && base != "." {
			return strings.TrimSuffix(base, filepath.Ext(base))
		}
		return detected
	}
}

// OpenBrowser: Win32 ShellExecuteW를 통해 기본 웹 브라우저로 대상 URL을 엽니다.
func OpenBrowser(targetURL string) {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW := shell32.NewProc("ShellExecuteW")
	verbPtr, _ := syscall.UTF16PtrFromString("open")
	urlPtr, _ := syscall.UTF16PtrFromString(targetURL)
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verbPtr)), uintptr(unsafe.Pointer(urlPtr)), 0, 0, 1 /* SW_SHOWNORMAL */)
}

var (
	defaultPopupVal  = false
	defaultNotifyVal = true
	defaultGPUVal    = true
	defaultGuardVal  = true
	settingsMu       sync.RWMutex
	cachedSettings   = AppSettings{
		SchemaVersion:        1,
		WatchdogTimeoutSec:   60,
		HttpPort:             8081,
		PopupOnStart:         &defaultPopupVal,
		NotifyOnShutdown:     &defaultNotifyVal,
		EnableGPU:            &defaultGPUVal,
		ExternalBrowserGuard: &defaultGuardVal,
	}
	settingsLoaded = false
)

func getSettingsFilePath() string {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		if configDir, err := os.UserConfigDir(); err == nil {
			localAppData = configDir
		} else {
			localAppData = "."
		}
	}
	return filepath.Join(localAppData, "ChzzkObsDock", "settings.json")
}

// LoadSettings: %LOCALAPPDATA%\ChzzkObsDock\settings.json 에서 설정을 로드합니다.
func LoadSettings() AppSettings {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if settingsLoaded {
		return cachedSettings
	}

	filePath := getSettingsFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		defPop := false
		cachedSettings = AppSettings{WatchdogTimeoutSec: 60, HttpPort: 8081, PopupOnStart: &defPop}
		settingsLoaded = true
		return cachedSettings
	}

	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil {
		defPop := false
		cachedSettings = AppSettings{WatchdogTimeoutSec: 60, HttpPort: 8081, PopupOnStart: &defPop}
	} else {
		if s.WatchdogDisabled || s.WatchdogTimeoutSec < 0 {
			s.WatchdogDisabled = true
			s.WatchdogTimeoutSec = -1
		} else if s.WatchdogTimeoutSec == 0 {
			s.WatchdogDisabled = false
			s.WatchdogTimeoutSec = 0
		}
		if s.HttpPort < 1024 || s.HttpPort > 65535 {
			s.HttpPort = 8081
		}
		if s.PopupOnStart == nil {
			defPop := false
			s.PopupOnStart = &defPop
		}
		cachedSettings = s
	}
	settingsLoaded = true
	return cachedSettings
}

// SaveSettings: %LOCALAPPDATA%\ChzzkObsDock\settings.json 에 설정을 저장합니다.
func SaveSettings(s AppSettings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if s.SchemaVersion == 0 {
		s.SchemaVersion = 1
	}
	if s.WatchdogDisabled || s.WatchdogTimeoutSec < 0 {
		s.WatchdogDisabled = true
		s.WatchdogTimeoutSec = -1
	} else if s.WatchdogTimeoutSec == 0 {
		s.WatchdogDisabled = false
		s.WatchdogTimeoutSec = 0
	}
	if s.HttpPort < 1024 || s.HttpPort > 65535 {
		s.HttpPort = 8081
	}

	filePath := getSettingsFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return err
	}

	cachedSettings = s
	settingsLoaded = true
	return nil
}

// GetConfiguredPort: 설정된 포트 번호를 반환 (기본값 8081)
func GetConfiguredPort() int {
	return LoadSettings().HttpPort
}

// SaveConfiguredPort: 지정된 포트 번호를 설정에 저장
func SaveConfiguredPort(port int) error {
	st := LoadSettings()
	st.HttpPort = port
	return SaveSettings(st)
}

// GetPopupOnStart: 시작 시 팝업 열기 설정 조회 (기본값 false)
func GetPopupOnStart() bool {
	return LoadSettings().IsPopupOnStart()
}

// SetPopupOnStartSetting: 시작 시 팝업 열기 설정 저장
func SetPopupOnStartSetting(val bool) error {
	st := LoadSettings()
	st.SetPopupOnStart(val)
	return SaveSettings(st)
}

// GetNotifyOnShutdown: 자동 종료 시 Windows 알림 표시 여부 조회 (기본값 true)
func GetNotifyOnShutdown() bool {
	return LoadSettings().IsNotifyOnShutdown()
}

// SetNotifyOnShutdownSetting: 자동 종료 시 Windows 알림 표시 여부 설정 저장
func SetNotifyOnShutdownSetting(val bool) error {
	st := LoadSettings()
	st.SetNotifyOnShutdown(val)
	return SaveSettings(st)
}

// GetRemoteTesterUnlocked: 리모컨 테스터 인증 승인 여부 조회
func GetRemoteTesterUnlocked() bool {
	return LoadSettings().IsRemoteTesterUnlocked()
}

// SetRemoteTesterUnlocked: 리모컨 테스터 인증 승인 여부 저장 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func SetRemoteTesterUnlocked(val bool) error {
	st := LoadSettings()
	st.SetRemoteTesterUnlocked(val)
	return SaveSettings(st)
}

// GetBroadcastPresets: 방송 정보 프리셋 목록 조회 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func GetBroadcastPresets() []BroadcastPreset {
	st := LoadSettings()
	if st.BroadcastPresets == nil {
		return []BroadcastPreset{}
	}
	return st.BroadcastPresets
}

// SaveBroadcastPresets: 방송 정보 프리셋 목록 저장 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func SaveBroadcastPresets(presets []BroadcastPreset) error {
	st := LoadSettings()
	st.BroadcastPresets = presets
	return SaveSettings(st)
}

// ResetSettingsForTest: 단위 테스트용 캐시 초기화
func ResetSettingsForTest() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settingsLoaded = false
	defPop := false
	defNotif := true
	defGPU := true
	defGuard := true
	cachedSettings = AppSettings{WatchdogTimeoutSec: 60, HttpPort: 8081, PopupOnStart: &defPop, NotifyOnShutdown: &defNotif, EnableGPU: &defGPU, ExternalBrowserGuard: &defGuard}
}

// -----------------------------------------------------------------------------
// 2. 인앱 자동 업데이트 파이프라인 (Updater)
// -----------------------------------------------------------------------------

const (
	GithubReleasesAPI = "https://api.github.com/repos/MK0522/chzzk-dock/releases/latest"
)

// UpdateInfo: 최신 릴리즈 정보 DTO
type UpdateInfo struct {
	HasUpdate         bool   `json:"has_update"`
	CurrentVersion    string `json:"current_version"`
	LatestVersion     string `json:"latest_version"`
	ReleaseTitle      string `json:"release_title"`
	ReleaseNotes      string `json:"release_notes"`
	ReleaseURL        string `json:"release_url"`
	DownloadURL       string `json:"download_url"`
	AssetSize         int64  `json:"asset_size"`
	AlreadyDownloaded bool   `json:"already_downloaded"`
}

// UpdateProgress: 다운로드 및 실행 진행 상황
type UpdateProgress struct {
	Active   bool   `json:"active"`
	Percent  int    `json:"percent"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	SpeedBps int64  `json:"speed_bps"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
}

var (
	updateCacheMu   sync.RWMutex
	cachedUpdate    *UpdateInfo
	lastCheckTime   time.Time
	progressMu      sync.RWMutex
	currentProgress UpdateProgress
)

// ParseSemVer: "v0.5.12" 또는 "0.5.12" 형태의 문자열을 [major, minor, patch] 숫자로 분리
func ParseSemVer(v string) (int, int, int) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return 0, 0, 0
	}
	var nums [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		p := parts[i]
		if idx := strings.IndexAny(p, "-+"); idx != -1 {
			p = p[:idx]
		}
		if n, err := strconv.Atoi(p); err == nil {
			nums[i] = n
		}
	}
	return nums[0], nums[1], nums[2]
}

// CompareSemVer: v1 < v2 (-1), v1 == v2 (0), v1 > v2 (1)
func CompareSemVer(v1, v2 string) int {
	maj1, min1, pat1 := ParseSemVer(v1)
	maj2, min2, pat2 := ParseSemVer(v2)

	if maj1 != maj2 {
		if maj1 < maj2 {
			return -1
		}
		return 1
	}
	if min1 != min2 {
		if min1 < min2 {
			return -1
		}
		return 1
	}
	if pat1 != pat2 {
		if pat1 < pat2 {
			return -1
		}
		return 1
	}
	return 0
}

func getInstallerTempPath(version string) string {
	cleanVer := strings.TrimPrefix(strings.TrimSpace(version), "v")
	return filepath.Join(os.TempDir(), fmt.Sprintf("chzzk-dock-setup-%s.exe", cleanVer))
}

// CleanOldInstallers: 현재 버전과 다른 이전 임시 인스톨러 파일 정리
func CleanOldInstallers(currentVersion string) {
	tempDir := os.TempDir()
	matches, err := filepath.Glob(filepath.Join(tempDir, "chzzk-dock-setup-*.exe"))
	if err != nil {
		return
	}
	keepName := filepath.Base(getInstallerTempPath(currentVersion))
	for _, m := range matches {
		if filepath.Base(m) != keepName {
			_ = os.Remove(m)
		}
	}
}

// CheckForUpdate: GitHub Releases API 비동기 조회 (10분 캐시)
func CheckForUpdate(currentVersion string, force bool) (*UpdateInfo, error) {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	if !force && cachedUpdate != nil && time.Since(lastCheckTime) < 10*time.Minute {
		cachedCopy := *cachedUpdate
		cachedCopy.AlreadyDownloaded = isInstallerReady(cachedCopy.LatestVersion, cachedCopy.AssetSize)
		return &cachedCopy, nil
	}

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", GithubReleasesAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 요청 생성 실패: %w", err)
	}
	req.Header.Set("User-Agent", "CHZZK-OBS-Dock-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 응답 오류 (HTTP %d)", resp.StatusCode)
	}

	var ghRelease struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("릴리즈 응답 파싱 실패: %w", err)
	}

	latestVer := ghRelease.TagName
	hasUpdate := CompareSemVer(currentVersion, latestVer) < 0

	var downloadURL string
	var assetSize int64
	for _, asset := range ghRelease.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, "-installer.exe") || strings.Contains(name, "setup") || strings.HasSuffix(name, ".exe") {
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
			break
		}
	}

	CleanOldInstallers(latestVer)

	info := &UpdateInfo{
		HasUpdate:         hasUpdate,
		CurrentVersion:    currentVersion,
		LatestVersion:     latestVer,
		ReleaseTitle:      ghRelease.Name,
		ReleaseNotes:      ghRelease.Body,
		ReleaseURL:        ghRelease.HTMLURL,
		DownloadURL:       downloadURL,
		AssetSize:         assetSize,
		AlreadyDownloaded: isInstallerReady(latestVer, assetSize),
	}

	cachedUpdate = info
	lastCheckTime = time.Now()

	return info, nil
}

func isInstallerReady(version string, expectedSize int64) bool {
	if version == "" {
		return false
	}
	path := getInstallerTempPath(version)
	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	if expectedSize > 0 && stat.Size() != expectedSize {
		return false
	}
	return stat.Size() > 1024*1024 // 최소 1MB 이상
}

// GetUpdateProgress: 현재 진행률 조회
func GetUpdateProgress() UpdateProgress {
	progressMu.RLock()
	defer progressMu.RUnlock()
	return currentProgress
}

// ExecuteUpdate: 인스톨러 다운로드 및 백그라운드 설치 실행
func ExecuteUpdate(version, downloadURL string, expectedSize int64, onShutdown func()) error {
	progressMu.Lock()
	if currentProgress.Active {
		progressMu.Unlock()
		return fmt.Errorf("이미 업데이트 다운로드가 진행 중입니다")
	}
	currentProgress = UpdateProgress{
		Active:   true,
		Percent:  0,
		Received: 0,
		Total:    expectedSize,
		Done:     false,
		Error:    "",
	}
	progressMu.Unlock()

	installerPath := getInstallerTempPath(version)

	// 이미 받아둔 온전한 인스톨러가 있는 경우 다운로드 건너뛰기
	if isInstallerReady(version, expectedSize) {
		LogInfo("[Updater] 기존에 다운로드 완료된 인스톨러 사용: %s", installerPath)
		progressMu.Lock()
		currentProgress.Percent = 100
		currentProgress.Done = true
		currentProgress.Active = false
		progressMu.Unlock()

		go func() {
			time.Sleep(500 * time.Millisecond)
			cmd := exec.Command(installerPath, "/SILENT", "/SP-")
			if err := cmd.Start(); err != nil {
				LogError("[Updater] 인스톨러 실행 실패: %v", err)
				progressMu.Lock()
				currentProgress.Error = fmt.Sprintf("설치기 실행 실패: %v", err)
				progressMu.Unlock()
				return
			}
			LogInfo("[Updater] 새 버전 인스톨러 실행됨 (PID %d). 현재 프로세스 종료 준비...", cmd.Process.Pid)
			if onShutdown != nil {
				onShutdown()
			}
			os.Exit(0)
		}()
		return nil
	}

	if downloadURL == "" {
		progressMu.Lock()
		currentProgress.Active = false
		currentProgress.Error = "다운로드 URL을 찾을 수 없습니다."
		progressMu.Unlock()
		return fmt.Errorf("다운로드 URL이 비어 있습니다")
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				progressMu.Lock()
				currentProgress.Active = false
				currentProgress.Error = fmt.Sprintf("패닉 발생: %v", r)
				progressMu.Unlock()
			}
		}()

		client := &http.Client{Timeout: 10 * time.Minute}
		resp, err := client.Get(downloadURL)
		if err != nil {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("다운로드 실패: %v", err)
			progressMu.Unlock()
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("다운로드 서버 응답 오류 (HTTP %d)", resp.StatusCode)
			progressMu.Unlock()
			return
		}

		totalSize := resp.ContentLength
		if totalSize <= 0 {
			totalSize = expectedSize
		}

		tmpPath := installerPath + ".tmp"
		outFile, err := os.Create(tmpPath)
		if err != nil {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("임시 파일 생성 실패: %v", err)
			progressMu.Unlock()
			return
		}

		buf := make([]byte, 32*1024)
		var received int64
		startTime := time.Now()
		lastTick := time.Now()
		lastReceived := int64(0)

		for {
			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				_, wErr := outFile.Write(buf[:n])
				if wErr != nil {
					outFile.Close()
					_ = os.Remove(tmpPath)
					progressMu.Lock()
					currentProgress.Active = false
					currentProgress.Error = fmt.Sprintf("파일 쓰기 실패: %v", wErr)
					progressMu.Unlock()
					return
				}
				received += int64(n)

				now := time.Now()
				if now.Sub(lastTick) >= 150*time.Millisecond || rErr == io.EOF {
					elapsedTick := now.Sub(lastTick).Seconds()
					var speed int64
					if elapsedTick > 0 {
						speed = int64(float64(received-lastReceived) / elapsedTick)
					}
					lastTick = now
					lastReceived = received

					percent := 0
					if totalSize > 0 {
						percent = int((float64(received) / float64(totalSize)) * 100)
						if percent > 99 && rErr != io.EOF {
							percent = 99
						}
					}

					progressMu.Lock()
					currentProgress.Received = received
					currentProgress.Total = totalSize
					currentProgress.Percent = percent
					currentProgress.SpeedBps = speed
					progressMu.Unlock()
				}
			}

			if rErr != nil {
				if rErr == io.EOF {
					break
				}
				outFile.Close()
				_ = os.Remove(tmpPath)
				progressMu.Lock()
				currentProgress.Active = false
				currentProgress.Error = fmt.Sprintf("데이터 수신 오류: %v", rErr)
				progressMu.Unlock()
				return
			}
		}
		outFile.Close()

		// 온전하게 다운로드된 임시 파일을 정식 인스톨러 이름으로 확정
		_ = os.Remove(installerPath)
		if err := os.Rename(tmpPath, installerPath); err != nil {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("인스톨러 확정 실패: %v", err)
			progressMu.Unlock()
			return
		}

		progressMu.Lock()
		currentProgress.Percent = 100
		currentProgress.Done = true
		currentProgress.Active = false
		progressMu.Unlock()

		LogInfo("[Updater] 인스톨러 다운로드 완료 (%d bytes) -> %s (경과: %v)", received, installerPath, time.Since(startTime).Round(time.Millisecond))

		// 1초 후 인스톨러 실행 (/SILENT /SP-)
		time.Sleep(1 * time.Second)

		cmd := exec.Command(installerPath, "/SILENT", "/SP-")
		if err := cmd.Start(); err != nil {
			LogError("[Updater] 인스톨러 실행 실패: %v", err)
			progressMu.Lock()
			currentProgress.Error = fmt.Sprintf("설치기 실행 실패: %v", err)
			progressMu.Unlock()
			return
		}

		LogInfo("[Updater] 새 버전 인스톨러 실행됨 (PID %d). 현재 프로세스 종료 준비...", cmd.Process.Pid)
		if onShutdown != nil {
			onShutdown()
		}
		os.Exit(0)
	}()

	return nil
}

// StartDownloadAndInstall: ExecuteUpdate를 호출하는 헬퍼 함수
func StartDownloadAndInstall(version, downloadURL string, expectedSize int64, onShutdown func()) error {
	return ExecuteUpdate(version, downloadURL, expectedSize, onShutdown)
}
