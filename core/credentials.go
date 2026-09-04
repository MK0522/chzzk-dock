package core

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"syscall"
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

// Config: 네이버 세션 쿠키 자격 증명 모델
type Config struct {
	NidAut string `json:"nid_aut"`
	NidSes string `json:"nid_ses"`
}

var (
	credLock       sync.Mutex
	cachedConfig   *Config
	cachedConfigMu sync.RWMutex
)

// CredWrite: Windows Credential Manager 시스템 보안 금고에 데이터 JSON 직렬화 저장
func CredWrite(targetName string, data map[string]interface{}) bool {
	credLock.Lock()
	defer credLock.Unlock()

	blobBytes, err := json.Marshal(data)
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

// CredRead: Windows Credential Manager 시스템 보안 금고에서 자격 증명 로드 및 역직렬화
func CredRead(targetName string) map[string]interface{} {
	credLock.Lock()
	defer credLock.Unlock()

	targetNamePtr, err := syscall.UTF16PtrFromString(targetName)
	if err != nil {
		return make(map[string]interface{})
	}

	var pCred *CREDENTIALW
	r1, _, _ := procCredReadW.Call(
		uintptr(unsafe.Pointer(targetNamePtr)),
		uintptr(CRED_TYPE_GENERIC),
		0,
		uintptr(unsafe.Pointer(&pCred)),
	)

	if r1 == 0 || pCred == nil {
		return make(map[string]interface{})
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pCred)))

	if pCred.CredentialBlobSize == 0 || pCred.CredentialBlob == nil {
		return make(map[string]interface{})
	}

	rawBytes := unsafe.Slice(pCred.CredentialBlob, pCred.CredentialBlobSize)
	result := make(map[string]interface{})
	if err := json.Unmarshal(rawBytes, &result); err != nil {
		return make(map[string]interface{})
	}

	return result
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

// GetTargetName: Windows Credential Manager 타깃 이름 반환 (테스트 환경 변수 CHZZK_CRED_TARGET 지원)
func GetTargetName() string {
	if val := os.Getenv("CHZZK_CRED_TARGET"); val != "" {
		return val
	}
	return CRED_TARGET_NAME
}

// LoadConfig: Windows Credential Manager에서 네이버 세션 쿠키(NID_AUT, NID_SES) 로드 (메모리 캐시 지원)
func LoadConfig() Config {
	cachedConfigMu.RLock()
	if cachedConfig != nil && cachedConfig.NidAut != "" && cachedConfig.NidSes != "" {
		cfg := *cachedConfig
		cachedConfigMu.RUnlock()
		return cfg
	}
	cachedConfigMu.RUnlock()

	cfg := Config{NidAut: "", NidSes: ""}
	stored := CredRead(GetTargetName())
	if stored != nil {
		if aut, ok := stored["nid_aut"].(string); ok {
			cfg.NidAut = strings.TrimSpace(aut)
		}
		if ses, ok := stored["nid_ses"].(string); ok {
			cfg.NidSes = strings.TrimSpace(ses)
		}
	}

	cachedConfigMu.Lock()
	cachedConfig = &cfg
	cachedConfigMu.Unlock()

	return cfg
}

// SaveConfig: 네이버 세션 쿠키를 Windows Credential Manager에 안전하게 저장 (디스크 파일 저장 없음)
func SaveConfig(newData map[string]interface{}) Config {
	cfg := LoadConfig()

	if newData != nil {
		if aut, ok := newData["nid_aut"].(string); ok {
			cfg.NidAut = strings.TrimSpace(aut)
		}
		if ses, ok := newData["nid_ses"].(string); ok {
			cfg.NidSes = strings.TrimSpace(ses)
		}
	}

	cleanCfg := Config{
		NidAut: strings.TrimSpace(cfg.NidAut),
		NidSes: strings.TrimSpace(cfg.NidSes),
	}

	target := GetTargetName()
	if cleanCfg.NidAut != "" || cleanCfg.NidSes != "" {
		dataMap := map[string]interface{}{
			"nid_aut": cleanCfg.NidAut,
			"nid_ses": cleanCfg.NidSes,
		}
		CredWrite(target, dataMap)
	} else {
		CredDelete(target)
	}

	cachedConfigMu.Lock()
	cachedConfig = &cleanCfg
	cachedConfigMu.Unlock()

	return cleanCfg
}

// ClearConfig: Windows Credential Manager에서 네이버 세션 쿠키 자격 증명 완전 삭제 (로그아웃용)
func ClearConfig() bool {
	cachedConfigMu.Lock()
	cachedConfig = &Config{NidAut: "", NidSes: ""}
	cachedConfigMu.Unlock()

	return CredDelete(GetTargetName())
}
