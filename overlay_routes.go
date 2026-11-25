package main

import (
	"net/http"

	"chzzk-dock/core"
)

func handleOverlayShortcut(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		var shortcut core.OverlayShortcut
		if !bindJSON(w, r, &shortcut) {
			return
		}
		if err := core.SaveOverlayShortcut(shortcut); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	shortcut, err := core.LoadOverlayShortcut()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(w, shortcut)
}

func handleOverlayLayers(w http.ResponseWriter, r *http.Request) {
	layers, err := core.LoadOverlayLayers()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var restartedOpen map[string]bool
	if r.Method == http.MethodPut {
		var next []core.OverlayLayer
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if !bindJSON(w, r, &next) {
			return
		}
		if err := core.SaveOverlayLayers(next); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		byID := make(map[string]core.OverlayLayer, len(next))
		for _, layer := range next {
			byID[layer.ID] = layer
		}
		restartedOpen = make(map[string]bool, len(layers))
		for _, layer := range layers {
			updated, exists := byID[layer.ID]
			if !exists {
				core.CloseOverlayLayer(layer.ID)
				continue
			}
			prevCSS := layer.ModeCSSMap()
			nextCSS := updated.ModeCSSMap()
			cssChanged := prevCSS["0"] != nextCSS["0"] || prevCSS["1"] != nextCSS["1"] || prevCSS["2"] != nextCSS["2"]
			if layer.Name != updated.Name || layer.URL != updated.URL || cssChanged || layer.LoginLinked() != updated.LoginLinked() {
				if core.OverlayLayerOpen(layer.ID) {
					restartedOpen[layer.ID] = true
					core.RestartOverlayLayer(layer.ID)
				}
			} else {
				core.ApplyOverlayAppearance(updated)
			}
		}
		layers = next
		if layers == nil {
			layers = []core.OverlayLayer{}
		}
		core.OrderOverlayLayers(layers)
	}
	running := make(map[string]bool, len(layers))
	for _, layer := range layers {
		running[layer.ID] = core.OverlayLayerOpen(layer.ID) || restartedOpen[layer.ID]
	}
	respondOK(w, map[string]any{"layers": layers, "running": running, "mode": core.CurrentOverlayMode(), "hotkey": core.OverlayHotkeyAvailable()})
}

func handleOverlayAction(w http.ResponseWriter, r *http.Request) {
	var action struct {
		ID                        string
		Action                    string
		Mode                      *uint32
		DonationBackgroundOpacity *int
		WindowOpacity             *int
		BackgroundColor           string
		BackgroundOpacity         *int
		ChatScale                 *int
	}
	if !bindJSON(w, r, &action) {
		return
	}
	if action.Action == "cycle" {
		core.RequestOverlayCycle()
		respondOK(w, map[string]any{"ok": true})
		return
	}
	if action.Action == "mode" {
		if action.Mode == nil || !core.RequestOverlayMode(*action.Mode) {
			respondErr(w, http.StatusBadRequest, "모드를 전환할 수 없습니다.")
			return
		}
		respondOK(w, map[string]any{"ok": true})
		return
	}
	layers, err := core.LoadOverlayLayers()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	for layerIndex, layer := range layers {
		if layer.ID != action.ID {
			continue
		}
		switch action.Action {
		case "preview":
			if action.DonationBackgroundOpacity == nil || *action.DonationBackgroundOpacity < 0 || *action.DonationBackgroundOpacity > 100 || action.WindowOpacity == nil || action.BackgroundOpacity == nil || *action.WindowOpacity < 0 || *action.WindowOpacity > 100 || *action.BackgroundOpacity < 0 || *action.BackgroundOpacity > 100 {
				respondErr(w, http.StatusBadRequest, "불투명도는 0~100 사이여야 합니다.")
				return
			}
			if len(action.BackgroundColor) != 7 || action.BackgroundColor[0] != '#' {
				respondErr(w, http.StatusBadRequest, "배경 색상을 확인해 주세요.")
				return
			}
			layer.DonationBackgroundOpacity = action.DonationBackgroundOpacity
			layer.BackgroundColor = action.BackgroundColor
			layer.WindowOpacity = action.WindowOpacity
			layer.BackgroundOpacity = action.BackgroundOpacity
			if action.ChatScale != nil && *action.ChatScale >= 100 && *action.ChatScale <= 170 {
				layer.ChatScale = action.ChatScale
			}
			core.ApplyOverlayAppearance(layer)
		case "hide":
			layers[layerIndex].Enabled = false
			if err := core.SaveOverlayLayers(layers); err != nil {
				respondErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			core.CloseOverlayLayer(layer.ID)
		case "show":
			if !core.OverlayLayerOpen(layer.ID) {
				channel := ""
				if layer.ID == "chzzk" && layer.URL == "" {
					channel = resolveChannelIdFromRequest(r)
					if channel == "" {
						respondErr(w, http.StatusBadRequest, "기본 치지직 채팅을 열려면 로그인하고 채널을 선택해 주세요.")
						return
					}
				}
				layers[layerIndex].Enabled = true
				layers[layerIndex].RestoreChannel = channel
				if err := core.SaveOverlayLayers(layers); err != nil {
					respondErr(w, http.StatusInternalServerError, err.Error())
					return
				}
				if err := startToolWebview("--chat-overlay", channel, layer.ID); err != nil {
					layers[layerIndex].Enabled = false
					_ = core.SaveOverlayLayers(layers)
					respondErr(w, http.StatusInternalServerError, "레이어를 열 수 없습니다.")
					return
				}
			}
		default:
			respondErr(w, http.StatusBadRequest, "지원하지 않는 작업입니다.")
			return
		}
		respondOK(w, map[string]any{"ok": true})
		return
	}
	respondErr(w, http.StatusNotFound, "레이어를 찾을 수 없습니다.")
}
