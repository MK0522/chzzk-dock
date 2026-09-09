package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppSettings: 애플리케이션의 비보안/일반 설정 모델
type AppSettings struct {
	WatchdogTimeoutSec int `json:"watchdog_timeout_sec"` // OBS 미감지 시 자체 종료 대기 시간 (초)
}

var (
	settingsMu     sync.RWMutex
	cachedSettings = AppSettings{
		WatchdogTimeoutSec: 60, // 기본값: 1분 (60초)
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
		cachedSettings = AppSettings{WatchdogTimeoutSec: 60}
		settingsLoaded = true
		return cachedSettings
	}

	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil || s.WatchdogTimeoutSec <= 0 {
		cachedSettings = AppSettings{WatchdogTimeoutSec: 60}
	} else {
		cachedSettings = s
	}
	settingsLoaded = true
	return cachedSettings
}

// SaveSettings: %LOCALAPPDATA%\ChzzkObsDock\settings.json 에 설정을 저장합니다.
func SaveSettings(s AppSettings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()

	if s.WatchdogTimeoutSec <= 0 {
		s.WatchdogTimeoutSec = 60
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

// ResetSettingsForTest: 단위 테스트용 캐시 초기화
func ResetSettingsForTest() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settingsLoaded = false
	cachedSettings = AppSettings{WatchdogTimeoutSec: 60}
}