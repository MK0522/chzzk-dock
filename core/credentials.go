package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	advapi32        = syscall.NewLazyDLL("advapi32.dll")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

const (
	CRED_TYPE_GENERIC          = 1
	CRED_PERSIST_LOCAL_MACHINE = 2
	CRED_TARGET_NAME           = "ChzzkObsDock/NaverSession"
)

type FILETIME struct {
	DwLowDateTime  uint32
	DwHighDateTime uint32
}

// CREDENTIALW: Windows advapi32 Credential 구조체 매핑
type CREDENTIALW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        FILETIME
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// SessionToken: 순수 네이버 세션 쿠키 자격 증명 모델 (단일 세션)
type SessionToken struct {
	NidAut string `json:"nid_aut"`
	NidSes string `json:"nid_ses"`
}

// Config: 네이버 세션 쿠키 자격 증명 모델 (하위 호환 활성 필드 및 세션 목록 포함)
type Config struct {
	NidAut   string         `json:"nid_aut"`
	NidSes   string         `json:"nid_ses"`
	Sessions []SessionToken `json:"sessions,omitempty"`
}

var (
	credLock       sync.Mutex
	cachedConfig   *Config
	cachedConfigMu sync.RWMutex
)

// InvalidateConfigCache: 메모리 캐시를 무효화하여 최신 자격 증명을 다시 로드하도록 강제합니다.
func InvalidateConfigCache() {
	cachedConfigMu.Lock()
	cachedConfig = nil
	cachedConfigMu.Unlock()
}

// CredWriteSessions: Windows Credential Manager 시스템 보안 금고에 순수 세션 배열 JSON 직렬화 저장
func CredWriteSessions(targetName string, sessions []SessionToken) bool {
	credLock.Lock()
	defer credLock.Unlock()

	blobBytes, err := json.Marshal(sessions)
	if err != nil {
		return false
	}

	targetNamePtr, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return false
	}

	commentPtr, _ := syscall.UTF16PtrFromString("CHZZK OBS Dock Naver Session Credentials")
	userNamePtr, _ := syscall.UTF16PtrFromString("ChzzkObsDockUser")

	var blobPtr *byte
	if len(blobBytes) > 0 {
		blobPtr = &blobBytes[0]
	}

	cred := CREDENTIALW{
		Flags:              0,
		Type:               CRED_TYPE_GENERIC,
		TargetName:         targetNamePtr,
		Comment:            commentPtr,
		CredentialBlobSize: uint32(len(blobBytes)),
		CredentialBlob:     blobPtr,
		Persist:            CRED_PERSIST_LOCAL_MACHINE,
		UserName:           userNamePtr,
	}

	r1, _, _ := procCredWriteW.Call(
		uintptr(unsafe.Pointer(&cred)),
		0,
	)

	return r1 != 0
}

// CredReadSessions: Windows Credential Manager에서 세션 목록 로드 (레거시 단일 객체 감지 및 자동 마이그레이션 지원)
func CredReadSessions(targetName string) []SessionToken {
	credLock.Lock()
	defer credLock.Unlock()

	targetNamePtr, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return nil
	}

	var pCred *CREDENTIALW
	r1, _, _ := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetNamePtr)),
		uintptr(CRED_TYPE_GENERIC),
		0,
		uintptr(unsafe.Pointer(&pCred)),
	)

	if r1 == 0 || pCred == nil {
		return nil
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pCred)))

	if pCred.CredentialBlobSize == 0 || pCred.CredentialBlob == nil {
		return nil
	}

	rawBytes := unsafe.Slice(pCred.CredentialBlob, pCred.CredentialBlobSize)
	trimmed := strings.TrimSpace(string(rawBytes))
	if len(trimmed) == 0 {
		return nil
	}

	// [과도기 마이그레이션] 시작 기호('{')로 레거시 객체 감지 시 배열로 자동 변환하여 덮어쓰기
	if trimmed[0] == '{' {
		var legacy struct {
			NidAut string `json:"nid_aut"`
			NidSes string `json:"nid_ses"`
		}
		if err := json.Unmarshal([]byte(trimmed), &legacy); err == nil {
			aut := strings.TrimSpace(legacy.NidAut)
			ses := strings.TrimSpace(legacy.NidSes)
			if aut != "" || ses != "" {
				migrated := []SessionToken{{NidAut: aut, NidSes: ses}}
				blobBytes, _ := json.Marshal(migrated)
				var blobPtr *byte
				if len(blobBytes) > 0 {
					blobPtr = &blobBytes[0]
				}
				pCred.CredentialBlobSize = uint32(len(blobBytes))
				pCred.CredentialBlob = blobPtr
				procCredWriteW.Call(uintptr(unsafe.Pointer(pCred)), 0)
				LogInfo("[Credentials] 레거시 단일 세션 감지 -> 신규 세션 배열 스키마로 자동 마이그레이션 완료")
				return migrated
			}
		}
		return nil
	}

	// 신규 배열 규격 파싱 ('[' 로 시작)
	var sessions []SessionToken
	if err := json.Unmarshal([]byte(trimmed), &sessions); err != nil {
		return nil
	}
	return sessions
}


// CredDelete: Windows Credential Manager 시스템 보안 금고에서 대상 자격 증명 삭제
func CredDelete(targetName string) bool {
	credLock.Lock()
	defer credLock.Unlock()

	targetNamePtr, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return false
	}

	r1, _, _ := procCredDeleteW.Call(
		uintptr(unsafe.Pointer(targetNamePtr)),
		uintptr(CRED_TYPE_GENERIC),
		0,
	)

	return r1 != 0
}

// GetTargetName: Windows Credential Manager 타깃 이름 반환
func GetTargetName() string {
	if val := os.Getenv("CHZZK_CRED_TARGET"); val != "" {
		return val
	}
	return CRED_TARGET_NAME
}

// LoadConfig: 세션 목록 로드 및 현재 활성(0번째) 세션 매핑
func LoadConfig() Config {
	cachedConfigMu.RLock()
	if cachedConfig != nil {
		cfg := *cachedConfig
		cachedConfigMu.RUnlock()
		return cfg
	}
	cachedConfigMu.RUnlock()

	cfg := Config{NidAut: "", NidSes: "", Sessions: nil}
	sessions := CredReadSessions(GetTargetName())
	if len(sessions) > 0 {
		cfg.Sessions = sessions
		cfg.NidAut = sessions[0].NidAut
		cfg.NidSes = sessions[0].NidSes
	}

	cachedConfigMu.Lock()
	cachedConfig = &cfg
	cachedConfigMu.Unlock()

	return cfg
}

// SaveConfig: 신규 세션 추가 또는 활성화 (동일 토큰이면 활성화, 새 토큰이면 맨 앞 추가)
func SaveConfig(newData map[string]interface{}) Config {
	target := GetTargetName()
	var newAut, newSes string
	if newData != nil {
		if aut, ok := newData["nid_aut"].(string); ok {
			newAut = strings.TrimSpace(aut)
		}
		if ses, ok := newData["nid_ses"].(string); ok {
			newSes = strings.TrimSpace(ses)
		}
	}

	sessions := CredReadSessions(target)
	if newAut != "" || newSes != "" {
		newToken := SessionToken{NidAut: newAut, NidSes: newSes}
		foundIdx := -1
		for i, s := range sessions {
			if s.NidAut == newAut && s.NidSes == newSes {
				foundIdx = i
				break
			}
		}

		if foundIdx == 0 {
			// 이미 활성 세션임
		} else if foundIdx > 0 {
			// 이미 존재하는 세션이면 0번째로 이동 (활성화)
			sessions = append([]SessionToken{sessions[foundIdx]}, append(sessions[:foundIdx], sessions[foundIdx+1:]...)...)
		} else {
			// 새 세션이면 맨 앞(0번)에 삽입
			sessions = append([]SessionToken{newToken}, sessions...)
		}
		CredWriteSessions(target, sessions)
	}

	InvalidateConfigCache()
	return LoadConfig()
}

// SwitchSession: index번째 세션을 활성(0번) 세션으로 전환
func SwitchSession(index int) bool {
	target := GetTargetName()
	sessions := CredReadSessions(target)
	if index < 0 || index >= len(sessions) {
		return false
	}
	if index != 0 {
		targetSession := sessions[index]
		sessions = append([]SessionToken{targetSession}, append(sessions[:index], sessions[index+1:]...)...)
		CredWriteSessions(target, sessions)
	}
	InvalidateConfigCache()
	return true
}

// RemoveSession: index번째 세션 삭제
func RemoveSession(index int) bool {
	target := GetTargetName()
	sessions := CredReadSessions(target)
	if index < 0 || index >= len(sessions) {
		return false
	}
	sessions = append(sessions[:index], sessions[index+1:]...)
	if len(sessions) == 0 {
		CredDelete(target)
	} else {
		CredWriteSessions(target, sessions)
	}
	InvalidateConfigCache()
	return true
}

// ClearConfig: 현재 활성 세션 삭제 (다중 세션일 경우 현재 활성만 제거)
func ClearConfig() bool {
	target := GetTargetName()
	sessions := CredReadSessions(target)
	if len(sessions) <= 1 {
		CredDelete(target)
	} else {
		sessions = sessions[1:]
		CredWriteSessions(target, sessions)
	}
	InvalidateConfigCache()
	return true
}

// cookieCapturingTransport: HTTP 리다이렉트 과정의 모든 홉에서 Set-Cookie 헤더를 누락 없이 캡처하는 커스텀 트랜스포트
type cookieCapturingTransport struct {
	transport http.RoundTripper
	captured  map[string]string
	mu        sync.Mutex
}

func (t *cookieCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, cookie := range resp.Cookies() {
		t.captured[cookie.Name] = cookie.Value
	}
	return resp, nil
}

// RefreshNaverSession: 장기 보관된 NID_AUT 쿠키를 사용하여 최신 NID_SES 세션 쿠키를 무중단 자동 갱신합니다.
// 성공 시 새 쿠키를 Windows Credential Manager에 안전하게 저장하고 메모리 캐시를 즉시 갱신합니다.
func RefreshNaverSession(nidAut string) (newAut, newSes string, err error) {
	cleanAut := strings.TrimSpace(nidAut)
	if cleanAut == "" {
		return "", "", fmt.Errorf("유효한 NID_AUT 쿠키가 없습니다")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", "", fmt.Errorf("cookiejar 생성 실패: %w", err)
	}

	capTransport := &cookieCapturingTransport{
		transport: http.DefaultTransport,
		captured:  make(map[string]string),
	}

	client := &http.Client{
		Jar:       jar,
		Transport: capTransport,
		Timeout:   10 * time.Second,
	}

	nidURL, _ := url.Parse("https://nid.naver.com")
	jar.SetCookies(nidURL, []*http.Cookie{
		{
			Name:   "NID_AUT",
			Value:  cleanAut,
			Domain: ".naver.com",
			Path:   "/",
		},
	})

	refreshURL := "https://nid.naver.com/nidlogin.login?url=https%3A%2F%2Fchzzk.naver.com%2F"
	req, err := http.NewRequest("GET", refreshURL, nil)
	if err != nil {
		return "", "", fmt.Errorf("갱신 요청 생성 실패: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("네이버 세션 갱신 요청 통신 실패: %w", err)
	}
	defer resp.Body.Close()

	// 1. 트랜스포트에서 전 구간 캡처된 쿠키 확인
	capTransport.mu.Lock()
	capturedSes := capTransport.captured["NID_SES"]
	capturedAut := capTransport.captured["NID_AUT"]
	capTransport.mu.Unlock()

	// 2. 혹시 Jar 내부에 남아있는 쿠키 확인 (보조)
	if capturedSes == "" {
		for _, uStr := range []string{"https://nid.naver.com", "https://naver.com", "https://chzzk.naver.com"} {
			u, _ := url.Parse(uStr)
			for _, c := range jar.Cookies(u) {
				if c.Name == "NID_SES" && c.Value != "" {
					capturedSes = c.Value
				}
				if c.Name == "NID_AUT" && c.Value != "" {
					capturedAut = c.Value
				}
			}
		}
	}

	if capturedSes == "" {
		return "", "", fmt.Errorf("네이버 세션 갱신 실패: NID_SES가 발급되지 않았습니다 (NID_AUT 만료 또는 재로그인 필요)")
	}

	if capturedAut == "" {
		capturedAut = cleanAut
	}

	// 자격 증명 관리자에 안전하게 자동 저장
	SaveConfig(map[string]interface{}{
		"nid_aut": capturedAut,
		"nid_ses": capturedSes,
	})
	LogInfo("[Auth] 네이버 세션 쿠키 자동 갱신 완료 (NID_SES 발급 성공)")

	return capturedAut, capturedSes, nil
}

// CheckAndRefreshSessionOnStartup: 프로그램 시작 시 백그라운드에서 세션 유효성을 점검하고 만료 시 자동 갱신합니다.
func CheckAndRefreshSessionOnStartup(nidAut, nidSes string) {
	if strings.TrimSpace(nidAut) == "" {
		return
	}

	// NID_SES가 비어있으면 즉시 자동 갱신 시도
	if strings.TrimSpace(nidSes) == "" {
		LogInfo("[Auth] NID_SES 미존재 감지 -> 시작 시 자동 세션 갱신 시도")
		_, _, _ = RefreshNaverSession(nidAut)
		return
	}

	// NID_SES가 유효한지 사용자 상태 API(/nng_main/v1/user/getUserStatus)로 가볍게 점검
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://comm-api.game.naver.com/nng_main/v1/user/getUserStatus", nil)
	if err != nil {
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", fmt.Sprintf("NID_AUT=%s; NID_SES=%s", nidAut, nidSes))

	resp, err := client.Do(req)
	if err != nil {
		// 네트워크 일시 오류일 수 있으므로 무시
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		LogInfo("[Auth] 시작 시 세션 만료 감지 (HTTP 401) -> 자동 갱신 수행")
		_, _, _ = RefreshNaverSession(nidAut)
		return
	}

	if resp.StatusCode == http.StatusOK {
		var statusResp struct {
			Content struct {
				LoggedIn bool `json:"loggedIn"`
			} `json:"content"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err == nil && !statusResp.Content.LoggedIn {
			LogInfo("[Auth] 시작 시 세션 미인증 상태 감지 (loggedIn=false) -> 자동 갱신 수행")
			_, _, _ = RefreshNaverSession(nidAut)
		}
	}
}

