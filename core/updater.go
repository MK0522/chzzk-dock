package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	GithubReleasesAPI = "https://api.github.com/repos/MK0522/chzzk-dock/releases/latest"
)

// UpdateInfo: 최신 릴리즈 정보 DTO
type UpdateInfo struct {
	HasUpdate      bool   `json:"has_update"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseTitle   string `json:"release_title"`
	ReleaseNotes   string `json:"release_notes"`
	ReleaseURL     string `json:"release_url"`
	DownloadURL       string `json:"download_url"`
	AssetSize         int64  `json:"asset_size"`
	AlreadyDownloaded bool   `json:"already_downloaded"`
}

// UpdateProgress: 다운로드 및 실행 진행 상황
type UpdateProgress struct {
	Active   bool   `json:"active"`
	Percent  int    `json:"percent"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	SpeedBps int64  `json:"speed_bps"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
}

var (
	updateCacheMu   sync.RWMutex
	cachedUpdate    *UpdateInfo
	lastCheckTime   time.Time
	progressMu      sync.RWMutex
	currentProgress UpdateProgress
)

// ParseSemVer: "v0.5.12" 또는 "0.5.12" 형태의 문자열을 [major, minor, patch] 숫자로 분리
func ParseSemVer(v string) (int, int, int) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return 0, 0, 0
	}

	maj, _ := strconv.Atoi(parts[0])
	min := 0
	if len(parts) > 1 {
		min, _ = strconv.Atoi(parts[1])
	}
	patch := 0
	if len(parts) > 2 {
		// 뒤에 붙은 -beta, -rc 등 제거
		patchPart := strings.Split(parts[2], "-")[0]
		patch, _ = strconv.Atoi(patchPart)
	}
	return maj, min, patch
}

// CompareSemVer: v1 > v2 이면 1, v1 < v2 이면 -1, 같으면 0
func CompareSemVer(v1, v2 string) int {
	maj1, min1, pat1 := ParseSemVer(v1)
	maj2, min2, pat2 := ParseSemVer(v2)

	if maj1 != maj2 {
		if maj1 > maj2 {
			return 1
		}
		return -1
	}
	if min1 != min2 {
		if min1 > min2 {
			return 1
		}
		return -1
	}
	if pat1 != pat2 {
		if pat1 > pat2 {
			return 1
		}
		return -1
	}
	return 0
}

// CheckForUpdate: GitHub Releases 최신 버전 비동기 감지 및 10분 메모리 캐싱 (force=true 시 즉시 조회)
func CheckForUpdate(currentVersion string, force bool) (*UpdateInfo, error) {
	updateCacheMu.RLock()
	if !force && cachedUpdate != nil && time.Since(lastCheckTime) < 10*time.Minute {
		res := *cachedUpdate
		updateCacheMu.RUnlock()
		return &res, nil
	}
	updateCacheMu.RUnlock()

	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest("GET", GithubReleasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "chzzk-dock-updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub API 요청 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API 응답 오류 (HTTP %d)", resp.StatusCode)
	}

	var ghRelease struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("릴리즈 응답 파싱 실패: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  ghRelease.TagName,
		ReleaseTitle:   ghRelease.Name,
		ReleaseNotes:   ghRelease.Body,
		ReleaseURL:     ghRelease.HTMLURL,
		HasUpdate:      CompareSemVer(ghRelease.TagName, currentVersion) > 0,
	}

	for _, a := range ghRelease.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
			info.DownloadURL = a.BrowserDownloadURL
			info.AssetSize = a.Size
			break
		}
	}

	info.AlreadyDownloaded = IsInstallerReady(info.LatestVersion, info.AssetSize)
	CleanOldInstallers(info.LatestVersion)

	updateCacheMu.Lock()
	cachedUpdate = info
	lastCheckTime = time.Now()
	updateCacheMu.Unlock()

	return info, nil
}

// GetInstallerPath: 버전별 임시 인스톨러 파일 경로 반환 (%TEMP%\chzzk-dock-setup-<version>.exe)
func GetInstallerPath(version string) string {
	cleanVer := strings.TrimSpace(version)
	if cleanVer == "" {
		cleanVer = "latest"
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("chzzk-dock-setup-%s.exe", cleanVer))
}

// IsInstallerReady: 해당 버전의 인스톨러가 임시폴더에 완전히 보관되어 있는지 확인
func IsInstallerReady(version string, expectedSize int64) bool {
	p := GetInstallerPath(version)
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	if expectedSize > 0 && fi.Size() != expectedSize {
		return false
	}
	return fi.Size() > 0
}

// CleanOldInstallers: 현재 타겟 버전을 제외한 이전 버전의 임시 인스톨러 파일 정리
func CleanOldInstallers(currentKeepVersion string) {
	tmpDir := os.TempDir()
	matches, err := filepath.Glob(filepath.Join(tmpDir, "chzzk-dock-setup-*.exe"))
	if err == nil {
		keepName := filepath.Base(GetInstallerPath(currentKeepVersion))
		for _, m := range matches {
			if filepath.Base(m) != keepName {
				_ = os.Remove(m)
			}
		}
	}
	// 레거시 임시 파일 정리
	_ = os.Remove(filepath.Join(tmpDir, "chzzk-dock-installer-latest.exe"))
}

// GetUpdateProgress: 현재 다운로드 상태 반환
func GetUpdateProgress() UpdateProgress {
	progressMu.RLock()
	defer progressMu.RUnlock()
	return currentProgress
}

// StartDownloadAndInstall: 최신 인스톨러를 다운로드하고 Inno Setup 실행 후 앱 정상 종료
// (이미 동일 버전의 인스톨러가 보관되어 있는 경우 다운로드를 스킵하고 즉시 실행)
func StartDownloadAndInstall(targetVersion, downloadURL string, assetSize int64, onShutdown func()) error {
	updateCacheMu.RLock()
	if targetVersion == "" && cachedUpdate != nil {
		targetVersion = cachedUpdate.LatestVersion
	}
	if assetSize <= 0 && cachedUpdate != nil {
		assetSize = cachedUpdate.AssetSize
	}
	updateCacheMu.RUnlock()

	progressMu.Lock()
	if currentProgress.Active {
		progressMu.Unlock()
		return fmt.Errorf("이미 업데이트 다운로드가 진행 중입니다")
	}
	progressMu.Unlock()

	// 1. 이미 동일 버전의 인스톨러가 온전하게 다운로드되어 있는 경우 다운로드 스킵
	if IsInstallerReady(targetVersion, assetSize) {
		progressMu.Lock()
		currentProgress = UpdateProgress{
			Active:   false,
			Percent:  100,
			Received: assetSize,
			Total:    assetSize,
			Done:     true,
			Error:    "",
		}
		progressMu.Unlock()

		installerPath := GetInstallerPath(targetVersion)
		LogInfo("[Updater] 이미 다운로드된 인스톨러 발견 (%s). 다운로드를 건너뛰고 설치를 시작합니다.", installerPath)

		go func() {
			time.Sleep(500 * time.Millisecond)
			// 작은 설치 진행 바만 스쳐 지나가도록 /SILENT /SP- 옵션 적용
			cmd := exec.Command(installerPath, "/SILENT", "/SP-")
			if err := cmd.Start(); err != nil {
				LogError("[Updater] 인스톨러 실행 실패: %v", err)
				progressMu.Lock()
				currentProgress.Error = fmt.Sprintf("설치기 실행 실패: %v", err)
				progressMu.Unlock()
				return
			}

			LogInfo("[Updater] 새 버전 인스톨러 실행됨 (PID %d). 현재 프로세스 종료 준비...", cmd.Process.Pid)
			if onShutdown != nil {
				onShutdown()
			}
			os.Exit(0)
		}()

		return nil
	}

	// 2. 인스톨러가 없는 경우 다운로드 진행
	progressMu.Lock()
	currentProgress = UpdateProgress{
		Active:  true,
		Percent: 0,
		Done:    false,
		Error:   "",
	}
	progressMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				progressMu.Lock()
				currentProgress.Active = false
				currentProgress.Error = fmt.Sprintf("패닉 발생: %v", r)
				progressMu.Unlock()
			}
		}()

		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Get(downloadURL)
		if err != nil {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("다운로드 실패: %v", err)
			progressMu.Unlock()
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("다운로드 서버 응답 오류 (HTTP %d)", resp.StatusCode)
			progressMu.Unlock()
			return
		}

		totalSize := resp.ContentLength
		installerPath := GetInstallerPath(targetVersion)
		tmpPath := installerPath + ".tmp"

		outFile, err := os.Create(tmpPath)
		if err != nil {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("임시 파일 생성 실패: %v", err)
			progressMu.Unlock()
			return
		}

		var received int64
		buf := make([]byte, 32*1024)
		startTime := time.Now()
		lastTick := time.Now()

		for {
			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				_, wErr := outFile.Write(buf[:n])
				if wErr != nil {
					outFile.Close()
					_ = os.Remove(tmpPath)
					progressMu.Lock()
					currentProgress.Active = false
					currentProgress.Error = fmt.Sprintf("파일 쓰기 실패: %v", wErr)
					progressMu.Unlock()
					return
				}
				received += int64(n)

				now := time.Now()
				if now.Sub(lastTick) >= 150*time.Millisecond || rErr == io.EOF {
					lastTick = now
					pct := 0
					if totalSize > 0 {
						pct = int(received * 100 / totalSize)
					}
					elapsed := now.Sub(startTime).Seconds()
					speed := int64(0)
					if elapsed > 0 {
						speed = int64(float64(received) / elapsed)
					}

					progressMu.Lock()
					currentProgress.Received = received
					currentProgress.Total = totalSize
					currentProgress.Percent = pct
					currentProgress.SpeedBps = speed
					progressMu.Unlock()
				}
			}

			if rErr != nil {
				if rErr == io.EOF {
					break
				}
				outFile.Close()
				_ = os.Remove(tmpPath)
				progressMu.Lock()
				currentProgress.Active = false
				currentProgress.Error = fmt.Sprintf("데이터 수신 오류: %v", rErr)
				progressMu.Unlock()
				return
			}
		}
		outFile.Close()

		// 온전하게 다운로드된 임시 파일을 정식 인스톨러 이름으로 확정
		_ = os.Remove(installerPath)
		if err := os.Rename(tmpPath, installerPath); err != nil {
			progressMu.Lock()
			currentProgress.Active = false
			currentProgress.Error = fmt.Sprintf("인스톨러 확정 실패: %v", err)
			progressMu.Unlock()
			return
		}

		progressMu.Lock()
		currentProgress.Percent = 100
		currentProgress.Done = true
		currentProgress.Active = false
		progressMu.Unlock()

		LogInfo("[Updater] 인스톨러 다운로드 완료 (%d bytes) -> %s", received, installerPath)

		// 1초 후 인스톨러 실행 (/SILENT /SP- : 작은 설치 바만 스쳐 지나가도록 실행)
		time.Sleep(1 * time.Second)

		cmd := exec.Command(installerPath, "/SILENT", "/SP-")
		if err := cmd.Start(); err != nil {
			LogError("[Updater] 인스톨러 실행 실패: %v", err)
			progressMu.Lock()
			currentProgress.Error = fmt.Sprintf("설치기 실행 실패: %v", err)
			progressMu.Unlock()
			return
		}

		LogInfo("[Updater] 새 버전 인스톨러 실행됨 (PID %d). 현재 프로세스 종료 준비...", cmd.Process.Pid)
		if onShutdown != nil {
			onShutdown()
		}
		os.Exit(0)
	}()

	return nil
}
