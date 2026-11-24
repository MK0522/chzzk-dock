package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
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
	WindowOpacity             *int   `json:"windowOpacity,omitempty"`
	BackgroundOpacity         *int   `json:"backgroundOpacity,omitempty"`
	LinkLogin                 *bool  `json:"linkLogin,omitempty"`
}

func (layer OverlayLayer) LoginLinked() bool {
	return layer.LinkLogin == nil || *layer.LinkLogin
}

var overlayLayerID = regexp.MustCompile("^[a-zA-Z0-9-]{1,48}$")
var overlaySettingsLock sync.Mutex

func overlayDataDir() string {
	return filepath.Dir(getSettingsFilePath())
}

func LoadOverlayLayers() ([]OverlayLayer, error) {
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	data, err := os.ReadFile(filepath.Join(overlayDataDir(), "overlay_layers.json"))
	layers := []OverlayLayer{{ID: "chzzk", Name: "치지직 채팅"}}
	if errors.Is(err, os.ErrNotExist) {
		return layers, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &layers); err != nil {
		return nil, err
	}
	if layers == nil {
		layers = []OverlayLayer{}
	}
	return layers, nil
}

func LoadOverlayMode() (uint32, error) {
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	data, err := os.ReadFile(filepath.Join(overlayDataDir(), "overlay_mode.json"))
	if errors.Is(err, os.ErrNotExist) {
		return uint32(overlayPass), nil
	}
	if err != nil {
		return 0, err
	}
	var mode uint32
	if err := json.Unmarshal(data, &mode); err != nil {
		return 0, err
	}
	if mode > uint32(overlayInteract) {
		return uint32(overlayPass), nil
	}
	return mode, nil
}

func SaveOverlayMode(mode uint32) error {
	if mode > uint32(overlayInteract) {
		return errors.New("잘못된 오버레이 모드입니다.")
	}
	overlaySettingsLock.Lock()
	defer overlaySettingsLock.Unlock()
	dir := overlayDataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(mode)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "overlay-mode-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, "overlay_mode.json"))
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
		if !overlayLayerID.MatchString(layer.ID) || seen[layer.ID] || layer.Name == "" || len(layer.Name) > 240 || len(layer.CSS) > 64*1024 {
			return errors.New("레이어 이름, ID 또는 CSS를 확인해 주세요.")
		}
		for _, opacity := range []*int{layer.WindowOpacity, layer.BackgroundOpacity, layer.DonationBackgroundOpacity} {
			if opacity != nil && (*opacity < 0 || *opacity > 100) {
				return errors.New("불투명도는 0~100 사이여야 합니다.")
			}
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
	dir := overlayDataDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "overlay-layers-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(layers)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(dir, "overlay_layers.json"))
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
		procPostMessageW.Call(hwnd, overlayAppearanceMessage, uintptr(window), uintptr(background))
	}
}

func CloseOverlayLayer(id string) {
	if hwnd := overlayWindow(id); hwnd != 0 {
		procPostMessageW.Call(hwnd, WM_CLOSE_VAL, 0, 0)
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
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) == 3 && parts[0] == "live" && parts[1] != "" && parts[2] == "chat"
}

func overlayScriptFor(chzzkChat bool, css, stateArguments string) string {
	body, _ := json.Marshal(css)
	return fmt.Sprintf("(%s)(%t, %s, [%s]);", chatOverlayScript, chzzkChat, body, stateArguments)
}
