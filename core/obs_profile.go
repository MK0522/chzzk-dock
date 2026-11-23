package core

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type BitrateTargetInfo struct {
	TargetKbps  int    `json:"targetKbps"`
	Fixed       bool   `json:"fixed"`
	RateControl string `json:"rateControl,omitempty"`
}

type ObsBitrateTargets struct {
	Mode   string            `json:"mode"`
	Stream BitrateTargetInfo `json:"stream"`
	Record BitrateTargetInfo `json:"record"`
}

var nonFixedRateControls = map[string]bool{
	"CQP":      true,
	"CRF":      true,
	"ICQ":      true,
	"LA_ICQ":   true,
	"LOSSLESS": true,
}

// GetObsBitrateTargets: %APPDATA%\obs-studio\basic\profiles\<profile> 에서 목표 비트레이트 설정을 조회합니다.
func GetObsBitrateTargets(profileName string) ObsBitrateTargets {
	dir, ok := resolveObsProfileDir(profileName)
	if !ok {
		return ObsBitrateTargets{}
	}
	return ParseObsBitrateTargetFromDir(dir)
}

func resolveObsProfileDir(profileName string) (string, bool) {
	name := strings.TrimSpace(profileName)
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", false
	}
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return "", false
	}
	profilesRoot := filepath.Join(appData, "obs-studio", "basic", "profiles")
	candidates := []string{
		filepath.Join(profilesRoot, name),
		filepath.Join(profilesRoot, strings.ReplaceAll(name, " ", "_")),
	}
	for _, cand := range candidates {
		clean := filepath.Clean(cand)
		rel, err := filepath.Rel(profilesRoot, clean)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		if info, err := os.Stat(filepath.Join(clean, "basic.ini")); err == nil && !info.IsDir() {
			return clean, true
		}
	}
	return "", false
}

// ParseObsBitrateTargetFromDir: 지정된 OBS 프로필 디렉터리에서 방송/녹화 목표 비트레이트를 파싱합니다.
func ParseObsBitrateTargetFromDir(profileDir string) ObsBitrateTargets {
	iniData, err := os.ReadFile(filepath.Join(profileDir, "basic.ini"))
	if err != nil {
		return ObsBitrateTargets{}
	}
	ini := parseSimpleINI(iniData)
	mode := strings.TrimSpace(ini["Output"]["Mode"])
	if mode == "" {
		mode = "Simple"
	}

	if strings.EqualFold(mode, "Advanced") {
		stream := readEncoderJSONTarget(filepath.Join(profileDir, "streamEncoder.json"))
		var record BitrateTargetInfo
		recType := strings.TrimSpace(ini["AdvOut"]["RecType"])
		if strings.EqualFold(recType, "FFmpeg") {
			ffBitrate, _ := strconv.Atoi(strings.TrimSpace(ini["AdvOut"]["FFVBitrate"]))
			if ffBitrate > 0 {
				record = BitrateTargetInfo{TargetKbps: ffBitrate, Fixed: true, RateControl: "FFmpeg"}
			} else {
				record = BitrateTargetInfo{Fixed: false, RateControl: "FFmpeg"}
			}
		} else {
			recEncoder := strings.TrimSpace(ini["AdvOut"]["RecEncoder"])
			if recEncoder == "" || strings.EqualFold(recEncoder, "none") {
				record = stream
			} else {
				record = readEncoderJSONTarget(filepath.Join(profileDir, "recordEncoder.json"))
			}
		}
		return ObsBitrateTargets{
			Mode:   "Advanced",
			Stream: stream,
			Record: record,
		}
	}

	// Simple Output Mode
	vBitrate, _ := strconv.Atoi(strings.TrimSpace(ini["SimpleOutput"]["VBitrate"]))
	if vBitrate <= 0 {
		vBitrate = 2500
	}
	stream := BitrateTargetInfo{TargetKbps: vBitrate, Fixed: true, RateControl: "CBR"}

	recQuality := strings.TrimSpace(ini["SimpleOutput"]["RecQuality"])
	var record BitrateTargetInfo
	if recQuality == "" || strings.EqualFold(recQuality, "Stream") {
		record = stream
	} else {
		record = BitrateTargetInfo{TargetKbps: 0, Fixed: false, RateControl: recQuality}
	}

	return ObsBitrateTargets{
		Mode:   "Simple",
		Stream: stream,
		Record: record,
	}
}

func readEncoderJSONTarget(path string) BitrateTargetInfo {
	data, err := os.ReadFile(path)
	if err != nil {
		return BitrateTargetInfo{}
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return BitrateTargetInfo{}
	}
	rc, _ := m["rate_control"].(string)
	rcUpper := strings.ToUpper(strings.TrimSpace(rc))
	if nonFixedRateControls[rcUpper] {
		return BitrateTargetInfo{TargetKbps: 0, Fixed: false, RateControl: rcUpper}
	}
	var bitrate int
	if v, ok := m["bitrate"].(float64); ok && v > 0 {
		bitrate = int(v)
	}
	if bitrate <= 0 {
		return BitrateTargetInfo{TargetKbps: 0, Fixed: false, RateControl: rcUpper}
	}
	return BitrateTargetInfo{TargetKbps: bitrate, Fixed: true, RateControl: rcUpper}
}

func parseSimpleINI(data []byte) map[string]map[string]string {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	res := make(map[string]map[string]string)
	section := ""
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if res[section] == nil {
				res[section] = make(map[string]string)
			}
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			k := strings.TrimSpace(line[:idx])
			v := strings.TrimSpace(line[idx+1:])
			if res[section] == nil {
				res[section] = make(map[string]string)
			}
			res[section][k] = v
		}
	}
	return res
}
