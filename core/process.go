package core

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ==============================================================================
// [CHZZK OBS DOCK - Process, Network & OBS Integration Engine]
// - 통합 모듈: Win32 프로세스 스냅샷(Toolhelp32), TCP 포트 점유 감지(iphlpapi), OBS 환경 감지 및 스크립트 설치
// - Zero-PowerShell / Zero-WMI: 순수 Win32 API로 백신 오탐 원천 차단 및 0ms 즉시 응답
// ==============================================================================

// -----------------------------------------------------------------------------
// 1. 공통 Win32 DLL 및 프로시저 정의
// -----------------------------------------------------------------------------

const (
	TH32CS_SNAPPROCESS                = 0x00000002
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

var (
	iphlpapiDLL       = syscall.NewLazyDLL("iphlpapi.dll")
	procGetOsTcpTable = iphlpapiDLL.NewProc("GetExtendedTcpTable")

	procCreateT32Snap   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW = kernel32.NewProc("Process32FirstW")
	procProcess32NextW  = kernel32.NewProc("Process32NextW")
	procOpenProcess     = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")

	procShellExecuteW        = shell32.NewProc("ShellExecuteW")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")

	procCoInitializeEx      = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize      = ole32DLL.NewProc("CoUninitialize")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
)

type PROCESSENTRY32W struct {
	DwSize              uint32
	CntUsage            uint32
	Th32ProcessID       uint32
	Th32DefaultHeapID   uintptr
	Th32ModuleID        uint32
	CntThreads          uint32
	Th32ParentProcessID uint32
	PcPriClassBase      int32
	DwFlags             uint32
	SzExeFile           [260]uint16
}

type OsTcpRow struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

type BROWSEINFOW struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

// -----------------------------------------------------------------------------
// 2. 프로세스 조회 및 스냅샷 (Process Inspection)
// -----------------------------------------------------------------------------

// IsProcessRunning: 특정 실행 파일명(예: "obs64.exe")이 시스템에서 실행 중인지 확인
func IsProcessRunning(targetExe string) bool {
	hSnap, _, _ := procCreateT32Snap.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == 0 || hSnap == uintptr(syscall.InvalidHandle) {
		return false
	}
	defer kernel32.NewProc("CloseHandle").Call(hSnap)

	var entry PROCESSENTRY32W
	entry.DwSize = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return false
	}

	for {
		exeName := syscall.UTF16ToString(entry.SzExeFile[:])
		if strings.EqualFold(exeName, targetExe) {
			return true
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}
	return false
}

// FindRunningObsPath: 실행 중인 obs64.exe / obs32.exe의 전체 실행 파일 경로 검색
func FindRunningObsPath() string {
	hSnap, _, _ := procCreateT32Snap.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == 0 || hSnap == uintptr(syscall.InvalidHandle) {
		return ""
	}
	defer kernel32.NewProc("CloseHandle").Call(hSnap)

	var entry PROCESSENTRY32W
	entry.DwSize = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return ""
	}

	for {
		exeName := syscall.UTF16ToString(entry.SzExeFile[:])
		if strings.EqualFold(exeName, "obs64.exe") || strings.EqualFold(exeName, "obs32.exe") {
			pid := entry.Th32ProcessID
			hProcess, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
			if hProcess != 0 {
				var buf [1024]uint16
				size := uint32(len(buf))
				qRet, _, _ := procQueryFullProcessImageNameW.Call(hProcess, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
				kernel32.NewProc("CloseHandle").Call(hProcess)
				if qRet != 0 {
					return syscall.UTF16ToString(buf[:size])
				}
			}
		}

		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}
	return ""
}

// -----------------------------------------------------------------------------
// 3. 네트워크 포트 탐색기 (Port Detector & Fallback Allocator)
// -----------------------------------------------------------------------------

// FindProcessUsingPort: 지정된 TCP 포트를 점유(LISTEN) 중인 프로세스의 PID와 이름을 반환합니다.
func FindProcessUsingPort(port int) (uint32, string) {
	var size uint32
	procGetOsTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 1, 2, 5, 0)
	if size == 0 {
		return 0, ""
	}

	buf := make([]byte, size)
	r1, _, _ := procGetOsTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1, 2, 5, 0,
	)
	if r1 != 0 {
		return 0, ""
	}

	numEntries := binary.LittleEndian.Uint32(buf[0:4])
	rowSize := int(unsafe.Sizeof(OsTcpRow{}))

	for i := 0; i < int(numEntries); i++ {
		offset := 4 + (i * rowSize)
		if offset+rowSize > len(buf) {
			break
		}

		row := (*OsTcpRow)(unsafe.Pointer(&buf[offset]))

		// 포트 번호 변환 (네트워크 바이트 순서)
		p := ((row.LocalPort & 0xFF) << 8) | ((row.LocalPort >> 8) & 0xFF)

		if int(p) == port {
			var pname string = ""
			if row.OwningPid != 0 {
				netSnap, _, _ := procCreateT32Snap.Call(TH32CS_SNAPPROCESS, 0)
				if netSnap != 0 && netSnap != uintptr(syscall.InvalidHandle) {
					var entry PROCESSENTRY32W
					entry.DwSize = uint32(unsafe.Sizeof(entry))

					rFirst, _, _ := procProcess32FirstW.Call(netSnap, uintptr(unsafe.Pointer(&entry)))
					if rFirst != 0 {
						for {
							if entry.Th32ProcessID == row.OwningPid {
								pname = syscall.UTF16ToString(entry.SzExeFile[:])
								break
							}
							rNext, _, _ := procProcess32NextW.Call(netSnap, uintptr(unsafe.Pointer(&entry)))
							if rNext == 0 {
								break
							}
						}
					}
					kernel32.NewProc("CloseHandle").Call(netSnap)
				}
			}
			return row.OwningPid, pname
		}
	}
	return 0, ""
}

// CheckPortAvailable: 지정된 포트가 사용 가능한지 검사합니다.
func CheckPortAvailable(port int) (bool, uint32, string, error) {
	if port < 1024 || port > 65535 {
		return false, 0, "", fmt.Errorf("포트 번호는 1024 ~ 65535 사이여야 합니다 (입력값: %d)", port)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		pid, procName := FindProcessUsingPort(port)
		return false, pid, procName, err
	}
	_ = l.Close()
	return true, 0, "", nil
}

// FindSafeFallbackPort: IANA 사설/동적 포트 대역(49152 ~ 65535)에서 안전한 랜덤 포트 탐색 및 바인딩
func FindSafeFallbackPort() (int, net.Listener, error) {
	const minPort = 49152
	const maxPort = 65535
	const maxAttempts = 30

	for attempt := 0; attempt < maxAttempts; attempt++ {
		n, err := rand.Int(rand.Reader, big.NewInt(maxPort-minPort+1))
		if err != nil {
			continue
		}
		candidate := int(n.Int64()) + minPort
		addr := fmt.Sprintf("127.0.0.1:%d", candidate)

		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return candidate, listener, nil
		}
	}

	fallbackListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, fmt.Errorf("대체 포트 할당 실패: %w", err)
	}
	assignedPort := fallbackListener.Addr().(*net.TCPAddr).Port
	return assignedPort, fallbackListener, nil
}

// -----------------------------------------------------------------------------
// 4. OBS Studio 디렉터리 탐색 및 스크립트 관리 (OBS Integration)
// -----------------------------------------------------------------------------

// DetectObsScriptsDir: OBS Studio의 scripts 디렉터리를 탐색하고 실제 디스크 존재 여부를 반환합니다.
func DetectObsScriptsDir() (string, bool) {
	// 1순위: 현재 실행 중인 OBS 프로세스로부터 추론
	if runningPath := FindRunningObsPath(); runningPath != "" {
		candidate := ResolveObsScriptsDir(runningPath)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, true
		}
	}

	// 2순위: 표준 C 드라이브 설치 경로 확인
	candidates := []string{
		`C:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		`C:\Program Files (x86)\obs-studio\data\obs-plugins\frontend-tools\scripts`,
	}

	for _, path := range candidates {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			return path, true
		}
	}

	// 3순위: 디스크에 아직 존재하지 않는 경우 표준 64비트 기본 경로와 함께 false 반환
	defaultPath := `C:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`
	return defaultPath, false
}

// FindObsScriptsDir: 호환성을 위한 기존 탐색 함수 (경로 문자열 반환)
func FindObsScriptsDir() (string, error) {
	path, _ := DetectObsScriptsDir()
	return path, nil
}

// ResolveObsScriptsDir: 사용자가 입력하거나 선택한 폴더 경로로부터 scripts 디렉터리 경로를 도출합니다.
func ResolveObsScriptsDir(inputPath string) string {
	cleanPath := strings.TrimSpace(inputPath)
	cleanPath = strings.Trim(cleanPath, `"'`)
	if cleanPath == "" {
		def, _ := DetectObsScriptsDir()
		return def
	}

	cleanPath = filepath.Clean(cleanPath)

	// 1. 이미 scripts 폴더를 가리키고 있는 경우
	base := strings.ToLower(filepath.Base(cleanPath))
	if base == "scripts" {
		return cleanPath
	}

	// 2. 입력된 경로 내에 data\obs-plugins\frontend-tools\scripts가 존재하는 경우
	subCandidate := filepath.Join(cleanPath, "data", "obs-plugins", "frontend-tools", "scripts")
	if stat, err := os.Stat(subCandidate); err == nil && stat.IsDir() {
		return subCandidate
	}

	// 3. 입력된 경로 내에 scripts 하위 폴더가 존재하는 경우 (포터블 또는 커스텀 구조)
	simpleSub := filepath.Join(cleanPath, "scripts")
	if stat, err := os.Stat(simpleSub); err == nil && stat.IsDir() {
		return simpleSub
	}

	// 4. bin\64bit 또는 bin 경로가 포함된 경우 루트 디렉터리로 거슬러 올라가 탐색
	norm := filepath.ToSlash(cleanPath)
	if idx := strings.Index(norm, "/bin/64bit"); idx != -1 {
		rootDir := cleanPath[:idx]
		return filepath.Join(rootDir, "data", "obs-plugins", "frontend-tools", "scripts")
	}
	if idx := strings.Index(norm, "/bin/32bit"); idx != -1 {
		rootDir := cleanPath[:idx]
		return filepath.Join(rootDir, "data", "obs-plugins", "frontend-tools", "scripts")
	}
	if strings.HasSuffix(norm, "/bin") {
		rootDir := filepath.Dir(cleanPath)
		return filepath.Join(rootDir, "data", "obs-plugins", "frontend-tools", "scripts")
	}

	// 5. 일반 obs-studio 루트 폴더로 간주하여 표준 하위 경로 결합
	return filepath.Join(cleanPath, "data", "obs-plugins", "frontend-tools", "scripts")
}

// BrowseForObsFolder: Win32 SHBrowseForFolderW를 사용하여 사용자에게 폴더 선택 다이얼로그를 표시합니다.
func BrowseForObsFolder(title string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procCoInitializeEx.Call(0, 0x6)
	defer procCoUninitialize.Call()

	if title == "" {
		title = "OBS Studio 설치 폴더(obs-studio) 또는 scripts 폴더를 선택하세요."
	}
	titleUTF16, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}

	fgHwnd, _, _ := procGetForegroundWindow.Call()

	callback := syscall.NewCallback(func(hwnd syscall.Handle, uMsg uint32, lParam, lpData uintptr) uintptr {
		if uMsg == 1 { // BFFM_INITIALIZED
			hwndTopmost := ^uintptr(0) // HWND_TOPMOST = -1
			procSetWindowPos.Call(
				uintptr(hwnd),
				hwndTopmost,
				0, 0, 0, 0,
				0x0001|0x0002|0x0040,
			)
			procSetForegroundWindow.Call(uintptr(hwnd))
		}
		return 0
	})

	var displayName [260]uint16
	bi := BROWSEINFOW{
		HwndOwner:      fgHwnd,
		LpszTitle:      titleUTF16,
		PszDisplayName: &displayName[0],
		UlFlags:        0x00000001 | 0x00000040 | 0x00000010,
		Lpfn:           callback,
	}

	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil // 취소
	}
	defer procCoTaskMemFree.Call(pidl)

	var pathBuf [1024]uint16
	ret, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	if ret == 0 {
		return "", fmt.Errorf("선택된 폴더의 경로 변환에 실패했습니다")
	}

	return syscall.UTF16ToString(pathBuf[:]), nil
}

// PrepareLauncherScriptWithExePath: 스크립트 템플릿에 현재 실행 중인 chzzk-dock.exe의 실제 절대 경로를 주입합니다.
func PrepareLauncherScriptWithExePath(scriptData []byte) []byte {
	exePath, err := os.Executable()
	if err != nil || exePath == "" {
		return scriptData
	}
	scriptStr := string(scriptData)
	escapedPath := strings.ReplaceAll(exePath, `\`, `\\`)
	scriptStr = strings.Replace(scriptStr, `local BAKED_EXE_PATH = ""`, fmt.Sprintf(`local BAKED_EXE_PATH = "%s"`, escapedPath), 1)
	return []byte(scriptStr)
}

// IsScriptInstalled: 대상 scripts 디렉터리에 chzzk_dock_launcher.lua가 정상 배치되어 있는지 확인합니다.
func IsScriptInstalled(scriptsDir string) bool {
	if scriptsDir == "" {
		scriptsDir, _ = DetectObsScriptsDir()
	}
	targetFile := filepath.Join(scriptsDir, "chzzk_dock_launcher.lua")
	if stat, err := os.Stat(targetFile); err == nil && stat.Size() > 0 {
		return true
	}
	return false
}

// CheckScriptStatus: 대상 scripts 디렉터리에 스크립트 설치 여부 및 최신 내용 일치 여부(업데이트 필요 여부)를 검사합니다.
func CheckScriptStatus(scriptsDir string, latestScript []byte) (installed bool, needsUpdate bool) {
	if scriptsDir == "" {
		scriptsDir, _ = DetectObsScriptsDir()
	}
	if scriptsDir == "" {
		return false, false
	}
	targetFile := filepath.Join(scriptsDir, "chzzk_dock_launcher.lua")
	existingData, err := os.ReadFile(targetFile)
	if err != nil || len(existingData) == 0 {
		return false, false
	}
	installed = true

	expectedData := PrepareLauncherScriptWithExePath(latestScript)
	normExisting := strings.ReplaceAll(string(existingData), "\r\n", "\n")
	normExpected := strings.ReplaceAll(string(expectedData), "\r\n", "\n")

	if strings.TrimSpace(normExisting) != strings.TrimSpace(normExpected) {
		needsUpdate = true
	}
	return installed, needsUpdate
}

// InstallLauncherScriptToObs: 지정된(또는 자동 감지된) OBS scripts 폴더에 스크립트를 추가합니다.
func InstallLauncherScriptToObs(customDir string, scriptData []byte) (string, error) {
	var targetDir string
	if customDir != "" {
		targetDir = ResolveObsScriptsDir(customDir)
	} else {
		var err error
		targetDir, err = FindObsScriptsDir()
		if err != nil {
			return "", err
		}
	}

	scriptData = PrepareLauncherScriptWithExePath(scriptData)
	targetPath := filepath.Join(targetDir, "chzzk_dock_launcher.lua")

	_ = os.MkdirAll(targetDir, 0755)
	if err := os.WriteFile(targetPath, scriptData, 0644); err == nil {
		return targetPath, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
	}

	opPtr, _ := syscall.UTF16PtrFromString("runas")
	filePtr, _ := syscall.UTF16PtrFromString(exePath)
	paramPtr, _ := syscall.UTF16PtrFromString(fmt.Sprintf(`--install-script "%s"`, targetDir))

	ret, _, err := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(opPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramPtr)),
		0,
		1,
	)

	if ret <= 32 {
		return "", fmt.Errorf("관리자 권한(UAC) 승인이 거부되었거나 실패했습니다 (코드: %d): %w", ret, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if stat, err := os.Stat(targetPath); err == nil && stat.Size() > 0 {
			return targetPath, nil
		}
	}

	return "", fmt.Errorf("관리자 권한 프로세스 실행 후 파일 생성을 확인하지 못했습니다")
}
