package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassW     = user32.NewProc("RegisterClassW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procLoadImageW         = user32.NewProc("LoadImageW")
	procLoadIconW          = user32.NewProc("LoadIconW")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procAppendMenuW        = user32.NewProc("AppendMenuW")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procPostMessageW       = user32.NewProc("PostMessageW")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procShell_NotifyIconW  = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
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
	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x00000010
	LR_DEFAULTSIZE  = 0x00000040
	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100
	MF_STRING       = 0x0000
	MF_SEPARATOR    = 0x0800
	MF_CHECKED      = 0x0008

	APP_NAME = "ChzzkObsDockServer"
	REG_PATH = `Software\Microsoft\Windows\CurrentVersion\Run`
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
	Label       string
	Callback    func()
	CheckFn     func() bool
	IsSeparator bool
}

type PureWinTrayIcon struct {
	Tooltip                string
	IconPath               string
	IconBytes              []byte
	Hwnd                   syscall.Handle
	Hicon                  syscall.Handle
	Nid                    *NOTIFYICONDATAW
	MenuItems              []MenuItem
	StartNotificationTitle string
	StartNotificationMsg   string
	mu                     sync.Mutex
}

var GlobalTray *PureWinTrayIcon

// IsStartupEnabled: 시작 프로그램 등록 여부 확인
func IsStartupEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, REG_PATH, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()

	_, _, err = k.GetStringValue(APP_NAME)
	return err == nil
}

// ToggleStartup: 시작 프로그램 등록/해제 토글
func ToggleStartup(enable bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, REG_PATH, registry.SET_VALUE)
	if err != nil {
		fmt.Printf("[Startup Reg Error] OpenKey: %v\n", err)
		return
	}
	defer k.Close()

	if enable {
		exePath, err := os.Executable()
		if err != nil {
			fmt.Printf("[Startup Reg Error] GetExecutable: %v\n", err)
			return
		}
		quotedPath := fmt.Sprintf(`"%s"`, exePath)
		err = k.SetStringValue(APP_NAME, quotedPath)
		if err != nil {
			fmt.Printf("[Startup Reg Error] SetStringValue: %v\n", err)
		}
	} else {
		_ = k.DeleteValue(APP_NAME)
	}
}

// NewPureWinTrayIcon: 트레이 아이콘 인스턴스 생성
func NewPureWinTrayIcon(tooltip, iconPath string, iconBytes []byte) *PureWinTrayIcon {
	return &PureWinTrayIcon{
		Tooltip:   tooltip,
		IconPath:  iconPath,
		IconBytes: iconBytes,
		MenuItems: make([]MenuItem, 0),
	}
}

func (t *PureWinTrayIcon) createIcon() syscall.Handle {
	if t.IconPath != "" {
		if _, err := os.Stat(t.IconPath); err == nil {
			pathPtr, err := syscall.UTF16PtrFromString(t.IconPath)
			if err == nil {
				hicon, _, _ := procLoadImageW.Call(
					0,
					uintptr(unsafe.Pointer(pathPtr)),
					uintptr(IMAGE_ICON),
					0,
					0,
					uintptr(LR_LOADFROMFILE|LR_DEFAULTSIZE),
				)
				if hicon != 0 {
					return syscall.Handle(hicon)
				}
			}
		}
	}

	// 임베디드 아이콘 바이트가 있는 경우 임시 파일로 작성 후 로드
	if len(t.IconBytes) > 0 {
		tempIconPath := filepath.Join(os.TempDir(), "chzzk_dock_temp_icon.ico")
		if err := os.WriteFile(tempIconPath, t.IconBytes, 0644); err == nil {
			pathPtr, err := syscall.UTF16PtrFromString(tempIconPath)
			if err == nil {
				hicon, _, _ := procLoadImageW.Call(
					0,
					uintptr(unsafe.Pointer(pathPtr)),
					uintptr(IMAGE_ICON),
					0,
					0,
					uintptr(LR_LOADFROMFILE|LR_DEFAULTSIZE),
				)
				if hicon != 0 {
					return syscall.Handle(hicon)
				}
			}
		}
	}

	// 시스템 기본 어플리케이션 아이콘 (IDI_APPLICATION = 32512)
	hicon, _, _ := procLoadIconW.Call(0, uintptr(32512))
	return syscall.Handle(hicon)
}

func (t *PureWinTrayIcon) showMenu() {
	hmenu, _, _ := procCreatePopupMenu.Call()
	if hmenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hmenu)

	cmdMap := make(map[uint32]func())
	var cmdID uint32 = 1000

	for _, item := range t.MenuItems {
		if item.IsSeparator {
			procAppendMenuW.Call(hmenu, uintptr(MF_SEPARATOR), 0, 0)
		} else {
			var flags uint32 = MF_STRING
			if item.CheckFn != nil && item.CheckFn() {
				flags |= MF_CHECKED
			}
			labelPtr, _ := syscall.UTF16PtrFromString(item.Label)
			procAppendMenuW.Call(hmenu, uintptr(flags), uintptr(cmdID), uintptr(unsafe.Pointer(labelPtr)))
			if item.Callback != nil {
				cmdMap[cmdID] = item.Callback
			}
			cmdID++
		}
	}

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

// ShowNotification: 트레이 풍선 알림 표시
func (t *PureWinTrayIcon) ShowNotification(title, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.Nid == nil {
		return
	}

	t.Nid.UFlags |= NIF_INFO

	titleUTF16, _ := syscall.UTF16FromString(title)
	msgUTF16, _ := syscall.UTF16FromString(msg)

	copy(t.Nid.SzInfoTitle[:], titleUTF16)
	copy(t.Nid.SzInfo[:], msgUTF16)
	t.Nid.DwInfoFlags = NIIF_INFO

	procShell_NotifyIconW.Call(uintptr(NIM_MODIFY), uintptr(unsafe.Pointer(t.Nid)))
}

// CopyDockUrl: 독 URL을 클립보드에 복사하고 알림 표시
func CopyDockUrl(port int) {
	dockURL := fmt.Sprintf("http://localhost:%d", port)
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("echo|set /p=\"%s\"|clip", dockURL))
	if err := cmd.Run(); err != nil {
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
			// WM_RBUTTONUP (0x0205), WM_RBUTTONDOWN (0x0204), WM_LBUTTONUP (0x0202), WM_CONTEXTMENU (0x007B)
			if lparam == 0x0205 || lparam == 0x0204 || lparam == 0x0202 || lparam == 0x007B {
				t.showMenu()
				return 0
			}
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
	t.Hicon = t.createIcon()

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
