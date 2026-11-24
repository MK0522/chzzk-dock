package core

import (
	"strings"
	"syscall"
	"unsafe"
)

const overlayMonitorTimer = 0x4344

var (
	getOverlayMonitor         = user32.NewProc("MonitorFromWindow")
	enumOverlayMonitors       = user32.NewProc("EnumDisplayMonitors")
	enumOverlayDisplayDevices = user32.NewProc("EnumDisplayDevicesW")
	setOverlayMonitorTimer    = user32.NewProc("SetTimer")
	killOverlayMonitorTimer   = user32.NewProc("KillTimer")
)

type overlayMonitorHome struct {
	Device string
	X, Y   int32
}

type overlayMonitorInfo struct {
	MONITORINFO
	Device [32]uint16
}

type overlayDisplayDevice struct {
	Size  uint32
	Name  [32]uint16
	Label [128]uint16
	Flags uint32
	ID    [128]uint16
	Key   [128]uint16
}

func overlayMonitorWork(monitor uintptr) (RECT, string, bool) {
	var info overlayMonitorInfo
	info.CbSize = uint32(unsafe.Sizeof(info))
	ok, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return RECT{}, "", false
	}
	var device overlayDisplayDevice
	device.Size = uint32(unsafe.Sizeof(device))
	found, _, _ := enumOverlayDisplayDevices.Call(uintptr(unsafe.Pointer(&info.Device[0])), 0, uintptr(unsafe.Pointer(&device)), 1)
	if found != 0 && device.ID[0] != 0 {
		return info.RcWork, syscall.UTF16ToString(device.ID[:]), true
	}
	return info.RcWork, syscall.UTF16ToString(info.Device[:]), true
}

type overlayMonitorMatch struct {
	device string
	work   RECT
	found  bool
}

var matchOverlayMonitor = syscall.NewCallback(func(monitor, _, _, param uintptr) uintptr {
	match := (*overlayMonitorMatch)(unsafe.Pointer(param))
	if work, device, ok := overlayMonitorWork(monitor); ok && strings.EqualFold(device, match.device) {
		match.work, match.found = work, true
		return 0
	}
	return 1
})

func (o *chatOverlayWindow) rememberMonitorPosition() {
	var bounds RECT
	if ok, _, _ := procGetWindowRect.Call(o.hwnd, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
		return
	}
	monitor, _, _ := getOverlayMonitor.Call(o.hwnd, 2 /* MONITOR_DEFAULTTONEAREST */)
	if work, device, ok := overlayMonitorWork(monitor); ok && device != "" {
		o.state.OverlayHome = &overlayMonitorHome{device, bounds.Left - work.Left, bounds.Top - work.Top}
	}
}

func overlayPositionInWorkArea(bounds, work RECT, x, y int32) (int32, int32) {
	w, h := bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
	return min(max(x, work.Left), max(work.Left, work.Right-w)),
		min(max(y, work.Top), max(work.Top, work.Bottom-h))
}

func (o *chatOverlayWindow) restoreMonitorPosition() {
	if o.state.OverlayHome == nil {
		o.rememberMonitorPosition()
		return
	}
	var bounds RECT
	if ok, _, _ := procGetWindowRect.Call(o.hwnd, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
		return
	}
	home := o.state.OverlayHome
	match := overlayMonitorMatch{device: home.Device}
	enumOverlayMonitors.Call(0, 0, matchOverlayMonitor, uintptr(unsafe.Pointer(&match)))
	work := match.work
	x, y := bounds.Left, bounds.Top
	if match.found {
		x, y = work.Left+home.X, work.Top+home.Y
	} else {
		monitor, _, _ := getOverlayMonitor.Call(o.hwnd, 2 /* MONITOR_DEFAULTTONEAREST */)
		var ok bool
		work, _, ok = overlayMonitorWork(monitor)
		if !ok {
			return
		}
	}
	x, y = overlayPositionInWorkArea(bounds, work, x, y)
	if x != bounds.Left || y != bounds.Top {
		procSetWindowPos.Call(o.hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0015 /* SWP_NOSIZE|SWP_NOZORDER|SWP_NOACTIVATE */)
	}
}
