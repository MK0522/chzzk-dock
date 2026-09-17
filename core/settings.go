package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// AppSettings: 애플리케이션의 비보안/일반 설정 모델
type AppSettings struct {
	WatchdogTimeoutSec   int   `json:"watchdog_timeout_sec"`             // OBS 미감지 시 자체 종료 대기 시간 (초, 0: 사용 안 함)
	WatchdogDisabled     bool  `json:"watchdog_disabled,omitempty"`     // OBS 자동 종료 비활성화 여부
	HttpPort             int   `json:"http_port"`                     // 기본 HTTP 서버 포트 (기본값: 8081)
	PopupOnStart         *bool `json:"popup_on_start,omitempty"`         // 프로그램 시작 시 웹뷰 팝업창 자동 열기 (기본값: false)
	NotifyOnShutdown     *bool `json:"notify_on_shutdown,omitempty"`     // OBS 종료/미감지로 자동 종료 시 Windows 알림 표시 (기본값: true)
	EnableGPU            *bool `json:"enable_gpu,omitempty"`             // 웹뷰 창 GPU 하드웨어 가속 여부 (기본값: true)
	ExternalBrowserGuard *bool `json:"external_browser_guard,omitempty"` // 네이버 외 외부 링크 클릭 시 기본 브라우저로 열기 (기본값: true)
	RemoteTesterUnlocked bool  `json:"remote_tester_unlocked,omitempty"` // 리모컨 테스터 인증 승인 여부
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

// SetEnableGPUSetting: 웹뷰 GPU 하드웨어 가속 여부 설정 저장
func SetEnableGPUSetting(val bool) error {
	st := LoadSettings()
	st.SetEnableGPU(val)
	return SaveSettings(st)
}

// GetExternalBrowserGuard: 네이버 외 외부 링크 클릭 시 기본 브라우저 열기 설정 조회
func GetExternalBrowserGuard() bool {
	s := LoadSettings()
	return s.IsExternalBrowserGuard()
}

// SetExternalBrowserGuardSetting: 네이버 외 외부 링크 클릭 시 기본 브라우저 열기 설정 저장
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
// 직접 레지스트리를 조회하지 않아 백신/EDR(행위 기반 탐지)의 오탐을 원천 차단합니다.
func GetDefaultBrowserName() string {
	assocPtr, _ := syscall.UTF16PtrFromString("https")
	extraPtr, _ := syscall.UTF16PtrFromString("open")

	buf := make([]uint16, 512)
	cch := uint32(len(buf))

	// 1차: ASSOCSTR_EXECUTABLE (2) - 기본 브라우저 실행 파일 전체 경로 조회
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
		WatchdogTimeoutSec:   60,                // 기본값: 1분 (60초)
		HttpPort:             8081,              // 기본값: 8081
		PopupOnStart:         &defaultPopupVal,  // 기본값: false
		NotifyOnShutdown:     &defaultNotifyVal, // 기본값: true
		EnableGPU:            &defaultGPUVal,    // 기본값: true
		ExternalBrowserGuard: &defaultGuardVal,  // 기본값: true
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
		if s.WatchdogDisabled || s.WatchdogTimeoutSec == 0 {
			s.WatchdogDisabled = true
			s.WatchdogTimeoutSec = 0
		} else if s.WatchdogTimeoutSec < 0 {
			s.WatchdogTimeoutSec = 60
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

	if s.WatchdogDisabled || s.WatchdogTimeoutSec == 0 {
		s.WatchdogDisabled = true
		s.WatchdogTimeoutSec = 0
	} else if s.WatchdogTimeoutSec < 0 {
		s.WatchdogTimeoutSec = 60
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