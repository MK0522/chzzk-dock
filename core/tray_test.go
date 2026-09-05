package core

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestSetClipboardText(t *testing.T) {
	testText := "http://localhost:8081?test=win32_clipboard"
	if err := SetClipboardText(testText); err != nil {
		t.Fatalf("SetClipboardText failed: %v", err)
	}

	// Verify clipboard contents
	procOpenClipboard := user32.NewProc("OpenClipboard")
	procCloseClipboard := user32.NewProc("CloseClipboard")
	procGetClipboardData := user32.NewProc("GetClipboardData")
	procGlobalLock := kernel32.NewProc("GlobalLock")
	procGlobalUnlock := kernel32.NewProc("GlobalUnlock")

	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		t.Fatalf("OpenClipboard failed during verification")
	}
	defer procCloseClipboard.Call()

	hData, _, _ := procGetClipboardData.Call(uintptr(CF_UNICODETEXT))
	if hData == 0 {
		t.Fatalf("GetClipboardData failed")
	}

	ptr, _, _ := procGlobalLock.Call(hData)
	if ptr == 0 {
		t.Fatalf("GlobalLock failed during verification")
	}
	defer procGlobalUnlock.Call(hData)

	got := syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(ptr))[:])
	if got != testText {
		t.Fatalf("expected clipboard text %q, got %q", testText, got)
	}
}
