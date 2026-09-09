package core

import (
	"fmt"
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
// - OBS가 실행 중인 상태에서 꺼지면 3초 후 자체 종료 (자폭)
// - 단독 실행 시 유예 시간(설정된 WatchdogTimeoutSec) 동안 OBS 실행을 대기함
func StartObsWatchdog() {
	settings := LoadSettings()
	SetWatchdogTimeoutSec(settings.WatchdogTimeoutSec)

	go func() {
		fmt.Printf("[Watchdog] OBS 프로세스 감시 시작 (대기 유예 시간: %d초)\n", GetWatchdogTimeoutSec())
		
		obsEverDetected := false
		startTime := time.Now()

		for {
			time.Sleep(3 * time.Second)

			running := IsObsRunning()

			if running {
				if !obsEverDetected {
					fmt.Println("[Watchdog] OBS Studio 프로세스(obs64.exe) 감지 완료 -> 연동 모드 활성화")
					obsEverDetected = true
				}
			} else {
				// OBS가 켜져 있다가 꺼진 경우 -> 3초 후 즉시 자폭
				if obsEverDetected {
					fmt.Println("[Watchdog] OBS Studio 종료 감지 -> 3초 후 치지직 독 서버를 자동으로 안전하게 종료합니다.")
					time.Sleep(3 * time.Second)
					os.Exit(0)
				}

				// 시작 후 한 번도 OBS가 안 켜졌고, 동적으로 설정된 유예 시간을 초과한 경우
				timeoutSec := GetWatchdogTimeoutSec()
				if timeoutSec > 0 && time.Since(startTime) > time.Duration(timeoutSec)*time.Second {
					fmt.Printf("[Watchdog] 대기 유예 시간(%d초) 내에 OBS Studio가 실행되지 않음 -> 서버 자동 종료\n", timeoutSec)
					os.Exit(0)
				}
			}
		}
	}()
}
