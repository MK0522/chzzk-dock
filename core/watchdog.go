package core

import (
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	TH32CS_SNAPPROCESS = 0x00000002
)

var (
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
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

// IsObsRunning: 시스템 프로세스 목록에서 obs64.exe 또는 obs32.exe가 실행 중인지 확인
func IsObsRunning() bool {
	hSnap, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
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
		if strings.EqualFold(exeName, "obs64.exe") || strings.EqualFold(exeName, "obs32.exe") {
			return true
		}

		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}

	return false
}

var (
	watchdogMu         sync.RWMutex
	currentWatchdogSec int = 60
)

// GetWatchdogTimeoutSec: 현재 설정된 OBS 미실행 대기 시간(초)을 반환합니다.
func GetWatchdogTimeoutSec() int {
	watchdogMu.RLock()
	defer watchdogMu.RUnlock()
	return currentWatchdogSec
}

// SetWatchdogTimeoutSec: OBS 미실행 대기 시간(초)을 동적으로 변경합니다.
func SetWatchdogTimeoutSec(sec int) {
	watchdogMu.Lock()
	defer watchdogMu.Unlock()
	if sec <= 0 {
		sec = 60
	}
	currentWatchdogSec = sec
}

// StartObsWatchdog: OBS 생명주기 감시 고루틴 실행
// - OBS 실행 감지 후, OBS가 종료되면 설정된 대기 시간(GetWatchdogTimeoutSec) 동안 대기 후 자동 종료
// - 대기 시간 내에 OBS가 재실행되면 카운트다운을 즉시 취소하고 연동 복구
func StartObsWatchdog(silentMode bool) {
	settings := LoadSettings()
	SetWatchdogTimeoutSec(settings.WatchdogTimeoutSec)

	go func() {
		LogInfo("[Watchdog] OBS 프로세스 감시 시작 (종료 대기 유예 시간: %d초, Silent: %v)", GetWatchdogTimeoutSec(), silentMode)

		obsEverDetected := false
		var obsStoppedAt time.Time
		startTime := time.Now()

		for {
			time.Sleep(1 * time.Second)

			running := IsObsRunning()

			if running {
				if !obsEverDetected {
					LogInfo("[Watchdog] OBS Studio 프로세스(obs64.exe) 감지 완료 -> 연동 모드 활성화")
					obsEverDetected = true
				}
				if !obsStoppedAt.IsZero() {
					LogInfo("[Watchdog] OBS Studio 재실행 감지 -> 자동 종료 카운트다운 취소 및 정상 모드 복구")
					obsStoppedAt = time.Time{}
				}
			} else {
				if obsEverDetected {
					// OBS가 실행 중이었다가 종료된 경우
					if obsStoppedAt.IsZero() {
						obsStoppedAt = time.Now()
						timeoutSec := GetWatchdogTimeoutSec()
						LogInfo("[Watchdog] OBS Studio 프로세스 종료 감지! 설정된 대기 시간(%d초) 카운트다운을 시작합니다.", timeoutSec)
					} else {
						timeoutSec := GetWatchdogTimeoutSec()
						if timeoutSec > 0 && time.Since(obsStoppedAt) >= time.Duration(timeoutSec)*time.Second {
							LogInfo("[Watchdog] OBS Studio 종료 후 대기 시간(%d초) 만료 -> 치지직 독 서버를 안전하게 자동 종료합니다.", timeoutSec)
							DestroyDockWindow()
							if GlobalTray != nil {
								GlobalTray.Stop()
							}
							os.Exit(0)
						}
					}
				} else if silentMode {
					// 백그라운드 스크립트 실행 모드에서 시작 후 한 번도 OBS가 안 켜진 경우 고아 프로세스 방지
					timeoutSec := GetWatchdogTimeoutSec()
					if timeoutSec > 0 && time.Since(startTime) > time.Duration(timeoutSec)*time.Second {
						LogInfo("[Watchdog] 백그라운드 대기 시간(%d초) 내에 OBS Studio가 실행되지 않음 -> 서버 자동 종료", timeoutSec)
						os.Exit(0)
					}
				}
			}
		}
	}()
}
