package core

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	EnableBetaUpdates    *bool             `json:"enable_beta_updates,omitempty"`    // 테스트(Beta) 버전 업데이트 수신 여부 (기본값: false)
	AutoCheckUpdate      *bool             `json:"auto_check_update,omitempty"`      // 앱 시작 시 업데이트 자동 확인 여부 (기본값: true)
	FontScale            float64            `json:"font_scale"`                       // 독 화면/폰트 배율 (기본값: 1.2)
	ShowViewerCount      *bool              `json:"show_viewer_count,omitempty"`      // 상단 실시간 시청자 수 표시 여부 (기본값: true)
	SkipUpdateVersion    string             `json:"skip_update_version,omitempty"`    // 건너뛴 업데이트 버전 태그
	SkipScriptVersion    int                `json:"skip_script_version,omitempty"`    // 건너뛴 OBS Lua 스크립트 버전 번호
	LastCleanedVersion   string             `json:"last_cleaned_version,omitempty"`   // 마지막으로 레거시 잔재 청소를 완료한 앱 버전
	BroadcastPresets        []BroadcastPreset       `json:"broadcast_presets,omitempty"`          // 방송 정보 프리셋 목록 (최대 10개)
	CategoryFavorites       []CategoryFavorite      `json:"category_favorites,omitempty"`        // 카테고리 즐겨찾기 목록 (최대 8개)
	PreWatchPartySnapshot   *WatchPartySnapshot     `json:"pre_watch_party_snapshot,omitempty"`  // 같이보기 전환 직전 방송 옵션 원본 스냅샷
}

// WatchPartySnapshot: 같이보기 전환 직전의 방송 원본 옵션 데이터 모델
type WatchPartySnapshot struct {
	CategoryID     string `json:"categoryId"`
	CategoryType   string `json:"categoryType"`
	CategorySearch string `json:"categorySearch"`
	Adult          bool   `json:"adult"`
	Clip           bool   `json:"clip"`
	Overseas       bool   `json:"overseas"`
	Replay         string `json:"replay"`
	ChatDonation   bool   `json:"chatDonation"`
}

// CategoryFavorite: 카테고리 즐겨찾기 데이터 모델
type CategoryFavorite struct {
	CategoryID     string `json:"category_id"`
	CategoryName   string `json:"category_name"`
	CategoryType   string `json:"category_type,omitempty"`
	PosterImageUrl string `json:"poster_image_url,omitempty"`
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

func (s AppSettings) IsEnableBetaUpdates() bool {
	if s.EnableBetaUpdates == nil {
		return false
	}
	return *s.EnableBetaUpdates
}

func (s *AppSettings) SetEnableBetaUpdates(val bool) {
	s.EnableBetaUpdates = &val
}

func (s AppSettings) IsAutoCheckUpdate() bool {
	if s.AutoCheckUpdate == nil {
		return true // 기본값: 자동 확인 켜짐
	}
	return *s.AutoCheckUpdate
}

func (s *AppSettings) SetAutoCheckUpdate(val bool) {
	s.AutoCheckUpdate = &val
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

func (s *AppSettings) SetFontScale(val float64) {
	if val <= 0 {
		val = 1.2
	}
	s.FontScale = val
}

func (s AppSettings) IsShowViewerCount() bool {
	if s.ShowViewerCount == nil {
		return true
	}
	return *s.ShowViewerCount
}

func (s *AppSettings) SetShowViewerCount(val bool) {
	s.ShowViewerCount = &val
}

func (s AppSettings) GetSkipUpdateVersion() string {
	return s.SkipUpdateVersion
}

func (s *AppSettings) SetSkipUpdateVersion(val string) {
	s.SkipUpdateVersion = val
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
	defaultPopupVal     = false
	defaultNotifyVal    = true
	defaultGPUVal       = true
	defaultGuardVal     = true
	defaultAutoCheckVal = true
	settingsMu          sync.RWMutex
	cachedSettings      = AppSettings{
		SchemaVersion:        1,
		WatchdogTimeoutSec:   60,
		HttpPort:             8081,
		PopupOnStart:         &defaultPopupVal,
		NotifyOnShutdown:     &defaultNotifyVal,
		EnableGPU:            &defaultGPUVal,
		ExternalBrowserGuard: &defaultGuardVal,
		AutoCheckUpdate:      &defaultAutoCheckVal,
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
		if s.FontScale <= 0 {
			s.FontScale = 1.2
		}
		if s.ShowViewerCount == nil {
			defViewer := true
			s.ShowViewerCount = &defViewer
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

// IsPreRelease: 태그/버전 문자열에 접미사(-Beta, -RC 등) 또는 알파벳/기호가 붙어있는지 검사 (화이트리스트: 순수 vX.Y.Z만 정식 버전)
func IsPreRelease(v string) bool {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return true
	}
	// 순수 숫자(0-9)와 구분자 점(.) 이외의 문자(알파벳, 하이픈, 플러스 등)가 1자라도 포함되면 테스트 버전
	for _, r := range v {
		if (r < '0' || r > '9') && r != '.' {
			return true
		}
	}
	return false
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

// GetCategoryFavorites: 카테고리 즐겨찾기 목록 조회 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func GetCategoryFavorites() []CategoryFavorite {
	st := LoadSettings()
	if st.CategoryFavorites == nil {
		return []CategoryFavorite{}
	}
	return st.CategoryFavorites
}

// SaveCategoryFavorites: 카테고리 즐겨찾기 목록 저장 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func SaveCategoryFavorites(favs []CategoryFavorite) error {
	st := LoadSettings()
	if len(favs) > 8 {
		favs = favs[:8]
	}
	st.CategoryFavorites = favs
	return SaveSettings(st)
}

// GetWatchPartySnapshot: 같이보기 전환 직전 방송 옵션 원본 스냅샷 조회 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func GetWatchPartySnapshot() *WatchPartySnapshot {
	st := LoadSettings()
	return st.PreWatchPartySnapshot
}

// SaveWatchPartySnapshot: 같이보기 전환 직전 방송 옵션 원본 스냅샷 저장/초기화 (%LOCALAPPDATA%\ChzzkObsDock\settings.json)
func SaveWatchPartySnapshot(snap *WatchPartySnapshot) error {
	st := LoadSettings()
	st.PreWatchPartySnapshot = snap
	return SaveSettings(st)
}



// -----------------------------------------------------------------------------
// 2. 인앱 자동 업데이트 파이프라인 (Updater)
// -----------------------------------------------------------------------------

var (
	GithubReleasesAPI = "https://api.github.com/repos/MK0522/chzzk-dock/releases/latest"
)

// UpdateInfo: 최신 릴리즈 정보 DTO
type UpdateInfo struct {
	HasUpdate         bool   `json:"has_update"`
	ShouldPrompt      bool   `json:"should_prompt"`
	CurrentVersion    string `json:"current_version"`
	LatestVersion     string `json:"latest_version"`
	ReleaseTitle      string `json:"release_title"`
	ReleaseNotes      string `json:"release_notes"`
	ReleaseURL        string `json:"release_url"`
	DownloadURL       string `json:"download_url"`
	AssetSize         int64  `json:"asset_size"`
	AssetDigest       string `json:"asset_digest,omitempty"`
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

// SemVerInfo: 시맨틱 버전 상세 구조체
type SemVerInfo struct {
	Major        int
	Minor        int
	Patch        int
	IsPrerelease bool
	PreWeight    int
	PreNum       int
	PreRaw       string
}

// ParseSemVerInfo: 버전 문자열을 상세 SemVerInfo로 파싱
func ParseSemVerInfo(v string) SemVerInfo {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")

	// 빌드 메타데이터 (+ 이후) 분리
	if idx := strings.Index(v, "+"); idx != -1 {
		v = v[:idx]
	}

	var preStr string
	// 하이픈(-) 또는 언더스코어(_)로 사전 릴리즈 분리
	if idx := strings.IndexAny(v, "-_"); idx != -1 {
		preStr = v[idx+1:]
		v = v[:idx]
	}

	// X.Y.Z 메이저/마이너/패치 숫자 파싱 (v 접두사 및 사전릴리즈 접미사가 분리된 순수 버전 번호)
	parts := strings.Split(v, ".")
	var nums [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		if n, err := strconv.Atoi(parts[i]); err == nil {
			nums[i] = n
		}
	}

	res := SemVerInfo{
		Major: nums[0],
		Minor: nums[1],
		Patch: nums[2],
	}

	preStr = strings.TrimSpace(preStr)
	if preStr == "" {
		// 사전 릴리즈 접미사가 없으면 정식 버전!
		return res
	}

	res.IsPrerelease = true
	res.PreRaw = strings.ToLower(preStr)

	// 사전 릴리즈 단계 판별 (alpha < beta < rc)
	switch {
	case strings.Contains(res.PreRaw, "alpha"):
		res.PreWeight = 10
	case strings.Contains(res.PreRaw, "beta"):
		res.PreWeight = 20
	case strings.Contains(res.PreRaw, "rc"):
		res.PreWeight = 30
	default:
		res.PreWeight = 5
	}

	// 사전 릴리즈 시퀀스 번호 추출 (예: "alpha1" -> 1, "beta2" -> 2, "rc.1" -> 1, "beta10" -> 10)
	// 의도: 사전 릴리즈 식별자(alpha/beta/rc 등) 앞쪽의 텍스트와 구분 기호(.)를 잘라내고 남은 순수 번호를 정수형으로 파싱하여
	// 문자열 정렬 오류("beta10" < "beta2")를 방지하고 정확한 크기 비교를 수행함. 번호가 없는 단독 "beta"는 0으로 처리.
	numStr := strings.TrimLeftFunc(res.PreRaw, func(r rune) bool {
		return r < '0' || r > '9'
	})
	if n, err := strconv.Atoi(numStr); err == nil {
		res.PreNum = n
	}

	return res
}



// CompareSemVer: v1 < v2 (-1), v1 == v2 (0), v1 > v2 (1)
// SemVer 2.0 규칙 적용: Major.Minor.Patch 비교 후, 동일할 경우 정식 버전(접미사 없음)이 사전 릴리즈보다 우선
func CompareSemVer(v1, v2 string) int {
	sv1 := ParseSemVerInfo(v1)
	sv2 := ParseSemVerInfo(v2)

	// Major.Minor.Patch 기본 비교
	if c := slices.Compare([]int{sv1.Major, sv1.Minor, sv1.Patch}, []int{sv2.Major, sv2.Minor, sv2.Patch}); c != 0 {
		return c
	}

	// Major.Minor.Patch가 완전히 같은 경우:
	// 1. 사전 릴리즈 여부 비교 (SemVer 2.0: 정식 버전 > 사전 릴리즈 버전)
	if !sv1.IsPrerelease && sv2.IsPrerelease {
		return 1 // v1은 정식 버전, v2는 사전 릴리즈 -> v1이 더 최신
	}
	if sv1.IsPrerelease && !sv2.IsPrerelease {
		return -1 // v1은 사전 릴리즈, v2는 정식 버전 -> v2가 더 최신
	}
	if !sv1.IsPrerelease && !sv2.IsPrerelease {
		return 0 // 둘 다 정식 버전이고 숫자가 같음
	}

	// 2. 둘 다 사전 릴리즈인 경우:
	// 단계 비교 (alpha < beta < rc)
	if c := cmp.Compare(sv1.PreWeight, sv2.PreWeight); c != 0 {
		return c
	}

	// 동일 단계 내에서 번호 비교 (beta1 < beta2 < beta10)
	if c := cmp.Compare(sv1.PreNum, sv2.PreNum); c != 0 {
		return c
	}

	// 단계와 번호까지 같으면 사전 릴리즈 원본 문자열 비교
	if sv1.PreRaw != sv2.PreRaw {
		return strings.Compare(sv1.PreRaw, sv2.PreRaw)
	}

	return 0
}

func getInstallerTempPath(version string) string {
	cleanVer := strings.TrimPrefix(strings.TrimSpace(version), "v")
	return filepath.Join(os.TempDir(), fmt.Sprintf("chzzk-dock-setup-%s.exe", cleanVer))
}

// CleanOldInstallers: 현재 버전과 다른 이전 임시 인스톨러 및 중단된 임시 파일(.tmp) 정리
func CleanOldInstallers(currentVersion string) {
	tempDir := os.TempDir()
	keepName := filepath.Base(getInstallerTempPath(currentVersion))
	if matches, err := filepath.Glob(filepath.Join(tempDir, "chzzk-dock-setup-*.exe*")); err == nil {
		for _, m := range matches {
			if filepath.Base(m) != keepName {
				_ = os.Remove(m)
			}
		}
	}
}

// CleanupLegacyArtifactsIfNeeded: 버전 변경 후 최초 1회 실행 시 구버전 임시 파일 및 레거시 잔재 일괄 청소
func CleanupLegacyArtifactsIfNeeded(currentVersion string) {
	exePath, _ := os.Executable()
	var exeDir string
	if exePath != "" {
		exeDir = filepath.Dir(exePath)
	}
	cleanupLegacyArtifactsInDir(currentVersion, exeDir)
}

func removeRequiredPath(path string) bool {
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		LogWarn("[Cleanup] 필수 잔재 삭제 실패 (다음 실행 시 재시도): %s (%v)", path, err)
		return false
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		LogWarn("[Cleanup] 삭제 시도 후에도 경로가 남아 있음 (다음 실행 시 재시도): %s", path)
		return false
	}
	return true
}

func cleanupLegacyArtifactsInDir(currentVersion string, exeDir string) {
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion == "" {
		return
	}
	st := LoadSettings()
	if st.LastCleanedVersion == currentVersion {
		return
	}

	allRequiredRemoved := true
	checkRemove := func(path string) {
		if !removeRequiredPath(path) {
			allRequiredRemoved = false
		}
	}

	tempDir := os.TempDir()
	checkRemove(filepath.Join(tempDir, "chzzk_dock_temp_icon.ico"))
	if matches, err := filepath.Glob(filepath.Join(tempDir, "chzzk-dock-setup-*.exe*")); err == nil {
		for _, m := range matches {
			checkRemove(m)
		}
	}

	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	if appData != "" {
		// 필수 삭제 대상: 폐기된 테스트 프로필 및 과거 오설치 실행 파일
		checkRemove(filepath.Join(appData, "ChzzkObsDock", "webview_profile_test"))
		checkRemove(filepath.Join(appData, "chzzk-dock.exe"))
		checkRemove(filepath.Join(appData, "CHZZK OBS Dock", "chzzk-dock.exe"))

		// 최선 시도(Best-effort) 대상: 현재 사용 중인 프로필의 WebView2 캐시는 런타임 중 잠길 수 있으므로 완료 판정에서 제외
		_ = os.RemoveAll(filepath.Join(appData, "ChzzkObsDock", "dock_profile", "EBWebView", "Default", "Cache"))
		_ = os.RemoveAll(filepath.Join(appData, "ChzzkObsDock", "dock_profile", "EBWebView", "Default", "Code Cache"))
		_ = os.RemoveAll(filepath.Join(appData, "ChzzkObsDock", "webview_profile", "EBWebView", "Default", "Cache"))
		_ = os.RemoveAll(filepath.Join(appData, "ChzzkObsDock", "webview_profile", "EBWebView", "Default", "Code Cache"))
	}

	// 개발 폴더(예: chzzk-dock)에서 실행된 경우 배포 폴더 정리를 생략하고 완료 마커(LastCleanedVersion)도 갱신하지 않음
	isProdInstallDir := exeDir != "" && strings.EqualFold(filepath.Base(exeDir), "CHZZK OBS Dock")
	if !isProdInstallDir {
		return
	}

	checkRemove(filepath.Join(exeDir, "ui"))
	checkRemove(filepath.Join(exeDir, "chzzk_dock_launcher.lua"))
	checkRemove(filepath.Join(exeDir, "chzzk-obs-dock.exe"))

	if !allRequiredRemoved {
		return
	}

	st = LoadSettings()
	st.LastCleanedVersion = currentVersion
	if err := SaveSettings(st); err == nil {
		LogInfo("[Cleanup] 버전(%s) 마이그레이션 및 레거시 잔재 1회 정리 완료", currentVersion)
	}
}

func normalizeSHA256Hex(digest string) string {
	d := strings.ToLower(strings.TrimSpace(digest))
	d = strings.TrimPrefix(d, "sha256:")
	return strings.TrimSpace(d)
}

func verifyFileSHA256(filePath, expectedDigest string) bool {
	expectedHex := normalizeSHA256Hex(expectedDigest)
	if expectedHex == "" {
		return false
	}
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	actualHex := hex.EncodeToString(h.Sum(nil))
	return strings.EqualFold(actualHex, expectedHex)
}

// CheckForUpdate: GitHub Releases API 비동기 조회 (10분 캐시)
func CheckForUpdate(currentVersion string, force bool) (*UpdateInfo, error) {
	updateCacheMu.Lock()
	defer updateCacheMu.Unlock()

	allowBeta := LoadSettings().IsEnableBetaUpdates()

	if !force && cachedUpdate != nil && time.Since(lastCheckTime) < 10*time.Minute {
		cachedCopy := *cachedUpdate
		// 🛡️ 캐시 유효성 재검증: 베타 옵션이 꺼져있는데 캐시된 대상이 테스트 버전이면 강제 차단
		if !allowBeta && IsPreRelease(cachedCopy.LatestVersion) {
			cachedCopy.HasUpdate = false
		}
		cachedCopy.AlreadyDownloaded = isInstallerRecorded(cachedCopy.LatestVersion, cachedCopy.AssetDigest)
		return &cachedCopy, nil
	}

	client := &http.Client{Timeout: 8 * time.Second}

	type ghReleaseDTO struct {
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		Body       string `json:"body"`
		HTMLURL    string `json:"html_url"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
		Assets     []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			Digest             string `json:"digest"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	var targetRelease ghReleaseDTO
	apiURL := GithubReleasesAPI
	if allowBeta {
		apiURL = "https://api.github.com/repos/MK0522/chzzk-dock/releases?per_page=5"
	}

	req, err := http.NewRequest("GET", apiURL, nil)
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

	if allowBeta {
		var releases []ghReleaseDTO
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, fmt.Errorf("릴리즈 응답 파싱 실패: %w", err)
		}
		if len(releases) == 0 {
			return &UpdateInfo{HasUpdate: false, CurrentVersion: currentVersion}, nil
		}
		targetRelease = releases[0]
	} else {
		if err := json.NewDecoder(resp.Body).Decode(&targetRelease); err != nil {
			return nil, fmt.Errorf("릴리즈 응답 파싱 실패: %w", err)
		}
	}

	latestVer := targetRelease.TagName
	hasUpdate := CompareSemVer(currentVersion, latestVer) < 0

	// 🛡️ 절대 보안 원칙: 베타 업데이트 옵션이 꺼져있는 경우, 대상 버전이 Pre-release이거나
	// 버전명에 beta/alpha/rc/test 등이 포함되어 있으면 절대 업데이트로 인정하지 않음!
	if !allowBeta && (targetRelease.Prerelease || IsPreRelease(latestVer)) {
		hasUpdate = false
	}

	var downloadURL string
	var assetSize int64
	var assetDigest string
	for _, asset := range targetRelease.Assets {
		name := strings.ToLower(asset.Name)
		if strings.HasSuffix(name, ".exe") && (strings.Contains(name, "setup") || strings.Contains(name, "installer")) {
			downloadURL = asset.BrowserDownloadURL
			assetSize = asset.Size
			assetDigest = strings.TrimSpace(asset.Digest)
			break
		}
	}

	CleanOldInstallers(latestVer)

	shouldPrompt := hasUpdate && (LoadSettings().GetSkipUpdateVersion() != latestVer)

	info := &UpdateInfo{
		HasUpdate:         hasUpdate,
		ShouldPrompt:      shouldPrompt,
		CurrentVersion:    currentVersion,
		LatestVersion:     latestVer,
		ReleaseTitle:      targetRelease.Name,
		ReleaseNotes:      targetRelease.Body,
		ReleaseURL:        targetRelease.HTMLURL,
		DownloadURL:       downloadURL,
		AssetSize:         assetSize,
		AssetDigest:       assetDigest,
		AlreadyDownloaded: isInstallerRecorded(latestVer, assetDigest),
	}

	cachedUpdate = info
	lastCheckTime = time.Now()

	return info, nil
}

// Stores verified installer digests by path for the lifetime of the app.
var verifiedInstallers sync.Map

func isInstallerRecorded(version, expectedDigest string) bool {
	digest := normalizeSHA256Hex(expectedDigest)
	if version == "" || digest == "" {
		return false
	}
	path := getInstallerTempPath(version)
	if recorded, ok := verifiedInstallers.Load(path); !ok || recorded != digest {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func isInstallerReady(version string, expectedDigest string) bool {
	if version == "" || normalizeSHA256Hex(expectedDigest) == "" {
		return false
	}
	path := getInstallerTempPath(version)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	if !verifyFileSHA256(path, expectedDigest) {
		verifiedInstallers.Delete(path)
		_ = os.Remove(path)
		return false
	}
	verifiedInstallers.Store(path, normalizeSHA256Hex(expectedDigest))
	return true
}

// GetUpdateProgress: 현재 진행률 조회
func GetUpdateProgress() UpdateProgress {
	progressMu.RLock()
	defer progressMu.RUnlock()
	return currentProgress
}

// ExecuteUpdate: 인스톨러 다운로드 및 백그라운드 설치 실행
func ExecuteUpdate(version, downloadURL string, expectedSize int64, expectedDigest string, onShutdown func()) error {
	if !LoadSettings().IsEnableBetaUpdates() && IsPreRelease(version) {
		return fmt.Errorf("테스트 버전(Beta) 업데이트 옵션이 꺼져 있어 설치할 수 없습니다")
	}
	if normalizeSHA256Hex(expectedDigest) == "" {
		return fmt.Errorf("릴리즈 해시(SHA-256) 검증 정보가 없어 업데이트를 진행할 수 없습니다")
	}
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
	ready := isInstallerReady(version, expectedDigest)

	if !ready && downloadURL == "" {
		progressMu.Lock()
		currentProgress.Active = false
		currentProgress.Error = "다운로드 URL이 비어 있습니다"
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

		if ready {
			LogInfo("[Updater] 기존에 다운로드 완료된 인스톨러 사용: %s", installerPath)
			progressMu.Lock()
			currentProgress.Percent = 100
			currentProgress.Received = expectedSize
			currentProgress.Total = expectedSize
			currentProgress.Active = true
			currentProgress.Done = false
			progressMu.Unlock()
		} else {
			client := &http.Client{Timeout: 10 * time.Minute}
			tmpPath := installerPath + ".tmp"
			expectedHex := normalizeSHA256Hex(expectedDigest)
			const maxAttempts = 3 // 최초 1회 + 최대 2회 재다운로드 시도
			var lastErrMsg string

			for attempt := 1; attempt <= maxAttempts; attempt++ {
				if attempt > 1 {
					LogWarn("[Updater] 다운로드 재시도 중 (%d/%d)...", attempt-1, maxAttempts-1)
					progressMu.Lock()
					currentProgress.Received = 0
					currentProgress.Percent = 0
					currentProgress.SpeedBps = 0
					progressMu.Unlock()
					time.Sleep(1 * time.Second)
				}

				resp, err := client.Get(downloadURL)
				if err != nil {
					lastErrMsg = fmt.Sprintf("다운로드 실패: %v", err)
					LogWarn("[Updater] 시도 %d/%d 오류: %s", attempt, maxAttempts, lastErrMsg)
					continue
				}

				if resp.StatusCode != http.StatusOK {
					_ = resp.Body.Close()
					lastErrMsg = fmt.Sprintf("다운로드 서버 응답 오류 (HTTP %d)", resp.StatusCode)
					LogWarn("[Updater] 시도 %d/%d 오류: %s", attempt, maxAttempts, lastErrMsg)
					continue
				}

				totalSize := resp.ContentLength
				if totalSize <= 0 {
					totalSize = expectedSize
				}

				outFile, err := os.Create(tmpPath)
				if err != nil {
					_ = resp.Body.Close()
					lastErrMsg = fmt.Sprintf("임시 파일 생성 실패: %v", err)
					LogWarn("[Updater] 시도 %d/%d 오류: %s", attempt, maxAttempts, lastErrMsg)
					continue
				}

				buf := make([]byte, 32*1024)
				hasher := sha256.New()
				var received int64
				startTime := time.Now()
				lastTick := time.Now()
				lastReceived := int64(0)
				var streamErr error

				for {
					n, rErr := resp.Body.Read(buf)
					if n > 0 {
						_, wErr := outFile.Write(buf[:n])
						if wErr != nil {
							streamErr = fmt.Errorf("파일 쓰기 실패: %v", wErr)
							break
						}
						_, _ = hasher.Write(buf[:n])
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
						streamErr = fmt.Errorf("데이터 수신 오류: %v", rErr)
						break
					}
				}
				_ = outFile.Close()
				_ = resp.Body.Close()

				if streamErr != nil {
					_ = os.Remove(tmpPath)
					lastErrMsg = streamErr.Error()
					LogWarn("[Updater] 시도 %d/%d 오류: %s", attempt, maxAttempts, lastErrMsg)
					continue
				}

				actualHex := hex.EncodeToString(hasher.Sum(nil))
				if expectedHex == "" || !strings.EqualFold(actualHex, expectedHex) {
					_ = os.Remove(tmpPath)
					LogError("[Updater] 다운로드 파일 해시(SHA-256) 불일치 (시도 %d/%d, 기대: %s, 실제: %s)", attempt, maxAttempts, expectedHex, actualHex)
					lastErrMsg = "다운로드된 설치 파일의 무결성(SHA-256 해시) 검증에 실패했습니다."
					continue
				}

				verifiedInstallers.Delete(installerPath)
				// 해시 검증이 완료된 임시 파일을 정식 인스톨러 이름으로 확정
				_ = os.Remove(installerPath)
				if err := os.Rename(tmpPath, installerPath); err != nil {
					_ = os.Remove(tmpPath)
					lastErrMsg = fmt.Sprintf("인스톨러 확정 실패: %v", err)
					LogWarn("[Updater] 시도 %d/%d 오류: %s", attempt, maxAttempts, lastErrMsg)
					continue
				}

				progressMu.Lock()
				currentProgress.Percent = 100
				currentProgress.Active = true
				currentProgress.Done = false
				progressMu.Unlock()

				verifiedInstallers.Store(installerPath, expectedHex)
				LogInfo("[Updater] 인스톨러 다운로드 및 해시 검증 완료 (%d bytes, 시도 %d/%d) -> %s (경과: %v)", received, attempt, maxAttempts, installerPath, time.Since(startTime).Round(time.Millisecond))
				lastErrMsg = ""
				break
			}

			if lastErrMsg != "" {
				_ = os.Remove(tmpPath)
				progressMu.Lock()
				currentProgress.Active = false
				currentProgress.Done = false
				currentProgress.Error = fmt.Sprintf("%s (재다운로드 2회 시도 후 실패)", lastErrMsg)
				progressMu.Unlock()
				return
			}

			time.Sleep(1 * time.Second)
			if !isInstallerReady(version, expectedDigest) {
				LogError("[Updater] 설치 직전 파일 무결성 확인 실패: %s", installerPath)
				progressMu.Lock()
				currentProgress.Active = false
				currentProgress.Error = "설치 직전 파일의 무결성 확인에 실패했습니다."
				progressMu.Unlock()
				return
			}
		}

		cmd := exec.Command(installerPath, "/SILENT", "/SP-")
		DetachFromJob(cmd)
		if err := cmd.Start(); err != nil {
			LogError("[Updater] 인스톨러 실행 실패: %v", err)
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Done = false
			currentProgress.Error = fmt.Sprintf("설치기 실행 실패: %v", err)
			progressMu.Unlock()
			return
		}

		progressMu.Lock()
		currentProgress.Percent = 100
		currentProgress.Done = true
		currentProgress.Active = false
		progressMu.Unlock()

		LogInfo("[Updater] 새 버전 인스톨러 실행됨 (PID %d). 현재 프로세스 종료 준비...", cmd.Process.Pid)
		if onShutdown != nil {
			onShutdown()
		}
		os.Exit(0)
	}()

	return nil
}
