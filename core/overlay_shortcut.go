package core

import (
	"encoding/json"
	"errors"
	"math/bits"
	"os"
	"path/filepath"
)

type OverlayShortcut struct {
	Key       uint32 `json:"key"`
	Modifiers uint32 `json:"modifiers"`
	Label     string `json:"label"`
}

func LoadOverlayShortcut() (OverlayShortcut, error) {
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	var shortcut OverlayShortcut
	data, err := os.ReadFile(filepath.Join(overlayDataDir(), "overlay_shortcut.json"))
	if errors.Is(err, os.ErrNotExist) {
		return shortcut, nil
	}
	if err != nil {
		return shortcut, err
	}
	err = json.Unmarshal(data, &shortcut)
	return shortcut, err
}

func SaveOverlayShortcut(shortcut OverlayShortcut) error {
	if shortcut.Key > 255 || shortcut.Modifiers > 15 || bits.OnesCount32(shortcut.Modifiers) > 3 || len(shortcut.Label) > 80 {
		return errors.New("단축키 값을 확인해 주세요.")
	}
	overlaySettingsLock.Lock()
	file := filepath.Join(overlayDataDir(), "overlay_shortcut.json")
	previous, previousErr := os.ReadFile(file)
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		overlaySettingsLock.Unlock()
		return previousErr
	}
	data, err := json.Marshal(shortcut)
	if err == nil {
		err = os.MkdirAll(overlayDataDir(), 0700)
	}
	if err == nil {
		err = os.WriteFile(file, data, 0600)
	}
	overlaySettingsLock.Unlock()
	if err != nil {
		return err
	}
	if hwnd := overlayTrayWindow(); hwnd != 0 {
		procSendMessageW.Call(hwnd, overlayShortcutMessage, 0, 0)
		if shortcut.Key != 0 && !OverlayHotkeyAvailable() {
			overlaySettingsLock.Lock()
			if previousErr == nil {
				err = os.WriteFile(file, previous, 0600)
			} else {
				err = os.Remove(file)
			}
			overlaySettingsLock.Unlock()
			procSendMessageW.Call(hwnd, overlayShortcutMessage, 0, 0)
			if err != nil {
				return err
			}
			return errors.New("다른 프로그램에서 사용 중이거나 등록할 수 없는 단축키입니다. 다른 키를 지정해 주세요.")
		}
	}
	return nil
}
