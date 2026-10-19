package core

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassW      = user32.NewProc("RegisterClassW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procLoadIconW           = user32.NewProc("LoadIconW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procSendMessageW        = user32.NewProc("SendMessageW")
	procShell_NotifyIconW   = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	procGlobalLock       = kernel32.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	procGlobalFree       = kernel32.NewProc("GlobalFree")
)

const (
	WM_USER         = 0x0400
	WM_TRAYICON     = WM_USER + 20
	NIM_ADD         = 0x00000000
	NIM_MODIFY      = 0x00000001
	NIM_DELETE      = 0x00000002
	NIF_MESSAGE     = 0x00000001
	NIF_ICON        = 0x00000002
	NIF_TIP         = 0x00000004
	NIF_INFO        = 0x00000010
	NIIF_INFO       = 0x00000001
	NIIF_WARNING    = 0x00000002
	NIIF_ERROR      = 0x00000003
	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040
	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100
	MF_STRING       = 0x0000
	MF_POPUP        = 0x0010
	MF_SEPARATOR    = 0x0800
	MF_CHECKED      = 0x0008
	MF_GRAYED       = 0x0001
	MF_DISABLED     = 0x0002

	GMEM_MOVEABLE  = 0x0002
	CF_UNICODETEXT = 13

	WM_SETICON = 0x0080
	ICON_SMALL = 0
	ICON_BIG   = 1
)

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type WNDCLASSW struct {
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
}

type NOTIFYICONDATAW struct {
	CbSize            uint32
	HWnd              syscall.Handle
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             syscall.Handle
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          [16]byte
	HBalloonIcon      syscall.Handle
}

type MenuItem struct {
	Label        string
	DynamicLabel func() string
	Callback     func()
	CheckFn      func() bool
	DisabledFn   func() bool
	IsSeparator  bool
	SubItems     []MenuItem
}

type PureWinTrayIcon struct {
	Tooltip                string
	Hwnd                   syscall.Handle
	Hicon                  syscall.Handle
	Nid                    *NOTIFYICONDATAW
	MenuItems              []MenuItem
	StartNotificationTitle string
	StartNotificationMsg   string
	mu                     sync.Mutex
}

var GlobalTray *PureWinTrayIcon

// ExportLauncherScript: 실행 파일과 같은 폴더에 chzzk_dock_launcher.lua 생성 및 클립보드 복사
func ExportLauncherScript(scriptContent []byte) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
	}
	targetPath := filepath.Join(filepath.Dir(exePath), "chzzk_dock_launcher.lua")
	if err := os.WriteFile(targetPath, scriptContent, 0644); err != nil {
		return "", fmt.Errorf("스크립트 파일 생성 실패: %w", err)
	}
	_ = SetClipboardText(string(scriptContent))
	return targetPath, nil
}

// NewPureWinTrayIcon: 트레이 아이콘 인스턴스 생성
func NewPureWinTrayIcon(tooltip string) *PureWinTrayIcon {
	tray := &PureWinTrayIcon{
		Tooltip:   tooltip,
		MenuItems: make([]MenuItem, 0),
	}
	GlobalTray = tray
	return tray
}

var embeddedIconData []byte

// SetEmbeddedIcon: 바이너리에 임베드된 icon.ico 바이트 데이터 등록
func SetEmbeddedIcon(data []byte) {
	embeddedIconData = data
}

func loadIconFromFile(filePath string) syscall.Handle {
	if _, err := os.Stat(filePath); err != nil {
		return 0
	}
	pathPtr, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return 0
	}
	h, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(IMAGE_ICON),
		0, 0,
		uintptr(LR_LOADFROMFILE|LR_DEFAULTSIZE),
	)
	if h != 0 {
		return syscall.Handle(h)
	}
	return 0
}

// LoadAppIcon: 실행 파일 디렉토리, 로컬 캐시, 임베디드 리소스에서 HICON을 안전하게 로드
func LoadAppIcon() syscall.Handle {
	// 1. 실행 파일과 동일한 디렉토리에 있는 icon.ico 확인
	if exePath, err := os.Executable(); err == nil {
		exeIcon := filepath.Join(filepath.Dir(exePath), "icon.ico")
		if h := loadIconFromFile(exeIcon); h != 0 {
			return h
		}
	}

	// 2. 현재 작업 디렉토리의 icon.ico 확인
	if h := loadIconFromFile("icon.ico"); h != 0 {
		return h
	}

	// 3. 임베디드 아이콘 데이터가 있으면 로컬 앱데이터 캐시에 저장 후 로드
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	if appData != "" {
		cachedIcon := filepath.Join(appData, "ChzzkObsDock", "icon.ico")
		if len(embeddedIconData) > 0 {
			if _, err := os.Stat(cachedIcon); err != nil {
				_ = os.MkdirAll(filepath.Dir(cachedIcon), 0755)
				_ = os.WriteFile(cachedIcon, embeddedIconData, 0644)
			}
		}
		if h := loadIconFromFile(cachedIcon); h != 0 {
			return h
		}
	}

	// 4. 컴파일 시 임베드된 PE 리소스 아이콘 (goversioninfo 기본 ID 1)
	hInst, _, _ := procGetModuleHandleW.Call(0)
	hIcon, _, _ := procLoadIconW.Call(hInst, uintptr(1))
	if hIcon != 0 {
		return syscall.Handle(hIcon)
	}

	// 5. 시스템 기본 어플리케이션 아이콘 (IDI_APPLICATION = 32512)
	h, _, _ := procLoadIconW.Call(0, uintptr(32512))
	return syscall.Handle(h)
}

func (t *PureWinTrayIcon) showMenu() {
	hmenu, _, _ := procCreatePopupMenu.Call()
	if hmenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hmenu)

	cmdMap := make(map[uint32]func())
	var cmdID uint32 = 1000

	var appendMenuItems func(hTarget uintptr, items []MenuItem)
	appendMenuItems = func(hTarget uintptr, items []MenuItem) {
		for _, item := range items {
			if item.IsSeparator {
				procAppendMenuW.Call(hTarget, uintptr(MF_SEPARATOR), 0, 0)
				continue
			}

			label := item.Label
			if item.DynamicLabel != nil {
				label = item.DynamicLabel()
			}
			labelPtr, _ := syscall.UTF16PtrFromString(label)

			if len(item.SubItems) > 0 {
				hSub, _, _ := procCreatePopupMenu.Call()
				if hSub != 0 {
					appendMenuItems(hSub, item.SubItems)
					procAppendMenuW.Call(hTarget, uintptr(MF_POPUP), hSub, uintptr(unsafe.Pointer(labelPtr)))
				}
			} else {
				var flags uint32 = MF_STRING
				if item.CheckFn != nil && item.CheckFn() {
					flags |= MF_CHECKED
				}
				if item.DisabledFn != nil && item.DisabledFn() {
					flags |= MF_GRAYED | MF_DISABLED
				}
				procAppendMenuW.Call(hTarget, uintptr(flags), uintptr(cmdID), uintptr(unsafe.Pointer(labelPtr)))
				if item.Callback != nil {
					cmdMap[cmdID] = item.Callback
				}
				cmdID++
			}
		}
	}

	appendMenuItems(hmenu, t.MenuItems)

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(uintptr(t.Hwnd))

	chosen, _, _ := procTrackPopupMenu.Call(
		hmenu,
		uintptr(TPM_RIGHTBUTTON|TPM_RETURNCMD),
		uintptr(pt.X),
		uintptr(pt.Y),
		0,
		uintptr(t.Hwnd),
		0,
	)

	procPostMessageW.Call(uintptr(t.Hwnd), 0, 0, 0) // WM_NULL

	if cb, ok := cmdMap[uint32(chosen)]; ok && cb != nil {
		cb()
	}
}

func (t *PureWinTrayIcon) sendNotification(title, msg string, dwInfoFlags uint32) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Nid == nil {
		return
	}

	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = t.Hwnd
	nid.UID = 1
	nid.UFlags = NIF_INFO
	nid.DwInfoFlags = dwInfoFlags

	titleUTF16, _ := syscall.UTF16FromString(title)
	msgUTF16, _ := syscall.UTF16FromString(msg)
	copy(nid.SzInfoTitle[:], titleUTF16)
	copy(nid.SzInfo[:], msgUTF16)

	procShell_NotifyIconW.Call(uintptr(NIM_MODIFY), uintptr(unsafe.Pointer(&nid)))
}

// ShowNotification: 트레이 풍선 알림 표시 (정보)
func (t *PureWinTrayIcon) ShowNotification(title, msg string) {
	LogInfo("[Tray Notification] %s: %s", title, msg)
	t.sendNotification(title, msg, NIIF_INFO)
}

// ShowErrorNotification: 트레이 풍선 알림 표시 (경고/에러)
func (t *PureWinTrayIcon) ShowErrorNotification(title, msg string) {
	LogWarn("[Tray Notification] %s: %s", title, msg)
	t.sendNotification(title, msg, NIIF_ERROR)
}

// UpdateTooltip: 트레이 아이콘의 마우스 호버 툴팁을 동적으로 변경
func (t *PureWinTrayIcon) UpdateTooltip(tip string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Nid == nil {
		return
	}
	t.Tooltip = tip
	tipUTF16, err := syscall.UTF16FromString(tip)
	if err != nil {
		return
	}
	var nid NOTIFYICONDATAW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = t.Hwnd
	nid.UID = 1
	nid.UFlags = NIF_TIP
	copy(nid.SzTip[:], tipUTF16)

	procShell_NotifyIconW.Call(uintptr(NIM_MODIFY), uintptr(unsafe.Pointer(&nid)))
}



// SetClipboardText: [SYS-501] Win32 네이티브 API(OpenClipboard / SetClipboardData)를 사용하여
// cmd.exe 및 clip.exe 호출 없이 즉각적이고 안전하게 텍스트를 클립보드에 복사합니다.
func SetClipboardText(text string) error {
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	size := uintptr(len(utf16) * 2)

	hMem, _, err := procGlobalAlloc.Call(uintptr(GMEM_MOVEABLE), size)
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc failed: %v", err)
	}

	ptr, _, err := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("GlobalLock failed: %v", err)
	}

	dest := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16))
	copy(dest, utf16)

	procGlobalUnlock.Call(hMem)

	// 타 프로세스의 클립보드 점유 가능성을 고려한 재시도 루프
	var opened bool
	for i := 0; i < 10; i++ {
		r, _, _ := procOpenClipboard.Call(0)
		if r != 0 {
			opened = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !opened {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("OpenClipboard failed")
	}
	defer procCloseClipboard.Call()

	r, _, err := procEmptyClipboard.Call()
	if r == 0 {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("EmptyClipboard failed: %v", err)
	}

	r, _, err = procSetClipboardData.Call(uintptr(CF_UNICODETEXT), hMem)
	if r == 0 {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("SetClipboardData failed: %v", err)
	}

	return nil
}

// CopyDockUrl: 독 URL을 클립보드에 복사하고 알림 표시
func CopyDockUrl(port int) {
	dockURL := fmt.Sprintf("http://localhost:%d", port)
	if err := SetClipboardText(dockURL); err != nil {
		fmt.Printf("[Clipboard Error] %v\n", err)
	} else if GlobalTray != nil {
		GlobalTray.ShowNotification("CHZZK OBS Dock", fmt.Sprintf("독 URL이 복사되었습니다: %s", dockURL))
	}
}

// Run: Win32 윈도우 생성 및 트레이 메시지 루프 실행
func (t *PureWinTrayIcon) Run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	GlobalTray = t

	wndProcCallback := syscall.NewCallback(func(hwnd syscall.Handle, msg uint32, wparam, lparam uintptr) uintptr {
		if msg == WM_TRAYICON {
			switch lparam {
			case 0x0203: // WM_LBUTTONDBLCLK: 트레이 아이콘 더블클릭 시 독 화면 열기
				ShowDockWindow()
				return 0
			case 0x0205, 0x007B: // WM_RBUTTONUP (0x0205), WM_CONTEXTMENU (0x007B): 우클릭 시 메뉴 열기
				t.showMenu()
				return 0
			}
		} else if msg == 0x0011 { // WM_QUERYENDSESSION: Windows 정상 종료/재부팅 질의 시 정상 승인(1)
			return 1
		} else if msg == 0x0010 || (msg == 0x0016 && wparam != 0) { // WM_CLOSE, WM_ENDSESSION: 인스톨러 및 윈도우 종료 신호 시 즉시 완전 종료
			if OnShutdownCallback != nil {
				OnShutdownCallback()
			}
			os.Exit(0)
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
		return r
	})

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	classNamePtr, _ := syscall.UTF16PtrFromString("ChzzkPureTrayClass")
	windowNamePtr, _ := syscall.UTF16PtrFromString("ChzzkTrayWindow")

	wc := WNDCLASSW{
		LpfnWndProc:   wndProcCallback,
		HInstance:     syscall.Handle(hInstance),
		LpszClassName: classNamePtr,
	}

	procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(classNamePtr)),
		uintptr(unsafe.Pointer(windowNamePtr)),
		0, 0, 0, 0, 0, 0, 0,
		hInstance,
		0,
	)
	t.Hwnd = syscall.Handle(hwnd)
	t.Hicon = LoadAppIcon()

	nid := NOTIFYICONDATAW{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATAW{})),
		HWnd:             t.Hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_TRAYICON,
		HIcon:            t.Hicon,
	}

	tipUTF16, _ := syscall.UTF16FromString(t.Tooltip)
	copy(nid.SzTip[:], tipUTF16)

	t.Nid = &nid
	procShell_NotifyIconW.Call(uintptr(NIM_ADD), uintptr(unsafe.Pointer(t.Nid)))

	// 서버 실행 알림 (Windows 알림 토스트/풍선)
	if t.StartNotificationTitle != "" && t.StartNotificationMsg != "" {
		go func() {
			time.Sleep(300 * time.Millisecond)
			t.ShowNotification(t.StartNotificationTitle, t.StartNotificationMsg)
		}()
	}

	var msg MSG
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Stop: 트레이 아이콘 삭제 및 윈도우 파괴
func (t *PureWinTrayIcon) Stop() {
	if t.Nid != nil {
		procShell_NotifyIconW.Call(uintptr(NIM_DELETE), uintptr(unsafe.Pointer(t.Nid)))
		t.Nid = nil
	}
	if t.Hwnd != 0 {
		procDestroyWindow.Call(uintptr(t.Hwnd))
		t.Hwnd = 0
	}
}
