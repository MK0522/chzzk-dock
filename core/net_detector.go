package core

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/big"
	"net"
	"syscall"
	"unsafe"
)

// ==============================================================================
// [CHZZK OBS DOCK - Network Port Detector & Safe Fallback Allocator]
// - Windows IP Helper API (iphlpapi.dll -> GetExtendedTcpTable) 바인딩
// - Zero-CMD / Zero-PowerShell: 콘솔 깜빡임 없이 1ms 내 포트 점유 프로세스(PID, EXE) 탐색
// - IANA 사설/동적 포트 대역(49152 ~ 65535) 기반 안전 랜덤 Fallback 포트 바인딩
// ==============================================================================

var (
	iphlpapiDLL             = syscall.NewLazyDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapiDLL.NewProc("GetExtendedTcpTable")
)

type MIB_TCPROW_OWNER_PID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

// GetProcessNameByPid: PID에 해당하는 실행 파일명(.exe)을 조회합니다.
func GetProcessNameByPid(pid uint32) string {
	if pid == 0 {
		return ""
	}
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
		if entry.Th32ProcessID == pid {
			return syscall.UTF16ToString(entry.SzExeFile[:])
		}
		ret, _, _ = procProcess32NextW.Call(hSnap, uintptr(unsafe.Pointer(&entry)))
		if ret == 0 {
			break
		}
	}
	return ""
}

// FindProcessUsingPort: 지정된 TCP 포트를 점유(LISTEN) 중인 프로세스의 PID와 이름을 반환합니다.
func FindProcessUsingPort(port int) (uint32, string) {
	var size uint32
	// AF_INET = 2, TCP_TABLE_OWNER_PID_ALL = 5
	procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 1, 2, 5, 0)
	if size == 0 {
		return 0, ""
	}

	buf := make([]byte, size)
	ret, _, _ := procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		1, 2, 5, 0,
	)
	if ret != 0 {
		return 0, ""
	}

	numEntries := binary.LittleEndian.Uint32(buf[0:4])
	rowSize := unsafe.Sizeof(MIB_TCPROW_OWNER_PID{})
	offset := 4

	for i := uint32(0); i < numEntries; i++ {
		if offset+int(rowSize) > len(buf) {
			break
		}
		row := (*MIB_TCPROW_OWNER_PID)(unsafe.Pointer(&buf[offset]))
		offset += int(rowSize)

		// row.LocalPort는 네트워크 바이트 순서(Big-Endian)
		p := binary.BigEndian.Uint16([]byte{byte(row.LocalPort), byte(row.LocalPort >> 8)})
		if int(p) == port {
			pname := GetProcessNameByPid(row.OwningPid)
			return row.OwningPid, pname
		}
	}
	return 0, ""
}

// CheckPortAvailable: 지정된 포트가 사용 가능한지 검사합니다.
// - 사용 가능 시: (true, 0, "", nil)
// - 점유 중일 시: (false, pid, processName, err)
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

// FindSafeFallbackPort: IANA 사설/동적 포트 대역(49152 ~ 65535)에서
// 즉시 바인딩 가능한 안전한 랜덤 포트를 탐색하여 (포트, 활성 Listener)를 반환합니다.
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

	// 30회 랜덤 시도 실패 시 커널에게 0번 포트 할당 요청
	fallbackListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, fmt.Errorf("대체 포트 할당 실패: %w", err)
	}
	assignedPort := fallbackListener.Addr().(*net.TCPAddr).Port
	return assignedPort, fallbackListener, nil
}
