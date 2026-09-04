package core

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ==============================================================================
// [SECURITY & AUDIT NOTE: Naver Login & Cookie Extraction]
// - 목적: 네이버 치지직 방송 제어(제목, 카테고리, 채팅 설정 등)를 위한 사용자 인증 세션 연동
// - 메커니즘:
//   1. 네이버의 2차 기기 인증 및 보안 캡차를 지원하기 위해 격리된 전용 WebView(Edge) 창 팝업
//   2. 사용자가 직접 정상 로그인을 완료하면 로컬 루프백(127.0.0.1) CDP를 통해
//      치지직 도메인의 방송 제어용 쿠키(NID_AUT, NID_SES)만 1회 감지/추출
//   3. 추출 즉시 Edge 프로세스를 안전하게 자동 종료하고 OS 보안 금고(Credential Manager)에 저장
// - 안전성 보장:
//   * 사용자 기존 브라우저 프로필과 완전 분리된 전용 격리 디렉터리(ChzzkObsDock\webview_profile) 사용
//   * 추출된 쿠키는 원격 서버로 일절 전송되지 않으며, 오직 네이버 공식 치지직 API 통신 프록시에만 로컬 사용
//   * 오픈소스 코드 서명(SignPath, Certum 등) 및 보안 소프트웨어 심사 시 악성 행위가 아닌 정당한 사용자 편의 기능임
// ==============================================================================

// 독립 프로세스로 실행될 때 core 패키지 동작 보장
const (
	TARGET_DOMAIN = "chzzk.naver.com"
	LOGIN_URL     = "https://nid.naver.com/nidlogin.login?url=https%3A%2F%2Fchzzk.naver.com%2F"
)

var (
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procCloseHandle  = kernel32.NewProc("CloseHandle")
	procGetLastError = kernel32.NewProc("GetLastError")
)

type CDPPage struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type CDPCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	HTTPOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
}

type CDPCookiesResult struct {
	ID     int `json:"id"`
	Result struct {
		Cookies []CDPCookie `json:"cookies"`
	} `json:"result"`
}

// findEdgePath: 시스템에 설치된 Microsoft Edge 실행 파일 경로 검색
func findEdgePath() string {
	candidates := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Microsoft\Edge\Application\msedge.exe`),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return "msedge.exe"
}

// getFreePort: 사용 가능한 로컬 TCP 포트 할당
func getFreePort() int {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return 9222
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 9222
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// sendRawCDPRequest: 순수 Go 기반 경량 WebSocket으로 CDP Network.getCookies 명령 전송
func queryCDPCookies(wsURL string) (map[string]string, error) {
	// wsURL 형식: ws://127.0.0.1:PORT/devtools/page/...
	cleanURL := strings.TrimPrefix(wsURL, "ws://")
	parts := strings.SplitN(cleanURL, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid ws url: %s", wsURL)
	}
	hostPort := parts[0]
	path := "/" + parts[1]

	conn, err := net.DialTimeout("tcp", hostPort, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// WebSocket 핸드셰이크 키 생성
	keyBytes := make([]byte, 16)
	_, _ = rand.Read(keyBytes)
	secKey := base64.StdEncoding.EncodeToString(keyBytes)

	// 핸드셰이크 HTTP 요청 작성
	req := fmt.Sprintf(
		"GET %s HTTP/1.1\r\n"+
			"Host: %s\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Key: %s\r\n"+
			"Sec-WebSocket-Version: 13\r\n\r\n",
		path, hostPort, secKey,
	)

	_, err = conn.Write([]byte(req))
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	// HTTP 핸드셰이크 응답 대기
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 101 {
		return nil, fmt.Errorf("handshake failed with status %d", resp.StatusCode)
	}

	// CDP Network.getCookies 명령 프레임 전송
	cmdJSON := `{"id": 100, "method": "Network.getCookies", "params": {"urls": ["https://chzzk.naver.com", "https://nid.naver.com"]}}`
	payload := []byte(cmdJSON)

	// WebSocket 텍스트 프레임 인코딩 (Client -> Server 마스킹 필수)
	maskKey := []byte{0x12, 0x34, 0x56, 0x78}
	maskedPayload := make([]byte, len(payload))
	for i := range payload {
		maskedPayload[i] = payload[i] ^ maskKey[i%4]
	}

	var frame []byte
	frame = append(frame, 0x81) // FIN + Text opcode
	if len(payload) < 126 {
		frame = append(frame, byte(0x80|len(payload))) // MASK bit on
	} else {
		frame = append(frame, 0x80|126)
		frame = append(frame, byte(len(payload)>>8), byte(len(payload)&0xFF))
	}
	frame = append(frame, maskKey...)
	frame = append(frame, maskedPayload...)

	_, err = conn.Write(frame)
	if err != nil {
		return nil, err
	}

	// 응답 프레임 수신 (최대 3초 대기)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		header := make([]byte, 2)
		_, err := io.ReadFull(reader, header)
		if err != nil {
			return nil, err
		}

		payloadLen := int(header[1] & 0x7F)
		if payloadLen == 126 {
			extLen := make([]byte, 2)
			_, err := io.ReadFull(reader, extLen)
			if err != nil {
				return nil, err
			}
			payloadLen = int(extLen[0])<<8 | int(extLen[1])
		} else if payloadLen == 127 {
			extLen := make([]byte, 8)
			_, err := io.ReadFull(reader, extLen)
			if err != nil {
				return nil, err
			}
			payloadLen = int(extLen[4])<<24 | int(extLen[5])<<16 | int(extLen[6])<<8 | int(extLen[7])
		}

		body := make([]byte, payloadLen)
		_, err = io.ReadFull(reader, body)
		if err != nil {
			return nil, err
		}

		var cdpResp CDPCookiesResult
		if err := json.Unmarshal(body, &cdpResp); err == nil && cdpResp.ID == 100 {
			cookiesMap := make(map[string]string)
			for _, c := range cdpResp.Result.Cookies {
				if c.Name == "NID_AUT" || c.Name == "NID_SES" {
					cookiesMap[c.Name] = c.Value
				}
			}
			return cookiesMap, nil
		}
	}
}

// CheckAndExtractCookies: 네이버 로그인 완료 즉시 NID_AUT / NID_SES 쿠키를 감지하여 자격 증명 관리자에 저장 후 창 자동 닫기
func checkAndExtractCookies(port int, cmd *exec.Cmd) {
	client := &http.Client{Timeout: 1 * time.Second}
	jsonURL := fmt.Sprintf("http://127.0.0.1:%d/json", port)

	for {
		// 프로세스가 종료되었는지 확인
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			fmt.Println("[Webview] 사용자가 창을 닫았습니다.")
			return
		}

		resp, err := client.Get(jsonURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			var pages []CDPPage
			if err := json.NewDecoder(resp.Body).Decode(&pages); err == nil {
				resp.Body.Close()

				for _, page := range pages {
					if page.Type == "page" && page.WebSocketDebuggerURL != "" {
						cookies, err := queryCDPCookies(page.WebSocketDebuggerURL)
						if err == nil && cookies != nil {
							aut, hasAut := cookies["NID_AUT"]
							ses, hasSes := cookies["NID_SES"]

							if hasAut && hasSes && aut != "" && ses != "" {
								SaveConfig(map[string]interface{}{
									"nid_aut": aut,
									"nid_ses": ses,
								})
								fmt.Println("[Webview Login] 세션 쿠키 추출 완료 (NID_AUT, NID_SES) -> 자격 증명 관리자에 저장됨.")
								time.Sleep(500 * time.Millisecond)

								// 브라우저 프로세스 안전하게 종료
								if cmd.Process != nil {
									_ = cmd.Process.Kill()
								}
								return
							}
						}
					}
				}
			} else {
				resp.Body.Close()
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// RunLoginWebview: 네이버 로그인 웹뷰 메인 엔트리포인트
func RunLoginWebview() {
	// [단일 인스턴스 보장] 중복 클릭/동시 실행 시 0x800700AA 충돌 원천 방지
	mutexNamePtr, _ := syscall.UTF16PtrFromString(`Local\ChzzkObsDock_Login_Mutex`)
	mutexHandle, _, _ := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(mutexNamePtr)))
	lastErr, _, _ := procGetLastError.Call()

	if lastErr == 183 { // ERROR_ALREADY_EXISTS
		fmt.Println("[Webview] 이미 로그인 창이 실행 중입니다. 중복 실행을 건너뜁니다.")
		if mutexHandle != 0 {
			procCloseHandle.Call(mutexHandle)
		}
		os.Exit(0)
	}
	defer func() {
		if mutexHandle != 0 {
			procCloseHandle.Call(mutexHandle)
		}
	}()

	// 2단계 기기 인증 정보가 유지되도록 영구 프로필 폴더 사용
	appData := os.Getenv("LOCALAPPDATA")
	if appData == "" {
		appData = os.Getenv("USERPROFILE")
	}
	profileDir := filepath.Join(appData, "ChzzkObsDock", "webview_profile")
	_ = os.MkdirAll(profileDir, 0755)

	edgePath := findEdgePath()
	debugPort := getFreePort()

	// Chromium 내부 엔진 디버그 로그(Failed to unregister class 등) 콘솔 노이즈 억제
	args := []string{
		fmt.Sprintf("--app=%s", LOGIN_URL),
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		fmt.Sprintf("--remote-debugging-port=%d", debugPort),
		"--window-size=460,650",
		"--log-level=3",
		"--no-first-run",
		"--no-default-browser-check",
	}

	cmd := exec.Command(edgePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}

	if err := cmd.Start(); err != nil {
		fmt.Printf("[Webview Error] 브라우저 실행 실패: %v\n", err)
		return
	}

	// 비동기로 쿠키 캡처 루프 가동
	checkAndExtractCookies(debugPort, cmd)

	// 프로세스 종료 대기
	_ = cmd.Wait()
	time.Sleep(300 * time.Millisecond)
}
