package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// ==============================================================================
//  In-Memory Logger
//  - 디버깅을 위해 메모리 링 버퍼(최대 5,000줄)에 로그를 보관
//  - 프로그램 종료 시 자동 소멸 (비저장 시 휘발)
//  - 트레이 메뉴에서 [로그 확인하기] (메모장 실행) / [로그 저장] (.txt 파일 내보내기)
//  - ERROR 레벨 발생 시 윈도우 알림(Alert) 자동 트리거
// ==============================================================================

const (
	MaxLogEntries = 5000
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

type LogEntry struct {
	Timestamp time.Time
	Level     LogLevel
	Message   string
}

type MemoryLogger struct {
	mu      sync.RWMutex
	entries []LogEntry
}

var (
	appLogger = &MemoryLogger{
		entries: make([]LogEntry, 0, 500),
	}
	comdlg32             = syscall.NewLazyDLL("comdlg32.dll")
	procGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")
)

// Log: 메모리 버퍼에 로그 기록 및 표준 출력 전달
func (m *MemoryLogger) Log(level LogLevel, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	entry := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   msg,
	}

	m.mu.Lock()
	if len(m.entries) >= MaxLogEntries {
		// 원형 FIFO 유지: 앞쪽 500개 제거
		m.entries = m.entries[500:]
	}
	m.entries = append(m.entries, entry)
	m.mu.Unlock()

	// 터미널 및 디버그 출력
	timeStr := entry.Timestamp.Format("2006-01-02 15:04:05")
	fmt.Printf("[%s] [%s] %s\n", timeStr, level.String(), msg)

	// ERROR 레벨 발생 시 윈도우 알림 발송
	if level == LevelError {
		ShowAlert("CHZZK OBS Dock 오류", msg)
	}
}

// GetLogsText: 지금까지 수집된 메모리 로그 전체를 텍스트로 반환
func (m *MemoryLogger) GetLogsText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("==============================================================================\n")
	sb.WriteString(fmt.Sprintf("  CHZZK OBS Dock - In-Memory Diagnostics Log (%s)\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("  * 이 로그는 메모리에만 임시 저장되며, 프로그램 종료 시 자동 삭제됩니다.\n")
	sb.WriteString("==============================================================================\n\n")

	for _, e := range m.entries {
		sb.WriteString(fmt.Sprintf("[%s] [%-5s] %s\n",
			e.Timestamp.Format("2006-01-02 15:04:05"),
			e.Level.String(),
			e.Message,
		))
	}

	if len(m.entries) == 0 {
		sb.WriteString("(기록된 로그가 없습니다.)\n")
	}

	return sb.String()
}

// Global Logging Functions
func LogDebug(format string, args ...interface{}) {
	appLogger.Log(LevelDebug, format, args...)
}

func LogInfo(format string, args ...interface{}) {
	appLogger.Log(LevelInfo, format, args...)
}

func LogWarn(format string, args ...interface{}) {
	appLogger.Log(LevelWarn, format, args...)
}

func LogError(format string, args ...interface{}) {
	appLogger.Log(LevelError, format, args...)
}

// GetLogsText: 현재까지의 로그 텍스트 취득
func GetLogsText() string {
	return appLogger.GetLogsText()
}

// ViewLogsInNotepad: 메모리에 저장된 로그를 임시 파일로 작성 후 메모장(notepad.exe)으로 열람
func ViewLogsInNotepad() error {
	logText := appLogger.GetLogsText()
	tempDir := os.TempDir()
	logPath := filepath.Join(tempDir, "chzzk_dock_current_logs.txt")

	if err := os.WriteFile(logPath, []byte(logText), 0644); err != nil {
		LogError("임시 로그 파일 생성 실패: %v", err)
		return err
	}

	cmd := exec.Command("notepad.exe", logPath)
	if err := cmd.Start(); err != nil {
		LogError("메모장 실행 실패: %v", err)
		return err
	}
	return nil
}

// 64-bit Windows tagOFNW 구조체 (GetSaveFileNameW용, 152바이트 정렬)
type openFileNameW struct {
	lStructSize       uint32
	_                 uint32
	hwndOwner         syscall.Handle
	hInstance         syscall.Handle
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	_                 uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	_                 uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	flagsEx           uint32
}

// SaveLogsWithDialog: 파일 저장 대화상자를 띄워 .txt 파일로 저장
func SaveLogsWithDialog() (string, error) {
	defaultName := fmt.Sprintf("chzzk_dock_log_%s.txt", time.Now().Format("20060102_150405"))

	// 기본 저장 폴더 (바탕화면)
	userHome, _ := os.UserHomeDir()
	desktopDir := filepath.Join(userHome, "Desktop")
	if _, err := os.Stat(desktopDir); err != nil {
		desktopDir = userHome
	}

	targetPath := ""

	// Win32 GetSaveFileNameW 호출 시도
	buf := make([]uint16, 1024)
	copy(buf, syscall.StringToUTF16(defaultName))

	filter := "Text Files (*.txt)\x00*.txt\x00All Files (*.*)\x00*.*\x00\x00"
	filterPtr, _ := syscall.UTF16PtrFromString(filter)
	titlePtr, _ := syscall.UTF16PtrFromString("CHZZK OBS Dock 로그 저장")
	initialDirPtr, _ := syscall.UTF16PtrFromString(desktopDir)
	defExtPtr, _ := syscall.UTF16PtrFromString("txt")

	ofn := openFileNameW{
		lStructSize:     uint32(unsafe.Sizeof(openFileNameW{})),
		lpstrFilter:     filterPtr,
		nFilterIndex:    1,
		lpstrFile:       &buf[0],
		nMaxFile:        uint32(len(buf)),
		lpstrInitialDir: initialDirPtr,
		lpstrTitle:      titlePtr,
		lpstrDefExt:     defExtPtr,
		flags:           0x00000002 | 0x00000004, // OFN_OVERWRITEPROMPT | OFN_HIDEREADONLY
	}

	ret, _, _ := procGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	if ret != 0 {
		targetPath = syscall.UTF16ToString(buf)
	}

	// 대화상자 취소 시 아무 작업도 하지 않음
	if targetPath == "" {
		return "", nil
	}

	logText := appLogger.GetLogsText()
	if err := os.WriteFile(targetPath, []byte(logText), 0644); err != nil {
		LogError("로그 파일 저장 실패 (%s): %v", targetPath, err)
		return "", err
	}

	LogInfo("진단 로그 저장 완료: %s", targetPath)
	if GlobalTray != nil {
		GlobalTray.ShowNotification("로그 저장 완료", fmt.Sprintf("로그 파일이 저장되었습니다:\n%s", targetPath))
	}

	// 저장된 파일을 탐색기에서 선택된 상태로 표시
	_ = exec.Command("explorer.exe", fmt.Sprintf("/select,%s", targetPath)).Start()

	return targetPath, nil
}

// SaveLogsToFile: 지정된 경로에 로그 텍스트를 즉시 파일로 저장
func SaveLogsToFile(targetPath string) error {
	logText := appLogger.GetLogsText()
	return os.WriteFile(targetPath, []byte(logText), 0644)
}

// ClearLogs: 메모리 로그 초기화 (테스트용)
func ClearLogs() {
	appLogger.mu.Lock()
	defer appLogger.mu.Unlock()
	appLogger.entries = appLogger.entries[:0]
}
