package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppSettings: 애플리케이션의 비보안/일반 설정 모델
type AppSettings struct {
	WatchdogTimeoutSec int   `json:"watchdog_timeout_sec"` // OBS 미감지 시 자체 종료 대기 시간 (초, 0: 사용 안 함)
	WatchdogDisabled   bool  `json:"watchdog_disabled,omitempty"` // OBS 자동 종료 비활성화 여부
	HttpPort           int   `json:"http_port"`             // 기본 HTTP 서버 포트 (기본값: 8081)
	PopupOnStart       *bool `json:"popup_on_start,omitempty"` // 프로그램 시작 시 웹뷰 팝업창 자동 열기 (기본값: false)
	NotifyOnShutdown   *bool `json:"notify_on_shutdown,omitempty"` // OBS 종료/미감지로 자동 종료 시 Windows 알림 표시 (기본값: true)
	RemoteTesterUnlocked bool `json:"remote_tester_unlocked,omitempty"` // 리모컨 테스터 인증 승인 여부
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

var (
	defaultPopupVal  = false
	defaultNotifyVal = true
	settingsMu       sync.RWMutex
	cachedSettings   = AppSettings{
		WatchdogTimeoutSec: 60,               // 기본값: 1분 (60초)
		HttpPort:           8081,             // 기본값: 8081
		PopupOnStart:       &defaultPopupVal, // 기본값: false
		NotifyOnShutdown:   &defaultNotifyVal, // 기본값: true
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
	cachedSettings = AppSettings{WatchdogTimeoutSec: 60, HttpPort: 8081, PopupOnStart: &defPop, NotifyOnShutdown: &defNotif}
}