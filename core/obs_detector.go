package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procShellExecuteW              = shell32.NewProc("ShellExecuteW")
	procSHBrowseForFolderW         = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW       = shell32.NewProc("SHGetPathFromIDListW")

	procCoInitializeEx      = ole32DLL.NewProc("CoInitializeEx")
	procCoUninitialize      = ole32DLL.NewProc("CoUninitialize")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
)

const (
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

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

// FindRunningObsPath: 실행 중인 obs64.exe / obs32.exe의 전체 실행 파일 경로 검색
func FindRunningObsPath() string {
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
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
				defer kernel32.NewProc("CloseHandle").Call(hProcess)
				var buf [1024]uint16
				size := uint32(len(buf))
				qRet, _, _ := procQueryFullProcessImageNameW.Call(hProcess, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
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

// DetectObsScriptsDir: OBS Studio의 scripts 디렉터리를 탐색하고 실제 디스크 존재 여부를 반환합니다.
func DetectObsScriptsDir() (string, bool) {
	// 1순위: 현재 실행 중인 OBS 프로세스로부터 추론
	if runningPath := FindRunningObsPath(); runningPath != "" {
		// runningPath 예: C:\Program Files\obs-studio\bin\64bit\obs64.exe
		obsRoot := filepath.Dir(filepath.Dir(filepath.Dir(runningPath))) // 3단계 상위 (bin/64bit/obs64.exe -> obsRoot)
		candidate := filepath.Join(obsRoot, "data", "obs-plugins", "frontend-tools", "scripts")
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate, true
		}
	}

	// 2순위: 표준 설치 경로 목록 확인
	candidates := []string{
		`C:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		`C:\Program Files (x86)\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		`D:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		`E:\Program Files\obs-studio\data\obs-plugins\frontend-tools\scripts`,
		filepath.Join(os.Getenv("APPDATA"), "obs-studio", "scripts"),
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

// ResolveObsScriptsDir: 사용자가 입력하거나 선택한 폴더 경로로부터 scripts 디렉터리 경로를 스마트하게 도출합니다.
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
	scriptsSub := filepath.Join(cleanPath, "scripts")
	if stat, err := os.Stat(scriptsSub); err == nil && stat.IsDir() {
		return scriptsSub
	}

	// 4. bin\64bit 또는 bin 디렉터리를 선택한 경우 루트로 거슬러 올라감
	norm := strings.ToLower(filepath.ToSlash(cleanPath))
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
// OBS 창 뒤로 숨지 않도록 HWND_TOPMOST 및 포그라운드 활성화를 적용합니다.
func BrowseForObsFolder(title string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// COINIT_APARTMENTTHREADED = 0x2, COINIT_DISABLE_OLE1DDE = 0x4
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

	// BFFM_INITIALIZED(1) 수신 시 창을 HWND_TOPMOST로 지정하여 OBS 최상단으로 강제 이동
	callback := syscall.NewCallback(func(hwnd syscall.Handle, uMsg uint32, lParam, lpData uintptr) uintptr {
		if uMsg == 1 { // BFFM_INITIALIZED
			hwndTopmost := ^uintptr(0) // HWND_TOPMOST = -1
			procSetWindowPos.Call(
				uintptr(hwnd),
				hwndTopmost,
				0, 0, 0, 0,
				0x0001|0x0002|0x0040, // SWP_NOMOVE | SWP_NOSIZE | SWP_SHOWWINDOW
			)
			procSetForegroundWindow.Call(uintptr(hwnd))
		}
		return 0
	})

	var displayName [260]uint16
	// BIF_RETURNONLYFSDIRS (0x0001) | BIF_NEWDIALOGSTYLE (0x0040) | BIF_EDITBOX (0x0010)
	bi := BROWSEINFOW{
		HwndOwner:      fgHwnd,
		LpszTitle:      titleUTF16,
		PszDisplayName: &displayName[0],
		UlFlags:        0x00000001 | 0x00000040 | 0x00000010,
		Lpfn:           callback,
	}

	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", nil // 사용자가 취소함
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
	// local BAKED_EXE_PATH = "" 부분을 실제 실행 파일 경로로 치환
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

	// 실제 chzzk-dock.exe 경로를 스크립트에 주입
	scriptData = PrepareLauncherScriptWithExePath(scriptData)

	targetPath := filepath.Join(targetDir, "chzzk_dock_launcher.lua")

	// 1차 시도: 일반 권한으로 직접 쓰기 (폴더가 쓰기 가능한 경우 UAC 없이 즉시 완료)
	_ = os.MkdirAll(targetDir, 0755)
	if err := os.WriteFile(targetPath, scriptData, 0644); err == nil {
		return targetPath, nil
	}

	// 2차 시도: 권한 부족(Program Files 등) 시 Win32 ShellExecuteW "runas"로 UAC 관리자 권한 요청
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
		1, // SW_SHOWNORMAL (콘솔 창 없는 Windows GUI 바이너리이므로 화면 깜빡임 없이 휴리스틱 완화)
	)

	if ret <= 32 {
		return "", fmt.Errorf("관리자 권한(UAC) 승인이 거부되었거나 실패했습니다 (코드: %d): %w", ret, err)
	}

	// UAC 자식 프로세스가 파일 작성을 완료할 때까지 최대 5초간 확인 대기
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if stat, err := os.Stat(targetPath); err == nil && stat.Size() > 0 {
			return targetPath, nil
		}
	}

	return "", fmt.Errorf("관리자 권한 프로세스 실행 후 파일 생성을 확인하지 못했습니다")
}
