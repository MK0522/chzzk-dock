package core

import (
	"errors"
	"math/bits"
)

type OverlayShortcut struct {
	Key       uint32 `json:"key"`
	Modifiers uint32 `json:"modifiers"`
	Label     string `json:"label"`
}

func LoadOverlayShortcut() (OverlayShortcut, error) {
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	settings, err := loadOverlaySettingsLocked()
	return settings.Shortcut, err
}

func SaveOverlayShortcut(shortcut OverlayShortcut) error {
	if shortcut.Key > 255 || shortcut.Modifiers > 15 || bits.OnesCount32(shortcut.Modifiers) > 3 || len(shortcut.Label) > 80 {
		return errors.New("단축키 값을 확인해 주세요.")
	}
	var previous OverlayShortcut
	overlaySettingsLock.Lock()
	settings, err := loadOverlaySettingsLocked()
	if err == nil {
		previous = settings.Shortcut
		settings.Shortcut = shortcut
		err = saveOverlaySettingsLocked(settings)
		if err == nil {
			removeLegacyOverlaySettings()
		}
	}
	overlaySettingsLock.Unlock()
	if err != nil {
		return err
	}
	if hwnd := overlayTrayWindow(); hwnd != 0 {
		procSendMessageTimeoutW := user32.NewProc("SendMessageTimeoutW")
		procSendMessageTimeoutW.Call(hwnd, overlayShortcutMessage, 0, 0, 0x0002 /* SMTO_ABORTIFHUNG */, 500, 0)
		if shortcut.Key != 0 && !OverlayHotkeyAvailable() {
			overlaySettingsLock.Lock()
			settings, loadErr := loadOverlaySettingsLocked()
			if loadErr == nil {
				settings.Shortcut = previous
				err = saveOverlaySettingsLocked(settings)
			} else {
				err = loadErr
			}
			overlaySettingsLock.Unlock()
			procSendMessageTimeoutW.Call(hwnd, overlayShortcutMessage, 0, 0, 0x0002 /* SMTO_ABORTIFHUNG */, 500, 0)
			if err != nil {
				return err
			}
			return errors.New("다른 프로그램에서 사용 중이거나 등록할 수 없는 단축키입니다. 다른 키를 지정해 주세요.")
		}
	}
	return nil
}
