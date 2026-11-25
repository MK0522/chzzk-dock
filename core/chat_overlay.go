package core

import (
	_ "embed"
	"fmt"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
)

//go:embed chat_overlay.js
var chatOverlayScript string

const (
	overlayClassName           = "ChzzkChatOverlayWindowClass"
	overlayPopupStyle          = 0x80000000 | 0x02000000 | 0x04000000
	overlayActivateMessage     = 0x8000 + 71 // WM_APP
	overlayDragMessage         = 0x8000 + 72
	overlayHotkeyID            = 51
	overlaySetModeMessage      = 0x8000 + 73
	overlayTrayCycleMessage    = 0x8000 + 74
	overlayTrayModeMessage     = 0x8000 + 75
	overlayFailureMessage      = 0x8000 + 76
	overlayOrderMessage        = 0x8000 + 77
	overlayAppearanceMessage   = 0x8000 + 78
	overlayTraySetModeMessage  = 0x8000 + 79
	overlaySurfaceReadyMessage = 0x8000 + 80
	overlayDonationMessage     = 0x8000 + 83
	overlayColorMessage        = 0x8000 + 81
	overlayShortcutMessage     = 0x8000 + 82
	overlayRestartMessage      = 0x8000 + 84
	overlayScaleMessage        = 0x8000 + 85
	overlayLayered             = 0x00080000
	overlayToolWindow          = 0x00000080
	overlayTransparent         = 0x00000020
	overlayNoActivate          = 0x08000000
	overlayExStyle             = ^uintptr(19) // GWL_EXSTYLE (-20)
)

type overlayMode uint8

const (
	overlayPass overlayMode = iota
	overlayArrange
	overlayInteract
)

var (
	registerOverlayHotkey = user32.NewProc("RegisterHotKey")
	getOverlayStyle       = user32.NewProc("GetWindowLongW")
	setOverlayStyle       = user32.NewProc("SetWindowLongW")
	setOverlayAlpha       = user32.NewProc("SetLayeredWindowAttributes")
	getOverlayCursor      = user32.NewProc("GetCursorPos")
	setOverlayCapture     = user32.NewProc("SetCapture")
	releaseOverlayCapture = user32.NewProc("ReleaseCapture")
	getOverlayAsyncKey    = user32.NewProc("GetAsyncKeyState")
	loadOverlayCursor     = user32.NewProc("LoadCursorW")
	setOverlayCursor      = user32.NewProc("SetCursor")
	extendOverlayFrame    = dwmapiDLL.NewProc("DwmExtendFrameIntoClientArea")
	getOverlayStockObject = gdi32DLL.NewProc("GetStockObject")
	redrawOverlayWindow   = user32.NewProc("RedrawWindow")
)

type chatOverlayWindow struct {
	hwnd                      uintptr
	chromium                  *edge.Chromium
	state                     *ToolWindowState
	mode                      overlayMode
	previousForeground        uintptr
	reloaded                  bool
	restart                   bool
	target                    string
	resizeDrag                *overlayResizeDrag
	lastViewport              edge.Rect
	lastCrop                  overlayCrop
	windowOpacity             int
	donationBackgroundOpacity int
	backgroundColor           uint32
	backgroundOpacity         int
	chatScale                 int
	stateFileName             string
	surfaceReady              bool
}

func ActivateChatOverlay() bool {
	className, _ := syscall.UTF16PtrFromString(overlayClassName)
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(className)), 0)
	if hwnd == 0 {
		return false
	}
	ok, _, _ := procPostMessageW.Call(hwnd, overlayActivateMessage, 0, 0)
	return ok != 0
}

func overlayNeedsLayered(mode overlayMode, opacity int) bool {
	return mode == overlayPass || opacity < 100
}

func overlayModeExStyle(base uintptr, mode overlayMode, opacity int) uintptr {
	// Hide overlay windows from Alt+Tab and the taskbar across all modes.
	style := (base &^ (overlayLayered | overlayTransparent | overlayNoActivate | WS_EX_APPWINDOW_VAL)) | overlayToolWindow
	if mode == overlayPass {
		style |= overlayTransparent | overlayNoActivate
	}
	// Only enable WS_EX_LAYERED when click-through or window opacity needs it.
	// Keeping edit mode on pure DWM DirectComposition at 100% opacity avoids
	// layered redirection surface reallocations (white flashes) during resize.
	if overlayNeedsLayered(mode, opacity) {
		style |= overlayLayered
	}
	return style
}

func (o *chatOverlayWindow) extendFrame() {
	margins := [4]int32{-1, -1, -1, -1}
	result, _, _ := extendOverlayFrame.Call(o.hwnd, uintptr(unsafe.Pointer(&margins)))
	if int32(result) < 0 {
		LogWarn("[Chat Overlay] DWM transparency unavailable: 0x%08x", uint32(result))
	}
}

func (o *chatOverlayWindow) initialize(chromium *edge.Chromium) {
	o.chromium = chromium
	chromium.SetBackgroundColour(0, 0, 0, 0)
	o.resizeWebView()
	o.extendFrame()
	mode := overlayMode(CurrentOverlayMode())
	if loaded, err := LoadOverlayMode(); err == nil && loaded <= uint32(overlayInteract) {
		mode = overlayMode(loaded)
	}
	if tray := overlayTrayWindow(); tray != 0 {
		var trayMode uintptr
		if ok, _, _ := user32.NewProc("SendMessageTimeoutW").Call(tray, overlayTrayModeMessage, 0, 0, 0x0002 /* SMTO_ABORTIFHUNG */, 500, uintptr(unsafe.Pointer(&trayMode))); ok != 0 && trayMode <= uintptr(overlayInteract) {
			mode = overlayMode(trayMode)
		}
	}
	o.setMode(mode)
}

func (o *chatOverlayWindow) setMode(mode overlayMode) {
	if o.resizeDrag != nil && mode != overlayArrange {
		o.finishResizeDrag(false)
	}
	if o.mode == overlayPass && mode != overlayPass {
		foreground, _, _ := procGetForegroundWindow.Call()
		if foreground != o.hwnd {
			o.previousForeground = foreground
		}
	}
	style, _, _ := getOverlayStyle.Call(o.hwnd, overlayExStyle)
	opacity := o.windowOpacity
	style = overlayModeExStyle(style, mode, opacity)
	setOverlayStyle.Call(o.hwnd, overlayExStyle, style)
	if style&overlayLayered != 0 {
		alpha := uintptr((opacity*255 + 50) / 100)
		if ok, _, err := setOverlayAlpha.Call(o.hwnd, 0, alpha, 0x02 /* LWA_ALPHA */); ok == 0 {
			LogWarn("[Chat Overlay] opacity setup failed: %v", err)
			if mode == overlayPass {
				mode = overlayInteract
			}
			fallbackStyle := overlayModeExStyle(style, mode, 100) &^ overlayLayered
			setOverlayStyle.Call(o.hwnd, overlayExStyle, fallbackStyle)
		}
	}
	o.mode = mode
	if mode == overlayPass && o.previousForeground != 0 {
		foreground, _, _ := procGetForegroundWindow.Call()
		if foreground == o.hwnd {
			procSetForegroundWindow.Call(o.previousForeground)
		}
	}
	procSetWindowPos.Call(o.hwnd, 0, 0, 0, 0, 0, 0x0037 /* FRAMECHANGED | NOACTIVATE | NOZORDER | NOMOVE | NOSIZE */)
	o.extendFrame()
	borderColor := uint32(0xfffffffe) // DWMWA_COLOR_NONE
	cornerPreference := uint32(1)     // DWMWCP_DONOTROUND
	procDwmSetWindowAttribute.Call(o.hwnd, 34, uintptr(unsafe.Pointer(&borderColor)), 4)
	procDwmSetWindowAttribute.Call(o.hwnd, 33, uintptr(unsafe.Pointer(&cornerPreference)), 4)
	o.updatePage()
}

func (o *chatOverlayWindow) setBackgroundOpacity(percent int) {
	o.backgroundOpacity = percent
}

func (o *chatOverlayWindow) pageStateArguments() string {
	crop := o.state.OverlayCrop.normalized()
	background := 0
	if o.mode != overlayInteract {
		background = o.backgroundOpacity
	}
	scale := o.chatScale
	if scale < 100 || scale > 170 {
		scale = 100
	}
	return fmt.Sprintf(`%d, %d, [%d, %d, %d, %d], "#%06x", %d, %d`, o.mode, background, crop.Left, crop.Top, crop.Right, crop.Bottom, o.backgroundColor, o.donationBackgroundOpacity, scale)
}

func (o *chatOverlayWindow) persistBounds() {
	if o.state == nil || o.stateFileName == "" || o.hwnd == 0 {
		return
	}
	var rect RECT
	if ok, _, _ := procGetWindowRect.Call(o.hwnd, uintptr(unsafe.Pointer(&rect))); ok != 0 {
		w := int(rect.Right - rect.Left)
		h := int(rect.Bottom - rect.Top)
		if w >= minOverlayWidth && h >= minOverlayHeight {
			o.state.X = int(rect.Left)
			o.state.Y = int(rect.Top)
			o.state.Width = w
			o.state.Height = h
		}
	}
	saveToolWindowState(o.stateFileName, *o.state)
}

func (o *chatOverlayWindow) updatePage() {
	if o.chromium != nil {
		o.chromium.Eval("window.chzzkDockOverlay?.update(" + o.pageStateArguments() + ")")
	}
}

func (o *chatOverlayWindow) handleMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) (bool, uintptr) {
	switch msg {
	case 0x007E: // WM_DISPLAYCHANGE
		setOverlayMonitorTimer.Call(hwnd, overlayMonitorTimer, 300, 0)
	case 0x0113: // WM_TIMER
		if wParam == overlayMonitorTimer {
			killOverlayMonitorTimer.Call(hwnd, overlayMonitorTimer)
			o.restoreMonitorPosition()
			return true, 0
		}
	case 0x0232: // WM_EXITSIZEMOVE: a manual move replaces the preferred position.
		o.rememberMonitorPosition()
		o.persistBounds()
	case 0x0083, 0x0085: // WM_NCCALCSIZE, WM_NCPAINT: no native frame.
		return true, 0
	case 0x0086: // WM_NCACTIVATE: skip the border repaint on focus changes.
		result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, ^uintptr(0))
		return true, result
	case 0x0024: // WM_GETMINMAXINFO
		limits := (*MINMAXINFO)(unsafe.Pointer(lParam))
		limits.PtMinTrackSize = POINT{X: minOverlayWidth, Y: minOverlayHeight}
		return true, 0
	case 0x0020: // WM_SETCURSOR
		if o.resizeDrag != nil {
			cursor, _, _ := loadOverlayCursor.Call(0, overlayResizeCursor(o.resizeDrag.hit))
			if cursor != 0 {
				setOverlayCursor.Call(cursor)
			}
			return true, 1
		}
	case 0x0200: // WM_MOUSEMOVE
		if o.resizeDrag != nil {
			o.updateResizeDrag()
			return true, 0
		}
	case 0x0202, 0x0215: // WM_LBUTTONUP, WM_CAPTURECHANGED
		if o.resizeDrag != nil {
			o.finishResizeDrag(false)
			return true, 0
		}
	case 0x0100: // WM_KEYDOWN
		if o.resizeDrag != nil && wParam == 0x1B /* VK_ESCAPE */ {
			o.finishResizeDrag(true)
			return true, 0
		}
	case overlayDonationMessage:
		if wParam <= 100 {
			o.donationBackgroundOpacity = int(wParam)
			o.updatePage()
		}
		return true, 0
	case overlayColorMessage:
		o.backgroundColor = uint32(wParam)
		o.updatePage()
		return true, 0
	case overlayScaleMessage:
		if wParam >= 100 && wParam <= 170 {
			o.chatScale = int(wParam)
			o.updatePage()
		}
		return true, 0
	case overlayAppearanceMessage:
		opacityChanged := wParam <= 100 && o.windowOpacity != int(wParam)
		if opacityChanged {
			o.windowOpacity = int(wParam)
		}
		if lParam <= 100 {
			o.setBackgroundOpacity(int(lParam))
		}
		if opacityChanged {
			o.setMode(o.mode)
		} else {
			o.updatePage()
		}
		return true, 0
	case overlaySetModeMessage:
		if wParam <= uintptr(overlayInteract) {
			o.setMode(overlayMode(wParam))
		}
		return true, 0
	case overlaySurfaceReadyMessage:
		// Refresh the layered surface after WebView2 has drawn its first frame.
		o.setMode(o.mode)
		o.lastViewport = edge.Rect{}
		o.resizeWebView()
		_ = o.chromium.NotifyParentWindowPositionChanged()
		redrawOverlayWindow.Call(hwnd, 0, 0, 0x0181 /* INVALIDATE | ALLCHILDREN | UPDATENOW */)
		return true, 0
	case overlayActivateMessage:
		procShowWindow.Call(hwnd, 4 /* SW_SHOWNOACTIVATE */)
		return true, 0
	case overlayFailureMessage:
		if wParam == 1 && !o.reloaded {
			o.reloaded = true
			o.chromium.Navigate(o.target)
		} else {
			o.restart = true
			procPostMessageW.Call(hwnd, WM_CLOSE_VAL, 0, 0)
		}
		return true, 0
	case overlayRestartMessage:
		o.restart = true
		procPostMessageW.Call(hwnd, WM_CLOSE_VAL, 0, 0)
		return true, 0
	case overlayDragMessage:
		if o.mode == overlayArrange {
			o.beginDrag(wParam != 0)
		}
		return true, 0
	}
	return false, 0
}

func (o *chatOverlayWindow) handleWebMessage(message string) {
	switch message {
	case "chat-overlay:surface-ready":
		if !o.surfaceReady {
			o.surfaceReady = true
			procPostMessageW.Call(o.hwnd, overlaySurfaceReadyMessage, 0, 0)
		}
	case "chat-overlay:drag", "chat-overlay:drag-crop":
		if o.mode == overlayArrange {
			flag := uintptr(0)
			if message == "chat-overlay:drag-crop" {
				flag = 1
			}
			procPostMessageW.Call(o.hwnd, overlayDragMessage, flag, 0)
		}
	}
}

func (o *chatOverlayWindow) beginDrag(cropRequested bool) {
	var pt POINT
	var bounds RECT
	gotCursor, _, _ := getOverlayCursor.Call(uintptr(unsafe.Pointer(&pt)))
	gotBounds, _, _ := procGetWindowRect.Call(o.hwnd, uintptr(unsafe.Pointer(&bounds)))
	if gotCursor == 0 || gotBounds == 0 {
		return
	}

	const edge = 10
	hit := uintptr(2) // HTCAPTION: drag the whole window.
	switch {
	case pt.X < bounds.Left+edge:
		hit = 10 // HTLEFT
		if pt.Y < bounds.Top+edge {
			hit = 13 // HTTOPLEFT
		} else if pt.Y >= bounds.Bottom-edge {
			hit = 16 // HTBOTTOMLEFT
		}
	case pt.X >= bounds.Right-edge:
		hit = 11 // HTRIGHT
		if pt.Y < bounds.Top+edge {
			hit = 14 // HTTOPRIGHT
		} else if pt.Y >= bounds.Bottom-edge {
			hit = 17 // HTBOTTOMRIGHT
		}
	case pt.Y < bounds.Top+edge:
		hit = 12 // HTTOP
	case pt.Y >= bounds.Bottom-edge:
		hit = 15 // HTBOTTOM
	}
	if hit == 2 {
		releaseOverlayCapture.Call()
		position := uintptr(uint16(pt.X)) | uintptr(uint16(pt.Y))<<16
		procSendMessageW.Call(o.hwnd, 0x00A1 /* WM_NCLBUTTONDOWN */, hit, position)
		return
	}
	// Resize directly via pointer capture instead of toggling WS_THICKFRAME +
	// WM_NCLBUTTONDOWN, which triggers DWM's framed resize surface and causes a
	// white background on transparent WebView2 overlays.
	lbtn, _, _ := getOverlayAsyncKey.Call(0x01 /* VK_LBUTTON */)
	if int16(lbtn) >= 0 {
		return
	}
	drag := &overlayResizeDrag{
		hit:    hit,
		origin: pt,
		start:  bounds,
		last:   bounds,
	}
	if o.state.OverlayCrop != nil {
		orig := *o.state.OverlayCrop
		drag.origCrop = &orig
	}
	alt, _, _ := getOverlayKeyState.Call(0x12 /* VK_MENU */)
	if cropRequested || int16(alt) < 0 {
		drag.crop = &overlayCropDrag{bounds: bounds, crop: o.state.OverlayCrop.normalized()}
	}
	o.resizeDrag = drag
	releaseOverlayCapture.Call()
	setOverlayCapture.Call(o.hwnd)
	if cursor, _, _ := loadOverlayCursor.Call(0, overlayResizeCursor(hit)); cursor != 0 {
		setOverlayCursor.Call(cursor)
	}
}

func (o *chatOverlayWindow) updateResizeDrag() {
	drag := o.resizeDrag
	if drag == nil {
		return
	}
	lbtn, _, _ := getOverlayAsyncKey.Call(0x01 /* VK_LBUTTON */)
	if int16(lbtn) >= 0 {
		o.finishResizeDrag(false)
		return
	}
	var pt POINT
	if ok, _, _ := getOverlayCursor.Call(uintptr(unsafe.Pointer(&pt))); ok == 0 {
		return
	}
	proposed := overlayResizeRect(drag.start, drag.origin, pt, drag.hit)
	if drag.crop != nil {
		crop := drag.crop.sizing(overlayResizeEdge(drag.hit), &proposed)
		o.state.OverlayCrop = &crop
	}
	if proposed == drag.last {
		return
	}
	originMoved := proposed.Left != drag.last.Left || proposed.Top != drag.last.Top
	drag.last = proposed
	procSetWindowPos.Call(
		o.hwnd, 0,
		uintptr(proposed.Left), uintptr(proposed.Top),
		uintptr(proposed.Right-proposed.Left), uintptr(proposed.Bottom-proposed.Top),
		0x0014, // SWP_NOZORDER | SWP_NOACTIVATE
	)
	if originMoved && o.chromium != nil {
		_ = o.chromium.NotifyParentWindowPositionChanged()
	}
}

func (o *chatOverlayWindow) finishResizeDrag(cancel bool) {
	drag := o.resizeDrag
	if drag == nil {
		return
	}
	o.resizeDrag = nil
	releaseOverlayCapture.Call()
	if cancel {
		o.state.OverlayCrop = drag.origCrop
		procSetWindowPos.Call(
			o.hwnd, 0,
			uintptr(drag.start.Left), uintptr(drag.start.Top),
			uintptr(drag.start.Right-drag.start.Left), uintptr(drag.start.Bottom-drag.start.Top),
			0x0014,
		)
		if o.chromium != nil {
			o.resizeWebView()
			_ = o.chromium.NotifyParentWindowPositionChanged()
		}
		o.updatePage()
		return
	}
	o.rememberMonitorPosition()
	o.persistBounds()
	if overlayNeedsLayered(o.mode, o.windowOpacity) {
		redrawOverlayWindow.Call(o.hwnd, 0, 0, 0x0181 /* INVALIDATE | ALLCHILDREN | UPDATENOW */)
	}
}

var displayedOverlayMode atomic.Uint32
var overlayHotkeyRegistered atomic.Bool

func InitializeOverlayMode() {
	mode, err := LoadOverlayMode()
	if err != nil {
		LogWarn("[Chat Overlay] mode restore: %v", err)
		return
	}
	displayedOverlayMode.Store(mode)
}

func overlayTrayWindow() uintptr {
	class, _ := syscall.UTF16PtrFromString("ChzzkPureTrayClass")
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), 0)
	return hwnd
}

func CurrentOverlayMode() uint32 { return displayedOverlayMode.Load() }

func OverlayHotkeyAvailable() bool { return overlayHotkeyRegistered.Load() }

func RequestOverlayCycle() {
	if hwnd := overlayTrayWindow(); hwnd != 0 {
		procPostMessageW.Call(hwnd, overlayTrayCycleMessage, 0, 0)
	}
}

func RequestOverlayMode(mode uint32) bool {
	if mode > uint32(overlayInteract) {
		return false
	}
	hwnd := overlayTrayWindow()
	if hwnd == 0 {
		return false
	}
	ok, _, _ := procPostMessageW.Call(hwnd, overlayTraySetModeMessage, uintptr(mode), 0)
	return ok != 0
}

func registerOverlayTrayShortcut(hwnd uintptr) {
	user32.NewProc("UnregisterHotKey").Call(hwnd, overlayHotkeyID)
	shortcut, _ := LoadOverlayShortcut()
	overlayHotkeyRegistered.Store(false)
	if shortcut.Key == 0 {
		return
	}
	ok, _, err := registerOverlayHotkey.Call(hwnd, overlayHotkeyID, uintptr(shortcut.Modifiers|0x4000), uintptr(shortcut.Key))
	overlayHotkeyRegistered.Store(ok != 0)
	if ok == 0 {
		LogWarn("[Chat Overlay] shortcut registration failed: %v", err)
	}
}

func handleOverlayTrayMessage(msg uint32, key uintptr) (bool, uintptr) {
	if msg == overlayShortcutMessage {
		registerOverlayTrayShortcut(overlayTrayWindow())
		return true, 0
	}
	if msg == overlayTrayModeMessage {
		return true, uintptr(CurrentOverlayMode())
	}
	if msg == overlayOrderMessage {
		if layers, err := LoadOverlayLayers(); err == nil {
			OrderOverlayLayers(layers)
		}
		return true, 0
	}
	if msg != overlayTraySetModeMessage && msg != overlayTrayCycleMessage && (msg != 0x0312 || key != overlayHotkeyID) {
		return false, 0
	}
	layers, err := LoadOverlayLayers()
	if err != nil {
		LogWarn("[Chat Overlay] mode change: %v", err)
		return true, 0
	}
	mode := (CurrentOverlayMode() + 1) % 3
	if msg == overlayTraySetModeMessage {
		if key > uintptr(overlayInteract) {
			return true, 0
		}
		mode = uint32(key)
	}
	displayedOverlayMode.Store(mode)
	if err := SaveOverlayMode(mode); err != nil {
		LogWarn("[Chat Overlay] mode save: %v", err)
	}
	for _, layer := range layers {
		if hwnd := overlayWindow(layer.ID); hwnd != 0 {
			procPostMessageW.Call(hwnd, overlaySetModeMessage, uintptr(mode), 0)
		}
	}
	return true, 0
}

func (o *chatOverlayWindow) processFailed(_ *edge.ICoreWebView2, args *edge.ICoreWebView2ProcessFailedEventArgs) {
	kind, err := args.GetProcessFailedKind()
	if err != nil {
		return
	}
	switch kind {
	case edge.COREWEBVIEW2_PROCESS_FAILED_KIND_BROWSER_PROCESS_EXITED,
		edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_EXITED,
		edge.COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_UNRESPONSIVE:
		LogWarn("[Chat Overlay] WebView2 process failure: %d", kind)
		procPostMessageW.Call(o.hwnd, overlayFailureMessage, uintptr(kind), 0)
	}
}
