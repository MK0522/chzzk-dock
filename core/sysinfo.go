package core

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// ==============================================================================
// [CHZZK OBS DOCK - Ultra-Lightweight System Diagnostics (Ponytail)]
// - 외부 패키지 0개, WMI/PowerShell 프로세스 생성 배제 (0ms 즉시 응답, 백신 오탐 0)
// - Windows 레지스트리 + Win32/SMBIOS 표준 API 직접 쿼리
// ==============================================================================

// 레지스트리 문자열 읽기 헬퍼
func regStr(k registry.Key, path, key string) string {
	handle, err := registry.OpenKey(k, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer handle.Close()
	val, _, _ := handle.GetStringValue(key)
	return strings.TrimSpace(val)
}

// 레지스트리 DWORD 읽기 헬퍼
func regDword(k registry.Key, path, key string) uint32 {
	handle, err := registry.OpenKey(k, path, registry.QUERY_VALUE)
	if err != nil {
		return 0
	}
	defer handle.Close()
	val, _, _ := handle.GetIntegerValue(key)
	return uint32(val)
}

// GetOSInfo: Windows 10 vs 11, 에디션(Pro/Home), 빌드 번호 정밀 조합
func GetOSInfo() string {
	ntKey := `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	buildNum := regDword(registry.LOCAL_MACHINE, ntKey, "CurrentBuildNumber")
	if buildNum == 0 {
		// 문자열로 저장되어 있는 환경 대응
		buildStr := regStr(registry.LOCAL_MACHINE, ntKey, "CurrentBuildNumber")
		var b uint32
		_, _ = fmt.Sscanf(buildStr, "%d", &b)
		buildNum = b
	}

	osName := "Windows 10"
	if buildNum >= 22000 {
		osName = "Windows 11"
	}

	edition := regStr(registry.LOCAL_MACHINE, ntKey, "EditionID")
	switch strings.ToLower(edition) {
	case "professional":
		edition = "Pro"
	case "core":
		edition = "Home"
	}

	displayVer := regStr(registry.LOCAL_MACHINE, ntKey, "DisplayVersion")
	ubr := regDword(registry.LOCAL_MACHINE, ntKey, "UBR")

	var buildPart string
	if ubr > 0 {
		buildPart = fmt.Sprintf("Build %d.%d", buildNum, ubr)
	} else if buildNum > 0 {
		buildPart = fmt.Sprintf("Build %d", buildNum)
	}

	parts := []string{osName}
	if edition != "" {
		parts = append(parts, edition)
	}

	extra := []string{}
	if displayVer != "" {
		extra = append(extra, displayVer)
	}
	if buildPart != "" {
		extra = append(extra, buildPart)
	}

	if len(extra) > 0 {
		return fmt.Sprintf("%s (%s)", strings.Join(parts, " "), strings.Join(extra, ", "))
	}
	return strings.Join(parts, " ")
}

// GetCPUInfo: 프로세서 정식 명칭
func GetCPUInfo() string {
	cpu := regStr(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString")
	if cpu == "" {
		return "(알 수 없는 CPU)"
	}
	return cpu
}

// GetMotherboardInfo: 메인보드 상세 모델명 + 바이오스 버전
func GetMotherboardInfo() string {
	mbKey := `HARDWARE\DESCRIPTION\System\BIOS`
	model := regStr(registry.LOCAL_MACHINE, mbKey, "BaseBoardProduct")
	if model == "" {
		model = "(알 수 없는 메인보드)"
	}

	biosVer := regStr(registry.LOCAL_MACHINE, mbKey, "BIOSVersion")
	if biosVer != "" {
		return fmt.Sprintf("%s (BIOS: %s)", model, biosVer)
	}
	return model
}

// GetGPUList: 장착된 모든 물리 GPU 목록 (모델명 + 드라이버 버전 원본)
func GetGPUList() []string {
	classPath := `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, classPath, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()

	subKeys, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}

	var gpus []string
	for _, sub := range subKeys {
		// 0000, 0001 등 4자리 숫자 서브키만 조회 (Properties 등 시스템 키 무시)
		if len(sub) != 4 || sub < "0000" || sub > "9999" {
			continue
		}
		itemPath := classPath + `\` + sub
		desc := regStr(registry.LOCAL_MACHINE, itemPath, "DriverDesc")
		ver := regStr(registry.LOCAL_MACHINE, itemPath, "DriverVersion")

		if desc != "" {
			if ver != "" {
				gpus = append(gpus, fmt.Sprintf("%s (%s)", desc, ver))
			} else {
				gpus = append(gpus, desc)
			}
		}
	}
	return gpus
}

// GetRAMInfo: 전체 메모리 용량 + 작업관리자와 동일한 현재 동작 클럭(MT/s) 및 슬롯 위치
func GetRAMInfo() string {
	// 1. 총 물리 메모리 (GB)
	type memoryStatusEx struct {
		cbSize                  uint32
		dwMemoryLoad            uint32
		ullTotalPhys            uint64
		ullAvailPhys            uint64
		ullTotalPageFile        uint64
		ullAvailPageFile        uint64
		ullTotalVirtual         uint64
		ullAvailVirtual         uint64
		ullAvailExtendedVirtual uint64
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	var mem memoryStatusEx
	mem.cbSize = uint32(unsafe.Sizeof(mem))
	_, _, _ = kernel32.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&mem)))
	ramGB := float64(mem.ullTotalPhys) / (1024 * 1024 * 1024)

	// 2. SMBIOS Type 16/17 파싱하여 메모리 규격(D4/D5), 설정 클럭(MT/s), 슬롯 수 추출
	memType, clockSpeed, usedSlots, totalSlots := getSmbiosRAMDetails()

	// [추후 확장 메모 / 보류 사항]
	// - 각 슬롯별 용량 파싱: SMBIOS Type 17의 Offset 0x0C(Size) 및 0x1C(Extended Size)를 읽어
	//   슬롯별 실제 장착 용량(예: 24GB + 24GB, 16GB + 8GB 짝짝이) 세부 진단 가능.
	// - A/B 채널 구분: Type 17의 Offset 0x10(DeviceLocator) / 0x11(BankLocator) 문자열에서
	//   DIMM_A2, DIMM_B2 등 듀얼 채널 슬롯 위치 파싱 가능 (바이오스 제조사별 문자열 차이 고려 필요).
	// - 현재는 가독성과 일관성을 위해 "D5 6800MT/s, 47.6 GB (2/2 DIMM)" 간결한 단일 포맷 유지.

	var speedPart string
	if memType != "" && clockSpeed > 0 {
		speedPart = fmt.Sprintf("%s %dMT/s", memType, clockSpeed)
	} else if memType != "" {
		speedPart = memType
	} else if clockSpeed > 0 {
		speedPart = fmt.Sprintf("%dMT/s", clockSpeed)
	}

	var ramStr string
	if speedPart != "" {
		ramStr = fmt.Sprintf("%s, %.1f GB", speedPart, ramGB)
	} else {
		ramStr = fmt.Sprintf("%.1f GB", ramGB)
	}

	if totalSlots > 0 && usedSlots > 0 {
		ramStr += fmt.Sprintf(" (%d/%d DIMM)", usedSlots, totalSlots)
	} else if usedSlots > 0 {
		ramStr += fmt.Sprintf(" (%d DIMM)", usedSlots)
	}
	return ramStr
}

// getSmbiosRAMDetails: kernel32!GetSystemFirmwareTable 로 규격(DDR4/DDR5), 최고 속도, 장착 슬롯 수, 전체 슬롯 수 추출
func getSmbiosRAMDetails() (string, uint16, int, int) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procGetSystemFirmwareTable := kernel32.NewProc("GetSystemFirmwareTable")

	const rsmbSig = 0x52534D42 // 'RSMB'
	size, _, _ := procGetSystemFirmwareTable.Call(rsmbSig, 0, 0, 0)
	if size == 0 {
		return "", 0, 0, 0
	}

	buf := make([]byte, size)
	ret, _, _ := procGetSystemFirmwareTable.Call(rsmbSig, 0, uintptr(unsafe.Pointer(&buf[0])), size)
	if ret == 0 || len(buf) < 8 {
		return "", 0, 0, 0
	}

	tableData := buf[8:] // RawSMBIOSData 헤더(8바이트) 건너뜀

	var rawMemType byte
	var maxConfiguredSpeed uint16
	var usedSlots int
	var totalSlots int
	var type17Count int

	offset := 0
	for offset+4 <= len(tableData) {
		structType := tableData[offset]
		formattedLen := int(tableData[offset+1])
		if formattedLen < 4 || offset+formattedLen > len(tableData) {
			break
		}

		// Type 16: Physical Memory Array (메인보드 물리 슬롯 총 개수)
		if structType == 16 && formattedLen >= 0x10 {
			totalSlots = int(binary.LittleEndian.Uint16(tableData[offset+0x0E : offset+0x10]))
		}

		// Type 17: Memory Device (개별 슬롯)
		if structType == 17 && formattedLen >= 0x13 {
			type17Count++
			if rawMemType == 0 && tableData[offset+0x12] > 0 {
				rawMemType = tableData[offset+0x12]
			}
			var confSpeed uint16
			if formattedLen >= 0x22 {
				confSpeed = binary.LittleEndian.Uint16(tableData[offset+0x20 : offset+0x22])
			}
			if confSpeed > maxConfiguredSpeed {
				maxConfiguredSpeed = confSpeed
			}
			if confSpeed > 0 {
				usedSlots++
			}
		}

		// 다음 구조체로 이동 (이중 null 탐색)
		nextOffset := offset + formattedLen
		for nextOffset+1 < len(tableData) {
			if tableData[nextOffset] == 0 && tableData[nextOffset+1] == 0 {
				nextOffset += 2
				break
			}
			nextOffset++
		}
		if nextOffset <= offset {
			break
		}
		offset = nextOffset
	}

	if totalSlots == 0 {
		totalSlots = type17Count
	}

	memTypeStr := smbiosMemTypeToString(rawMemType, maxConfiguredSpeed)
	return memTypeStr, maxConfiguredSpeed, usedSlots, totalSlots
}

// SMBIOS MemoryType 바이트를 DDR 세대 문자열(D3/D4/D5)로 변환
func smbiosMemTypeToString(t byte, speed uint16) string {
	switch t {
	case 0x18:
		return "D3"
	case 0x1A:
		return "D4"
	case 0x1E:
		return "LPD4"
	case 0x22:
		return "D5"
	case 0x23:
		return "LPD5"
	default:
		// 미등록 바이오스 폴백 (클럭 기준 추정)
		if speed >= 4800 {
			return "D5"
		} else if speed >= 2133 {
			return "D4"
		} else if speed >= 800 {
			return "D3"
		}
		return ""
	}
}


// GetSystemSpecLines: 전체 하드웨어/OS 사양을 라인별로 포맷팅하여 반환
func GetSystemSpecLines(appVersion string) []string {
	lines := []string{
		fmt.Sprintf("- APP : %s", appVersion),
		fmt.Sprintf("- OS: %s", GetOSInfo()),
		fmt.Sprintf("- CPU: %s", GetCPUInfo()),
		fmt.Sprintf("- RAM : %s", GetRAMInfo()),
		fmt.Sprintf("- M/B: %s", GetMotherboardInfo()),
	}

	gpus := GetGPUList()
	if len(gpus) == 0 {
		lines = append(lines, "- GPU: (감지되지 않음)")
	} else {
		for i, g := range gpus {
			lines = append(lines, fmt.Sprintf("- GPU%d: %s", i, g))
		}
	}

	return lines
}
