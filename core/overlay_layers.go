package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const OverlayRestartCode = 73

type OverlayLayer struct {
	DonationBackgroundOpacity *int   `json:"donationBackgroundOpacity,omitempty"`
	Enabled                   bool   `json:"enabled,omitempty"`
	RestoreChannel            string `json:"restoreChannel,omitempty"`
	ID                        string `json:"id"`
	Name                      string `json:"name"`
	URL                       string `json:"url"`
	BackgroundColor           string `json:"backgroundColor,omitempty"`
	CSS                       string `json:"css"`
	CSSPass                   string `json:"cssPass,omitempty"`
	CSSArrange                string `json:"cssArrange,omitempty"`
	CSSInteract               string `json:"cssInteract,omitempty"`
	WindowOpacity             *int   `json:"windowOpacity,omitempty"`
	BackgroundOpacity         *int   `json:"backgroundOpacity,omitempty"`
	ChatScale                 *int   `json:"chatScale,omitempty"`
	LinkLogin                 *bool  `json:"linkLogin,omitempty"`
}

func (layer OverlayLayer) ModeCSSMap() map[string]string {
	pass := layer.CSSPass
	arrange := layer.CSSArrange
	interact := layer.CSSInteract
	if pass == "" && arrange == "" && interact == "" && layer.CSS != "" {
		pass = layer.CSS
		arrange = layer.CSS
	}
	return map[string]string{
		"0": pass,
		"1": arrange,
		"2": interact,
	}
}

func (layer OverlayLayer) LoginLinked() bool {
	return layer.LinkLogin == nil || *layer.LinkLogin
}

var overlayLayerID = regexp.MustCompile("^[a-zA-Z0-9-]{1,48}$")
var overlaySettingsLock sync.Mutex

func overlayDataDir() string {
	return filepath.Dir(getSettingsFilePath())
}

type overlaySettings struct {
	Layers   []OverlayLayer  `json:"layers"`
	Mode     uint32          `json:"mode"`
	Shortcut OverlayShortcut `json:"shortcut"`
}

func defaultOverlaySettings() overlaySettings {
	return overlaySettings{
		Layers: []OverlayLayer{{ID: "chzzk", Name: "치지직 채팅"}},
		Mode:   uint32(overlayPass),
	}
}

func saveOverlaySettingsLocked(settings overlaySettings) error {
	dir := overlayDataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "overlay-settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "overlay_settings.json"))
}

func removeLegacyOverlaySettings() {
	for _, name := range []string{"overlay_layers.json", "overlay_mode.json", "overlay_shortcut.json", "overlay_preview_window.json"} {
		_ = os.Remove(filepath.Join(overlayDataDir(), name))
	}
}

// loadOverlaySettingsLocked migrates legacy files only after the combined file is written.
func loadOverlaySettingsLocked() (overlaySettings, error) {
	settings := defaultOverlaySettings()
	dir := overlayDataDir()
	data, err := os.ReadFile(filepath.Join(dir, "overlay_settings.json"))
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return overlaySettings{}, err
		}
		if settings.Layers == nil {
			settings.Layers = []OverlayLayer{}
		}
		if settings.Mode > uint32(overlayInteract) {
			settings.Mode = uint32(overlayPass)
		}
		return settings, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return overlaySettings{}, err
	}

	if data, err = os.ReadFile(filepath.Join(dir, "overlay_layers.json")); err == nil {
		if err := json.Unmarshal(data, &settings.Layers); err != nil {
			return overlaySettings{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return overlaySettings{}, err
	}
	if data, err = os.ReadFile(filepath.Join(dir, "overlay_mode.json")); err == nil {
		if err := json.Unmarshal(data, &settings.Mode); err != nil {
			return overlaySettings{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return overlaySettings{}, err
	}
	if data, err = os.ReadFile(filepath.Join(dir, "overlay_shortcut.json")); err == nil {
		if err := json.Unmarshal(data, &settings.Shortcut); err != nil {
			return overlaySettings{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return overlaySettings{}, err
	}
	if settings.Layers == nil {
		settings.Layers = []OverlayLayer{}
	}
	if settings.Mode > uint32(overlayInteract) {
		settings.Mode = uint32(overlayPass)
	}
	if err := saveOverlaySettingsLocked(settings); err != nil {
		return overlaySettings{}, err
	}
	removeLegacyOverlaySettings()
	return settings, nil
}

func LoadOverlayLayers() ([]OverlayLayer, error) {
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	settings, err := loadOverlaySettingsLocked()
	return settings.Layers, err
}

func LoadOverlayMode() (uint32, error) {
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	settings, err := loadOverlaySettingsLocked()
	return settings.Mode, err
}

func SaveOverlayMode(mode uint32) error {
	if mode > uint32(overlayInteract) {
		return errors.New("잘못된 오버레이 모드입니다.")
	}
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	settings, err := loadOverlaySettingsLocked()
	if err != nil {
		return err
	}
	settings.Mode = mode
	if err := saveOverlaySettingsLocked(settings); err != nil {
		return err
	}
	removeLegacyOverlaySettings()
	return nil
}
func overlayURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	return u, err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}

func overlaySameOrigin(a, b string) bool {
	u, ok := overlayURL(a)
	v, valid := overlayURL(b)
	return ok && valid && u.Scheme == v.Scheme && strings.EqualFold(u.Host, v.Host)
}

func overlaySourceMatches(source, target string, custom bool) bool {
	if !overlaySameOrigin(source, target) {
		return false
	}
	if custom {
		return true
	}
	u, _ := overlayURL(source)
	v, _ := overlayURL(target)
	return strings.TrimRight(u.Path, "/") == strings.TrimRight(v.Path, "/")
}

func SaveOverlayLayers(layers []OverlayLayer) error {
	if len(layers) > 20 {
		return errors.New("레이어는 20개까지 저장할 수 있습니다.")
	}
	seen := make(map[string]bool, len(layers))
	for i := range layers {
		layer := &layers[i]
		layer.Name = strings.TrimSpace(layer.Name)
		layer.URL = strings.TrimSpace(layer.URL)
		if !overlayLayerID.MatchString(layer.ID) || seen[layer.ID] || layer.Name == "" || len(layer.Name) > 240 ||
			len(layer.CSS) > 64*1024 || len(layer.CSSPass) > 64*1024 || len(layer.CSSArrange) > 64*1024 || len(layer.CSSInteract) > 64*1024 {
			return errors.New("레이어 이름, ID 또는 CSS를 확인해 주세요.")
		}
		for _, opacity := range []*int{layer.WindowOpacity, layer.BackgroundOpacity, layer.DonationBackgroundOpacity} {
			if opacity != nil && (*opacity < 0 || *opacity > 100) {
				return errors.New("불투명도는 0~100 사이여야 합니다.")
			}
		}
		if layer.ChatScale != nil && (*layer.ChatScale < 100 || *layer.ChatScale > 170) {
			return errors.New("채팅 크기는 100~170% 사이여야 합니다.")
		}
		if layer.BackgroundColor != "" {
			if len(layer.BackgroundColor) != 7 || layer.BackgroundColor[0] != '#' {
				return errors.New("올바른 배경 색상을 선택해 주세요.")
			}
			if _, err := strconv.ParseUint(layer.BackgroundColor[1:], 16, 24); err != nil {
				return err
			}
		}
		seen[layer.ID] = true
		if layer.ID == "chzzk" && layer.URL == "" {
			continue
		}
		if _, ok := overlayURL(layer.URL); !ok || len(layer.URL) > 4096 {
			return fmt.Errorf("%s의 URL은 http:// 또는 https:// 주소여야 합니다.", layer.Name)
		}
	}
	if layers == nil {
		layers = []OverlayLayer{}
	}
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	settings, err := loadOverlaySettingsLocked()
	if err != nil {
		return err
	}
	settings.Layers = layers
	if err := saveOverlaySettingsLocked(settings); err != nil {
		return err
	}
	removeLegacyOverlaySettings()
	return nil
}

func overlayLayerMutex(id string) string {
	if id == "chzzk" {
		return `Local\ChzzkChatOverlayMutex`
	}
	return `Local\ChzzkOverlayLayer-` + id
}

func overlayLayerStateFile(id string) string {
	if id == "chzzk" {
		return "chat_overlay_window.json"
	}
	return "overlay_layer_" + id + ".json"
}

func overlayLayerClass(id string) string {
	if id == "chzzk" {
		return overlayClassName
	}
	return overlayClassName + "-" + id
}

func overlayWindow(id string) uintptr {
	class, err := syscall.UTF16PtrFromString(overlayLayerClass(id))
	if err != nil {
		return 0
	}
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(class)), 0)
	return hwnd
}

func OverlayLayerOpen(id string) bool { return overlayWindow(id) != 0 }

func ApplyOverlayAppearance(layer OverlayLayer) {
	if hwnd := overlayWindow(layer.ID); hwnd != 0 {
		window, background := 100, 0
		if layer.WindowOpacity != nil {
			window = *layer.WindowOpacity
		}
		if layer.BackgroundOpacity != nil {
			background = *layer.BackgroundOpacity
		}
		color := uint64(0x141517)
		if len(layer.BackgroundColor) == 7 {
			color, _ = strconv.ParseUint(layer.BackgroundColor[1:], 16, 24)
		}
		donation := 100
		if layer.DonationBackgroundOpacity != nil {
			donation = *layer.DonationBackgroundOpacity
		}
		procPostMessageW.Call(hwnd, overlayDonationMessage, uintptr(donation), 0)
		procPostMessageW.Call(hwnd, overlayColorMessage, uintptr(color), 0)
		chatScale := 100
		if layer.ChatScale != nil {
			chatScale = min(max(*layer.ChatScale, 100), 170)
		}
		procPostMessageW.Call(hwnd, overlayScaleMessage, uintptr(chatScale), 0)
		procPostMessageW.Call(hwnd, overlayAppearanceMessage, uintptr(window), uintptr(background))
	}
}

func CloseOverlayLayer(id string) {
	if hwnd := overlayWindow(id); hwnd != 0 {
		procPostMessageW.Call(hwnd, WM_CLOSE_VAL, 0, 0)
	}
}

func RestartOverlayLayer(id string) {
	if hwnd := overlayWindow(id); hwnd != 0 {
		procPostMessageW.Call(hwnd, overlayRestartMessage, 0, 0)
	}
}

func OrderOverlayLayers(layers []OverlayLayer) {
	for i := len(layers) - 1; i >= 0; i-- {
		if hwnd := overlayWindow(layers[i].ID); hwnd != 0 {
			procSetWindowPos.Call(hwnd, HWND_TOPMOST_VAL, 0, 0, 0, 0, 0x0013) // no move, size, or activation
		}
	}
}

func RunOverlayLayer(channelID, id string) error {
	layers, err := LoadOverlayLayers()
	if err != nil {
		return err
	}
	for _, layer := range layers {
		if layer.ID != id {
			continue
		}
		if layer.ID == "chzzk" && layer.URL == "" {
			if channelID == "" {
				return errors.New("치지직 채널을 선택해 주세요.")
			}
			runChatWebview(channelID, true, layer)
			return nil
		}
		target, ok := overlayURL(layer.URL)
		if !ok || !overlayLayerID.MatchString(layer.ID) {
			break
		}
		if target.Path == "" {
			target.Path = "/"
		}
		cfg := ToolWebviewConfig{
			Title: layer.Name, ClassName: overlayLayerClass(layer.ID),
			MutexName:     overlayLayerMutex(layer.ID),
			StateFileName: overlayLayerStateFile(layer.ID),
			DefaultWidth:  440, DefaultHeight: 720,
			TargetURL: target.String(), ProfileDir: filepath.Join(overlayDataDir(), "overlay_profiles", layer.ID),
			ChatOverlay: true, CustomOverlay: true, OverlayCSS: layer.CSS,
			OverlayLayer: layer,
		}
		if isChzzkChatURL(cfg.TargetURL) {
			configureOverlayLogin(&cfg, layer)
		}
		RunToolWebview(cfg)
		return nil
	}
	return errors.New("오버레이 레이어를 찾을 수 없습니다.")
}

func configureOverlayLogin(cfg *ToolWebviewConfig, layer OverlayLayer) {
	if layer.LoginLinked() {
		cfg.CookieInjector = injectUserSessionCookies
		return
	}
	cfg.CookieInjector = nil
	// The signed-in WebView profile can already contain cookies. Use a separate one.
	cfg.ProfileDir = filepath.Join(overlayDataDir(), "overlay_profiles_anonymous", layer.ID)
}

func isChzzkChatURL(raw string) bool {
	u, ok := overlayURL(raw)
	if !ok || u.Scheme != "https" || !strings.EqualFold(u.Host, "chzzk.naver.com") {
		return false
	}
	cleaned := path.Clean("/" + u.Path)
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	return len(parts) == 3 && parts[0] == "live" && parts[1] != "" && parts[2] == "chat"
}

func overlayScriptFor(chzzkChat bool, css any, stateArguments string) string {
	body, _ := json.Marshal(css)
	return fmt.Sprintf("(%s)(%t, %s, [%s]);", chatOverlayScript, chzzkChat, body, stateArguments)
}
