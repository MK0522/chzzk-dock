package core

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
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

	procCloseHandle = kernel32.NewProc("CloseHandle")
	procCreateT32Snap   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW = kernel32.NewProc("Process32FirstW")
	procProcess32NextW  = kernel32.NewProc("Process32NextW")
	procOpenProcess     = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")

	procShellExecuteW               = shell32.NewProc("ShellExecuteW")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	procSHBrowseForFolderW          = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW        = shell32.NewProc("SHGetPathFromIDListW")

	comdlg32DLL              = syscall.NewLazyDLL("comdlg32.dll")
	procGetOpenFileNameW     = comdlg32DLL.NewProc("GetOpenFileNameW")
	procCommDlgExtendedError = comdlg32DLL.NewProc("CommDlgExtendedError")

	procCoInitializeEx      = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize      = ole32DLL.NewProc("CoUninitialize")
	procCoCreateInstance    = ole32DLL.NewProc("CoCreateInstance")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")

	procCreateJobObjectW         = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	procGetCurrentProcess        = kernel32.NewProc("GetCurrentProcess")
	procWaitForSingleObject      = kernel32.NewProc("WaitForSingleObject")
	procGetProcessTimes          = kernel32.NewProc("GetProcessTimes")

	versionDLL                  = syscall.NewLazyDLL("version.dll")
	procGetFileVersionInfoSizeW = versionDLL.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = versionDLL.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = versionDLL.NewProc("VerQueryValueW")
)

type vsFixedFileInfo struct {
	DwSignature        uint32
	DwStrucVersion     uint32
	DwFileVersionMS    uint32
	DwFileVersionLS    uint32
	DwProductVersionMS uint32
	DwProductVersionLS uint32
	DwFileFlagsMask    uint32
	DwFileFlags        uint32
	DwFileOS           uint32
	DwFileType         uint32
	DwFileSubtype      uint32
	DwFileDateMS       uint32
	DwFileDateLS       uint32
}

const (
	JobObjectExtendedLimitInformation  = 9
	JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000
	JOB_OBJECT_LIMIT_BREAKAWAY_OK      = 0x00000800
	CREATE_BREAKAWAY_FROM_JOB          = 0x01000000
)

type JOBOBJECT_BASIC_LIMIT_INFORMATION struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type IO_COUNTERS struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct {
	BasicLimitInformation JOBOBJECT_BASIC_LIMIT_INFORMATION
	IoInfo                IO_COUNTERS
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var globalJobHandle uintptr

// InitProcessJobObject: [Ponytail/YAGNI 예외] 메인 서버가 강제 종료되거나 충돌하더라도 윈도우 커널이 자식 프로세스(--chat, --remote, --login)를 100% 자동 회수하도록 보장하는 표준 Win32 Job Object 바인딩
func InitProcessJobObject() {
	if runtime.GOOS != "windows" {
		return
	}
	hJob, _, err := procCreateJobObjectW.Call(0, 0)
	if hJob == 0 {
		LogError("[JobObject] 작업 개체 생성 실패 (CreateJobObjectW): %v", err)
		return
	}
	var info JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | JOB_OBJECT_LIMIT_BREAKAWAY_OK
	ret, _, err := procSetInformationJobObject.Call(
		hJob,
		uintptr(JobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if ret == 0 {
		LogError("[JobObject] 작업 개체 제한 설정 실패 (SetInformationJobObject): %v", err)
		procCloseHandle.Call(hJob)
		return
	}
	curProc, _, _ := procGetCurrentProcess.Call()
	ret, _, err = procAssignProcessToJobObject.Call(hJob, curProc)
	if ret != 0 {
		globalJobHandle = hJob
		LogInfo("[JobObject] 프로세스 작업 개체 등록 완료 (부모 프로세스 비정상 종료 시 자식 일괄 자동 회수 보장)")
	} else {
		LogError("[JobObject] 작업 개체 프로세스 할당 실패 (AssignProcessToJobObject): %v", err)
		procCloseHandle.Call(hJob)
	}
}

func StartParentWatchdog() {
	if runtime.GOOS != "windows" {
		return
	}
	ppid := os.Getppid()
	if ppid <= 0 {
		return
	}
	const SYNCHRONIZE = 0x00100000
	hParent, _, openErr := procOpenProcess.Call(
		uintptr(SYNCHRONIZE|PROCESS_QUERY_LIMITED_INFORMATION),
		0,
		uintptr(ppid),
	)
	if hParent == 0 {
		mode := ""
		if len(os.Args) > 1 {
			mode = os.Args[1]
		}
		message := fmt.Sprintf("OpenProcess failed: mode=%s parentPID=%d windowsError=%d (%v)", mode, ppid, openErr, openErr)
		fmt.Fprintf(os.Stderr, "[Watchdog] %s\n", message)
		LogError("[ParentWatchdog] %s", message)
		dir := filepath.Dir(getSettingsFilePath())
		if err := os.MkdirAll(dir, 0700); err == nil {
			if f, err := os.OpenFile(filepath.Join(dir, "watchdog.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); err == nil {
				fmt.Fprintf(f, "%s [ParentWatchdog] %s\n", time.Now().Format(time.RFC3339), message)
				f.Close()
			}
		}
		os.Exit(0)
	}

	var pCreate, pExit, pKernel, pUser uint64
	var sCreate, sExit, sKernel, sUser uint64
	curProc, _, _ := procGetCurrentProcess.Call()
	r1, _, _ := procGetProcessTimes.Call(hParent, uintptr(unsafe.Pointer(&pCreate)), uintptr(unsafe.Pointer(&pExit)), uintptr(unsafe.Pointer(&pKernel)), uintptr(unsafe.Pointer(&pUser)))
	r2, _, _ := procGetProcessTimes.Call(curProc, uintptr(unsafe.Pointer(&sCreate)), uintptr(unsafe.Pointer(&sExit)), uintptr(unsafe.Pointer(&sKernel)), uintptr(unsafe.Pointer(&sUser)))
	if r1 != 0 && r2 != 0 && pCreate > sCreate {
		procCloseHandle.Call(hParent)
		os.Exit(0)
	}

	go func() {
		procWaitForSingleObject.Call(hParent, 0xFFFFFFFF)
		procCloseHandle.Call(hParent)
		os.Exit(0)
	}()
}

// DetachFromJob: 독립적으로 실행되어야 하는 프로세스(인스톨러, 재시작 등)가 현재 Job Object에 묶여 부모 종료 시 함께 종료되지 않도록 탈출 플래그를 설정합니다.
func DetachFromJob(cmd *exec.Cmd) {
	if cmd == nil || runtime.GOOS != "windows" {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= CREATE_BREAKAWAY_FROM_JOB
}

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

// -----------------------------------------------------------------------------
// 2. 프로세스 조회 및 스냅샷 (Process Inspection)
// -----------------------------------------------------------------------------

// forEachProcess: Toolhelp32 프로세스 스냅샷을 안전하게 열고 순회하는 단일 캡슐화 헬퍼 (핸들 누수 방지 및 성능 최적화)
func forEachProcess(fn func(pid uint32, exeName string) bool) {
	hSnap, _, _ := procCreateT32Snap.Call(TH32CS_SNAPPROCESS, 0)
	if hSnap == 0 || hSnap == uintptr(syscall.InvalidHandle) {
		return
	}
	defer procCloseHandle.Call(hSnap)

	var entry PROCESSENTRY32W
	entry.DwSize = uint32(unsafe.Sizeof(entry))

	ret, _, _ := procProcess32FirstW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
	if ret == 0 {
		return
	}

	for {
		exeName := syscall.UTF16ToString(entry.SzExeFile[:])
		if !fn(entry.Th32ProcessID, exeName) {
			break
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}
}

// IsProcessRunning: 특정 실행 파일명(예: "obs64.exe")이 시스템에서 실행 중인지 확인
func IsProcessRunning(targetExe string) bool {
	var found bool
	forEachProcess(func(pid uint32, exeName string) bool {
		if strings.EqualFold(exeName, targetExe) {
			found = true
			return false
		}
		return true
	})
	return found
}

// FindRunningObsPath: 실행 중인 obs64.exe / obs32.exe의 전체 실행 파일 경로 검색
func FindRunningObsPath() string {
	var foundPath string
	forEachProcess(func(pid uint32, exeName string) bool {
		if strings.EqualFold(exeName, "obs64.exe") || strings.EqualFold(exeName, "obs32.exe") {
			hProcess, _, _ := procOpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
			if hProcess != 0 {
				var buf [1024]uint16
				size := uint32(len(buf))
				qRet, _, _ := procQueryFullProcessImageNameW.Call(hProcess, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
				procCloseHandle.Call(hProcess)
				if qRet != 0 {
					foundPath = syscall.UTF16ToString(buf[:size])
					return false
				}
			}
		}
		return true
	})
	return foundPath
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
				forEachProcess(func(pid uint32, exeName string) bool {
					if pid == row.OwningPid {
						pname = exeName
						return false
					}
					return true
				})
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

func getExecutableMajorVersion(exePath string) int {
	if strings.TrimSpace(exePath) == "" {
		return 0
	}
	p, err := syscall.UTF16PtrFromString(exePath)
	if err != nil {
		return 0
	}
	var handle uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&handle)))
	if size == 0 {
		return 0
	}
	buf := make([]byte, size)
	ret, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0])))
	if ret == 0 {
		return 0
	}
	subBlock, _ := syscall.UTF16PtrFromString(`\`)
	var infoPtr uintptr
	var infoLen uint32
	ret, _, _ = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&infoPtr)),
		uintptr(unsafe.Pointer(&infoLen)),
	)
	if ret == 0 || infoPtr == 0 || infoLen < uint32(unsafe.Sizeof(vsFixedFileInfo{})) {
		return 0
	}
	ffi := (*vsFixedFileInfo)(unsafe.Pointer(infoPtr))
	if ffi.DwSignature != 0xFEEF04BD {
		return 0
	}
	return int(ffi.DwFileVersionMS >> 16)
}

func detectObsMajorVersionFromRoot(rootDir string) int {
	for _, rel := range [][]string{
		{"bin", "64bit", "obs64.exe"},
		{"bin", "32bit", "obs32.exe"},
		{"obs64.exe"},
		{"obs32.exe"},
	} {
		exePath := filepath.Join(append([]string{rootDir}, rel...)...)
		if major := getExecutableMajorVersion(exePath); major > 0 {
			return major
		}
	}
	return 0
}

// DetectObsScriptsDir: OBS Studio의 scripts 디렉터리를 탐색하고 실제 디스크 존재 여부를 반환합니다.
func DetectObsScriptsDir() (string, bool) {
	// 1순위: 현재 실행 중인 OBS 프로세스로부터 추론
	if runningPath := FindRunningObsPath(); runningPath != "" {
		candidate := ResolveObsScriptsDir(runningPath)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, true
		}
	}

	// 2순위: 표준 C 드라이브 설치 경로 확인 (실행 파일 버전으로 33+ vs <33 우선 판별)
	stdRoots := []string{
		`C:\Program Files\obs-studio`,
		`C:\Program Files (x86)\obs-studio`,
	}
	for _, root := range stdRoots {
		if detectObsMajorVersionFromRoot(root) > 0 {
			candidate := resolveObsRootScriptsDir(root)
			if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
				return candidate, true
			}
		}
	}

	candidates := []string{
		`C:\Program Files\obs-studio\core\frontend-tools\data\scripts`,
		`C:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		`C:\Program Files (x86)\obs-studio\core\frontend-tools\data\scripts`,
		`C:\Program Files (x86)\obs-studio\data\obs-plugins\frontend-tools\scripts`,
	}

	for _, path := range candidates {
		if stat, err := os.Stat(path); err == nil && stat.IsDir() {
			return path, true
		}
	}

	// 3순위: 디스크에 아직 존재하지 않는 경우 표준 64비트 기본 경로와 함께 false 반환
	return resolveObsRootScriptsDir(`C:\Program Files\obs-studio`), false
}

func resolveObsRootScriptsDir(rootDir string) string {
	if major := detectObsMajorVersionFromRoot(rootDir); major > 0 {
		if major >= 33 {
			return filepath.Join(rootDir, "core", "frontend-tools", "data", "scripts")
		}
		return filepath.Join(rootDir, "data", "obs-plugins", "frontend-tools", "scripts")
	}
	for _, rel := range [][]string{
		{"core", "frontend-tools", "data", "scripts"},
		{"data", "obs-plugins", "frontend-tools", "scripts"},
		{"scripts"},
	} {
		candidate := filepath.Join(append([]string{rootDir}, rel...)...)
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
	}
	return filepath.Join(rootDir, "core", "frontend-tools", "data", "scripts")
}

// DetectObs33PathMigration: OBS 33 이상 경로(core\frontend-tools\data\scripts)로 설치할 때
// 이전 32 이하 경로(data\obs-plugins\frontend-tools\scripts)에서 전환되었는지 감지합니다.
func DetectObs33PathMigration(targetDir string, previousDirs ...string) bool {
	cleanSlash := filepath.ToSlash(filepath.Clean(targetDir))
	lowerSlash := strings.ToLower(cleanSlash)
	const newSuffix = "/core/frontend-tools/data/scripts"
	idx := strings.Index(lowerSlash, newSuffix)
	if idx == -1 || !strings.HasSuffix(lowerSlash, newSuffix) {
		return false
	}

	for _, prev := range previousDirs {
		if strings.TrimSpace(prev) == "" {
			continue
		}
		prevLower := strings.ToLower(filepath.ToSlash(filepath.Clean(prev)))
		if strings.Contains(prevLower, "/data/obs-plugins/frontend-tools/scripts") {
			return true
		}
	}

	root := filepath.FromSlash(cleanSlash[:idx])
	legacyScript := filepath.Join(root, "data", "obs-plugins", "frontend-tools", "scripts", "chzzk_dock_launcher.lua")
	if _, err := os.Stat(legacyScript); err == nil {
		return true
	}
	return false
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
	lowerClean := strings.ToLower(cleanPath)
	if strings.HasSuffix(lowerClean, ".lua") || strings.HasSuffix(lowerClean, ".exe") {
		cleanPath = filepath.Dir(cleanPath)
	}

	// 1. 이미 scripts 폴더를 가리키고 있는 경우
	base := strings.ToLower(filepath.Base(cleanPath))
	if base == "scripts" {
		return cleanPath
	}

	// 2. bin\64bit, bin, core, data 등 하위 경로가 선택된 경우 루트 디렉터리로 거슬러 올라가 탐색
	norm := filepath.ToSlash(cleanPath)
	lowerNorm := strings.ToLower(norm)
	if idx := strings.Index(lowerNorm, "/bin/64bit"); idx != -1 {
		return resolveObsRootScriptsDir(cleanPath[:idx])
	}
	if idx := strings.Index(lowerNorm, "/bin/32bit"); idx != -1 {
		return resolveObsRootScriptsDir(cleanPath[:idx])
	}
	if strings.HasSuffix(lowerNorm, "/bin") {
		return resolveObsRootScriptsDir(filepath.Dir(cleanPath))
	}
	if idx := strings.Index(lowerNorm, "/core/frontend-tools"); idx != -1 {
		return resolveObsRootScriptsDir(cleanPath[:idx])
	}
	if idx := strings.Index(lowerNorm, "/data/obs-plugins"); idx != -1 {
		return resolveObsRootScriptsDir(cleanPath[:idx])
	}
	if strings.HasSuffix(lowerNorm, "/core") || strings.HasSuffix(lowerNorm, "/data") {
		parent := filepath.Dir(cleanPath)
		if st, err := os.Stat(filepath.Join(parent, "bin")); err == nil && st.IsDir() {
			return resolveObsRootScriptsDir(parent)
		}
	}

	// 3. 폴더 이름과 무관하게 OBS 루트 폴더로 간주하여 내부 bin/core/data 구조로 scripts 경로 탐색
	return resolveObsRootScriptsDir(cleanPath)
}

type winGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type browseInfoW struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

// BrowseForObsFolder: Win32 IFileOpenDialog(폴더 선택 모드)를 사용하여 사용자에게 OBS 설치 폴더 선택 다이얼로그를 표시합니다.
func BrowseForObsFolder(title string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if title == "" {
		title = "OBS 설치 폴더(obs-studio)를 선택하세요."
	}
	titleUTF16, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return "", err
	}

	fgHwnd, _, _ := procGetForegroundWindow.Call()

	const (
		coinitApartmentThreaded = 0x2
		clsctxInprocServer      = 0x1
		fosPickFolders          = 0x00000020
		fosForceFileSystem      = 0x00000040
		fosPathMustExist        = 0x00000800
		sigdnFileSysPath        = 0x80058000
		hrCancelled             = 0x800704C7
	)

	hrInit, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if hrInit == 0 || hrInit == 1 {
		defer procCoUninitialize.Call()
	}

	clsidFileOpenDialog := winGUID{0xDC1C5A9C, 0xE88A, 0x4dde, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog := winGUID{0xd57c7288, 0xd4ad, 0x4768, [8]byte{0xbe, 0x02, 0x9d, 0x96, 0x95, 0x32, 0xd9, 0x60}}
	iidIShellItem := winGUID{0x43826d1e, 0xe718, 0x42ee, [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}

	var dlg uintptr
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dlg)),
	)
	if hr == 0 && dlg != 0 {
		vtable := *(*[28]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(dlg))))
		// IUnknown::Release (index 2)
		defer syscall.SyscallN(vtable[2], dlg)

		var opts uint32
		// IFileDialog::GetOptions (index 10) & SetOptions (index 9)
		_, _, _ = syscall.SyscallN(vtable[10], dlg, uintptr(unsafe.Pointer(&opts)))
		_, _, _ = syscall.SyscallN(vtable[9], dlg, uintptr(opts|fosPickFolders|fosForceFileSystem|fosPathMustExist))
		// IFileDialog::SetTitle (index 17)
		_, _, _ = syscall.SyscallN(vtable[17], dlg, uintptr(unsafe.Pointer(titleUTF16)))

		// 초기 폴더 설정 (C:\Program Files\obs-studio 우선, 없으면 C:\Program Files)
		for _, initDir := range []string{`C:\Program Files\obs-studio`, `C:\Program Files (x86)\obs-studio`, `C:\Program Files`} {
			if st, statErr := os.Stat(initDir); statErr == nil && st.IsDir() {
				if pInit, pErr := syscall.UTF16PtrFromString(initDir); pErr == nil {
					var folderItem uintptr
					hrItem, _, _ := procSHCreateItemFromParsingName.Call(
						uintptr(unsafe.Pointer(pInit)),
						0,
						uintptr(unsafe.Pointer(&iidIShellItem)),
						uintptr(unsafe.Pointer(&folderItem)),
					)
					if hrItem == 0 && folderItem != 0 {
						// IFileDialog::SetFolder (index 12)
						_, _, _ = syscall.SyscallN(vtable[12], dlg, folderItem)
						itemVtable := *(*[8]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(folderItem))))
						syscall.SyscallN(itemVtable[2], folderItem)
					}
				}
				break
			}
		}

		// IModalWindow::Show (index 3)
		hrShow, _, _ := syscall.SyscallN(vtable[3], dlg, fgHwnd)
		if uint32(hrShow) == hrCancelled {
			return "", nil
		}
		if hrShow == 0 {
			var resultItem uintptr
			// IFileDialog::GetResult (index 20)
			hrRes, _, _ := syscall.SyscallN(vtable[20], dlg, uintptr(unsafe.Pointer(&resultItem)))
			if hrRes == 0 && resultItem != 0 {
				itemVtable := *(*[8]uintptr)(unsafe.Pointer(*(*uintptr)(unsafe.Pointer(resultItem))))
				defer syscall.SyscallN(itemVtable[2], resultItem)
				var pszPath uintptr
				// IShellItem::GetDisplayName (index 5)
				hrName, _, _ := syscall.SyscallN(itemVtable[5], resultItem, sigdnFileSysPath, uintptr(unsafe.Pointer(&pszPath)))
				if hrName == 0 && pszPath != 0 {
					defer procCoTaskMemFree.Call(pszPath)
					return syscall.UTF16ToString((*[1024]uint16)(unsafe.Pointer(pszPath))[:]), nil
				}
			}
		}
	}

	// 폴백: SHBrowseForFolderW
	const (
		bifReturnOnlyFSDirs = 0x00000001
		bifEditBox          = 0x00000010
		bifNewDialogStyle   = 0x00000040
	)
	displayBuf := make([]uint16, 1024)
	bi := browseInfoW{
		hwndOwner:      fgHwnd,
		pszDisplayName: &displayBuf[0],
		lpszTitle:      titleUTF16,
		ulFlags:        bifReturnOnlyFSDirs | bifEditBox | bifNewDialogStyle,
	}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil
	}
	defer procCoTaskMemFree.Call(pidl)

	pathBuf := make([]uint16, 1024)
	ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	if ok == 0 {
		return "", fmt.Errorf("선택한 폴더 경로를 읽을 수 없습니다")
	}
	return syscall.UTF16ToString(pathBuf), nil
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

var reScriptVersion = regexp.MustCompile(`SCRIPT_VERSION\s*=\s*([0-9]+)`)

// ParseScriptVersion: 스크립트 내용에서 SCRIPT_VERSION 정수 버전을 추출합니다. (버전 상수가 없는 구버전은 0 반환)
func ParseScriptVersion(data []byte) int {
	if m := reScriptVersion.FindSubmatch(data); len(m) > 1 {
		if v, err := strconv.Atoi(string(m[1])); err == nil {
			return v
		}
	}
	return 0
}

func sameScriptContent(a, b []byte) bool {
	normA := strings.TrimSpace(strings.ReplaceAll(string(a), "\r\n", "\n"))
	normB := strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n"))
	return normA == normB
}

// CheckScriptStatusWithVersion: 스크립트 설치 여부, 업데이트 필요 여부(< 비교), 설치된 버전 및 최신 버전을 함께 반환합니다.
func CheckScriptStatusWithVersion(scriptsDir string, latestScript []byte) (installed bool, needsUpdate bool, installedVer int, latestVer int) {
	latestVer = ParseScriptVersion(latestScript)
	if scriptsDir == "" {
		scriptsDir, _ = DetectObsScriptsDir()
	}
	if scriptsDir == "" {
		return false, false, 0, latestVer
	}
	targetFile := filepath.Join(scriptsDir, "chzzk_dock_launcher.lua")
	existingData, err := os.ReadFile(targetFile)
	if err != nil || len(existingData) == 0 {
		return false, false, 0, latestVer
	}
	installed = true
	installedVer = ParseScriptVersion(existingData)

	// 정수 버전이 부여된 스크립트는 버전 번호(< 비교)만으로 업데이트 필요 여부를 판별합니다.
	// (exe 경로 차이, 수동 내보내기, 주석 변경 등으로 동일 버전에서 반복 업데이트 팝업이 뜨는 현상 방지)
	if latestVer > 0 {
		needsUpdate = installedVer < latestVer
	} else if installedVer > 0 {
		needsUpdate = false
	} else {
		expectedData := PrepareLauncherScriptWithExePath(latestScript)
		needsUpdate = !sameScriptContent(existingData, expectedData)
	}

	return installed, needsUpdate, installedVer, latestVer
}

// ValidateObsScriptsDir: 도출된 scripts 경로와 사용자가 선택한 원본 경로가 실제 OBS 환경인지 검증합니다.
func ValidateObsScriptsDir(resolvedDir, rawSelected string) (bool, string) {
	resolvedDir = strings.TrimSpace(resolvedDir)
	if resolvedDir == "" {
		return false, "OBS scripts 폴더 경로를 확인할 수 없습니다. OBS 설치 폴더(예: obs-studio), bin\\64bit 또는 scripts 폴더를 선택해 주세요."
	}

	hasDir := func(p string) bool {
		if p == "" {
			return false
		}
		st, err := os.Stat(p)
		return err == nil && st.IsDir()
	}
	hasFile := func(p string) bool {
		if p == "" {
			return false
		}
		st, err := os.Stat(p)
		return err == nil && !st.IsDir()
	}

	// rawSelected가 .exe 파일로 들어온 경우(예: 사용자가 obs64.exe 외의 다른 .exe를 고른 경우) 파일명 확인
	if raw := strings.TrimSpace(strings.Trim(rawSelected, `"'`)); raw != "" {
		lowerRaw := strings.ToLower(raw)
		if strings.HasSuffix(lowerRaw, ".exe") {
			exeBase := strings.ToLower(filepath.Base(raw))
			if exeBase != "obs64.exe" && exeBase != "obs32.exe" {
				return false, "선택한 파일이 obs64.exe(또는 obs32.exe)가 아닙니다. OBS 실행 파일(obs64.exe)을 선택해 주세요."
			}
			// obs64.exe를 선택했더라도 주변에 scripts 폴더가 실제로 존재하지 않으면 경고
			if !hasDir(resolvedDir) {
				return false, "선택한 obs64.exe 주변에서 OBS scripts 폴더를 찾지 못했습니다. 정상적인 OBS 설치 폴더인지 확인해 주세요."
			}
			return true, ""
		}
	}

	// 1. resolvedDir 자체가 실제 존재하는 scripts 디렉터리이고, OBS 기본 스크립트나 표준 OBS 경로 구조에 속하는지 확인
	if hasDir(resolvedDir) {
		for _, builtin := range []string{"clock-source.lua", "countdown.lua", "instant-replay.lua", "pause-scene.lua", "url-text.py"} {
			if hasFile(filepath.Join(resolvedDir, builtin)) {
				return true, ""
			}
		}
		normResolved := strings.ToLower(filepath.ToSlash(filepath.Clean(resolvedDir)))
		if strings.HasSuffix(normResolved, "/core/frontend-tools/data/scripts") ||
			strings.HasSuffix(normResolved, "/data/obs-plugins/frontend-tools/scripts") {
			return true, ""
		}
	}

	// 2. 선택 경로 또는 도출 경로의 상위에서 OBS 실행 파일(obs64.exe/obs32.exe), 표준 scripts 폴더 또는 내부 bin+(core|data) 구조 확인
	checkObsRoot := func(root string) bool {
		if root == "" || root == "." || root == string(filepath.Separator) {
			return false
		}
		return hasFile(filepath.Join(root, "bin", "64bit", "obs64.exe")) ||
			hasFile(filepath.Join(root, "bin", "32bit", "obs32.exe")) ||
			hasFile(filepath.Join(root, "obs64.exe")) ||
			hasFile(filepath.Join(root, "obs32.exe")) ||
			hasDir(filepath.Join(root, "core", "frontend-tools", "data", "scripts")) ||
			hasDir(filepath.Join(root, "data", "obs-plugins", "frontend-tools", "scripts")) ||
			(hasDir(filepath.Join(root, "bin")) && (hasDir(filepath.Join(root, "core")) || hasDir(filepath.Join(root, "data"))))
	}

	rootsToCheck := []string{}
	if raw := strings.TrimSpace(strings.Trim(rawSelected, `"'`)); raw != "" {
		cleanRaw := filepath.Clean(raw)
		rootsToCheck = append(rootsToCheck,
			cleanRaw,
			filepath.Dir(cleanRaw),
			filepath.Dir(filepath.Dir(cleanRaw)),
		)
	}
	rootsToCheck = append(rootsToCheck,
		filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(resolvedDir)))),
	)
	for _, r := range rootsToCheck {
		if checkObsRoot(r) {
			return true, ""
		}
	}

	return false, "선택한 폴더에서 OBS Studio 구조(bin, core, data 또는 scripts 폴더)를 찾지 못했습니다. 잘못된 위치에 설치하면 OBS에서 스크립트가 보이지 않을 수 있습니다."
}

// ObsScriptCleanupCandidates: 새 설치 경로(excludeDir) 외에 기존 chzzk_dock_launcher.lua가 남아있는 경로 목록을 반환합니다.
func ObsScriptCleanupCandidates(excludeDir string, extraDirs ...string) []string {
	normExclude := strings.ToLower(filepath.Clean(strings.TrimSpace(excludeDir)))
	seen := map[string]bool{}
	var result []string

	addDir := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return
		}
		cleanDir := filepath.Clean(dir)
		if strings.ToLower(cleanDir) == normExclude {
			return
		}
		luaPath := filepath.Join(cleanDir, "chzzk_dock_launcher.lua")
		key := strings.ToLower(luaPath)
		if seen[key] {
			return
		}
		seen[key] = true
		if st, err := os.Stat(luaPath); err == nil && !st.IsDir() {
			result = append(result, luaPath)
		}
	}

	for _, d := range extraDirs {
		addDir(d)
	}
	for _, d := range []string{
		`C:\Program Files\obs-studio\core\frontend-tools\data\scripts`,
		`C:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		`C:\Program Files (x86)\obs-studio\core\frontend-tools\data\scripts`,
		`C:\Program Files (x86)\obs-studio\data\obs-plugins\frontend-tools\scripts`,
	} {
		addDir(d)
	}
	if runningPath := FindRunningObsPath(); runningPath != "" {
		addDir(ResolveObsScriptsDir(runningPath))
	}

	// excludeDir가 core\... 또는 data\... 형태라면 같은 OBS 루트의 반대쪽 경로도 검사
	normSlashed := filepath.ToSlash(filepath.Clean(excludeDir))
	lowerSlashed := strings.ToLower(normSlashed)
	if idx := strings.Index(lowerSlashed, "/core/frontend-tools/data/scripts"); idx != -1 {
		root := filepath.FromSlash(normSlashed[:idx])
		addDir(filepath.Join(root, "data", "obs-plugins", "frontend-tools", "scripts"))
	} else if idx := strings.Index(lowerSlashed, "/data/obs-plugins/frontend-tools/scripts"); idx != -1 {
		root := filepath.FromSlash(normSlashed[:idx])
		addDir(filepath.Join(root, "core", "frontend-tools", "data", "scripts"))
	}

	return result
}

// InstallLauncherScriptToObs: 지정된(또는 자동 감지된) OBS scripts 폴더에 스크립트를 추가하고, 기존 위치의 구 파일을 제거합니다.
func InstallLauncherScriptToObs(customDir string, scriptData []byte, previousDirs ...string) (string, error) {
	var targetDir string
	if customDir != "" {
		targetDir = ResolveObsScriptsDir(customDir)
	} else {
		targetDir, _ = DetectObsScriptsDir()
	}
	if strings.TrimSpace(targetDir) == "" {
		return "", fmt.Errorf("OBS scripts 폴더를 찾을 수 없습니다")
	}
	if strings.ContainsAny(targetDir, "\"\r\n") || !filepath.IsAbs(targetDir) {
		return "", fmt.Errorf("유효하지 않은 OBS scripts 경로입니다")
	}
	cleanTargetDir := filepath.Clean(targetDir)
	if filepath.Dir(cleanTargetDir) == cleanTargetDir {
		return "", fmt.Errorf("드라이브 루트 경로에는 스크립트를 설치할 수 없습니다")
	}
	targetDir = cleanTargetDir

	scriptData = PrepareLauncherScriptWithExePath(scriptData)
	targetPath := filepath.Join(targetDir, "chzzk_dock_launcher.lua")
	cleanupFiles := ObsScriptCleanupCandidates(targetDir, previousDirs...)

	_ = os.MkdirAll(targetDir, 0755)
	writeErr := os.WriteFile(targetPath, scriptData, 0644)

	var remainingCleanup []string
	for _, oldFile := range cleanupFiles {
		if err := os.Remove(oldFile); err != nil && !os.IsNotExist(err) {
			remainingCleanup = append(remainingCleanup, oldFile)
		}
	}

	if writeErr == nil && len(remainingCleanup) == 0 {
		return targetPath, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
	}

	args := fmt.Sprintf(`--install-script "%s"`, strings.TrimRight(targetDir, `\`))
	for _, oldFile := range cleanupFiles {
		cleanOld := filepath.Clean(oldFile)
		if strings.ContainsAny(oldFile, "\"\r\n") || !strings.EqualFold(filepath.Base(cleanOld), "chzzk_dock_launcher.lua") {
			continue
		}
		args += fmt.Sprintf(` --cleanup "%s"`, cleanOld)
	}

	opPtr, _ := syscall.UTF16PtrFromString("runas")
	filePtr, _ := syscall.UTF16PtrFromString(exePath)
	paramPtr, _ := syscall.UTF16PtrFromString(args)

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

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if written, err := os.ReadFile(targetPath); err == nil && len(written) > 0 {
			if sameScriptContent(written, scriptData) {
				allCleaned := true
				for _, oldFile := range remainingCleanup {
					if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
						allCleaned = false
						break
					}
				}
				if allCleaned {
					return targetPath, nil
				}
			}
		}
	}

	return "", fmt.Errorf("관리자 권한 프로세스 실행 후 최신 스크립트 반영을 확인하지 못했습니다")
}

