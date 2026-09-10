# <img src="docs/icon.png" width="40" alt=""> CHZZK OBS Dock

OBS Studio 안에서 사용자 브라우저 독(`http://localhost:8081`)을 추가하면 치지직(CHZZK) 방송 정보와 설정을 편하게 수정할 수 있는 초경량·고성능 로컬 서버 및 독 위젯 애플리케이션입니다.

과거 트위치 시절처럼 스트리머들이 번거롭게 치지직 스튜디오 웹에 접속할 필요 없이 OBS 화면 안에서 방송의 모든 것을 즉시 제어하는 것을 목표로 합니다.

| Chzzk OBS Dock | Twitch Info Dock |
| :---: | :---: |
| <img src="docs/preview.png" width="380" alt="Chzzk OBS Dock"> | <img src="docs/twitch.jpg" width="380" alt="Twitch Info Dock"> |
> 현재 개발 버전: `v0.5.2`

---

## ✨ 주요 기능

- 📝 **원클릭 방송 설정 동기화**
  - 방송 제목(원클릭 즉시 삭제 `✕`), 카테고리 실시간 검색 및 자동완성, 방송 태그 간편 추가/삭제(최대 10개)
- 🎬 **치지직 같이보기 연동**
  - 같이보기 목록에서 콘텐츠 선택 시 공식 규정에 따라 카테고리·클립·다시보기가 자동으로 고정 및 잠금 보호
- ⚙️ **방송 옵션 & 다시보기 세부 제어**
  - 19금 연령 제한, 클립 생성 허용 여부, 다시보기(확인 후 수동 게시 / 즉시 자동 게시 / 다시보기 없음) 설정
- 💬 **채팅 권한 및 실시간 모드 제어**
  - 채팅 참여 대상(모두 / 팔로워 전용 / 채널 관리자) 및 최소 팔로우 기간 설정
  - 방송 중 **이모티콘 전용 모드**, **저속 모드(3초~5분)** 클릭 즉시 실시간 반영
- ⭐ **후원 4종 실시간 제어 & 세부 설정**
  - 방송 중에도 **치즈**, **영상**, **미션** 후원을 클릭 한 번으로 즉시 ON/OFF 전환
  - 구독/후원 세부 설정 모달: 후원 알림, 영상 후원 제어, **구독 알림(볼륨/TTS)** 실시간 연동
- 🤝 **치지직 스튜디오 공식 규격 파티 관리 & 초대 링크 원클릭 합류**
  - 스튜디오 공식 파티 1:1 연동 (파티 생성, 파티명 변경, 누적 치즈액, 참여자 인원수 확인)
  - **초대 링크 합류**: 다른 스트리머가 보낸 초대 링크/토큰을 입력하면 즉시 해당 파티에 게스트로 합류
  - **역할 자동 분기**: `👑 방장` vs `🤝 게스트`를 자동 판별하여 게스트 전용 UI(`파티 나가기`, `다른 파티 합류`) 및 권한 보호 제공
- 🎛️ **치지직 공식 리모컨 무로그인 독립 창 (테스트중) (Edge WebView2)**
  - 네이버 계정 쿠키 자동 주입으로 별도 로그인 없이 0초 만에 공식 리모컨 미니 창 로드
  - **올인원 프레임리스 다크 윈도우**: 📌 항상 위 고정(Always-on-top) 토글, 새로고침, 최소화/최대화/닫기 일체형 타이틀바 및 마지막 창 위치/크기/상태 영속적 기억
  - 치지직 공식 리모컨 상단 헤더, 피드 본문, 우측 볼륨 패널 3단 높이 정밀 보정 (글자/헤더 겹침 및 잘림 해소)
  - 독 내부 미디어 단축 컨트롤러: `[⏮️ 이전]`, `[⏪ -10s]`, `[⏸️ 정지/재생]`, `[⏩ +10s]`, `[⏭️ 다음]`
- ⚡ **OBS 자동 연동 및 프로세스 와치독 (Watchdog)**
  - 독 설정에서 스위치 하나로 `scripts/chzzk_dock_launcher.lua` 자동 설치/삭제
  - OBS 실행 시 서버 자동 기동, OBS 종료 시 백엔드 안전 자동 종료 (유예 시간 10초~10분 사용자 맞춤 설정)
- 📋 **인메모리 실시간 로깅 & 트레이 진단 도구**
  - 디스크 파일을 오염시키지 않는 초경량 인메모리 로깅 및 시스템 트레이 메뉴에서 `로그 확인하기(메모장 열기)` / `로그 저장(.txt)` 지원
  - 치명적 오류 발생 시 Windows 네이티브 알림 팝업 안내
- 🔐 **OS 커널 레벨 무결점 로컬 보안 (Zero-File Security)**
  - 민감한 네이버 세션 쿠키를 디스크 파일(`config.json`)에 평문 저장하지 않고 **Windows 자격 증명 관리자(Windows Credential Manager)** 시스템 금고에 직접 암호화 보관
  - Local CSRF 방어(`X-Requested-With` 헤더 강제 검증), DNS Rebinding 방어, 엄격한 CORS Origin 화이트리스트
  - 비공식 API 과호출 방지를 위한 3초 인메모리 캐시 Rate Limiter
- 🚀 **Zero-CGO Pure Go 단일 실행 파일 (`chzzk-dock.exe`)**
  - Python 인터프리터나 CGO 컴파일러 없이 순수 Go 단일 바이너리로 컴파일되어 초경량·초고속 동작 (무콘솔 GUI 서브시스템 적용)

---

## 🚀 빠른 시작 가이드

### 1. 요구 사항
- **OS**: Windows 10 / Windows 11 (64-bit)
- **OBS Studio**: v28.0 이상 권장
- **WebView2 런타임**: Windows 10/11 기본 탑재

### 2. OBS 연동 및 자동 실행 (가장 추천 ⭐️)
평소에 서버를 수동으로 켤 필요 없이, **OBS를 켤 때 자동으로 서버가 켜지고 OBS를 끌 때 자동으로 꺼지도록** 설정할 수 있습니다:
1. `chzzk-dock.exe`를 실행하고 브라우저나 OBS 독에서 `http://localhost:8081`을 엽니다.
2. 상단 헤더 우측의 **[⚙️ 설정]** 클릭 ➡️ **`Chzzk-Dock 자동시작`** 스위치를 **ON**으로 켭니다. (관리자 권한 UAC 창이 뜨면 **[예]** 클릭)
3. OBS Studio 상단 메뉴 **[도구] ➡️ [스크립트] ➡️ [+] 버튼**을 누르고, 열리는 폴더에서 `chzzk_dock_launcher.lua`를 선택합니다.
4. 설정 완료! 이제 평소처럼 OBS만 켜고 끄시면 서버가 완전히 자동으로 함께 동작합니다.

### 3. OBS Studio 독(Dock) 등록
1. OBS Studio 실행 ➡️ 상단 메뉴 **[독(Docks)]** ➡️ **[사용자 지정 브라우저 독...]** 클릭
2. 독 이름: `치지직 방송 제어` (원하는 이름 입력)
3. URL: `http://localhost:8081` 입력 후 **[적용]** 클릭
4. OBS 원하는 위치에 드래그하여 도킹 완료!

### 4. 수동 단독 실행
- `chzzk-dock.exe`를 더블 클릭하여 단독 실행할 수 있습니다.
- OBS가 실행되지 않은 상태에서는 3분 동안 OBS 실행을 대기하며, OBS가 종료되면 설정된 유예 시간(기본 10초) 후 안전 자동 종료됩니다.
- 와치독 자동 종료 없이 상시 구동하려면 `--no-watchdog` 또는 `--standalone` 옵션으로 실행하세요.

---

## 📁 프로젝트 구조

```
chzzk-dock/
├── core/                         # 백엔드 핵심 비즈니스 로직 및 Win32 네이티브 바인딩
│   ├── credentials.go            # Windows Credential Manager(advapi32) 자격 증명 금고 관리
│   ├── obs_detector.go           # OBS 프로세스 감지, 경로 탐색 및 Lua 스크립트 설치
│   ├── remote_webview.go         # 치지직 공식 리모컨 전용 WebView2 플로팅 윈도우 & DWM 테두리
│   ├── security.go               # Local CSRF / DNS Rebinding 방어 및 Rate Limiter
│   ├── tray.go                   # Win32 네이티브 시스템 트레이 아이콘 구현 (Zero-CGO)
│   ├── watchdog.go               # OBS 프로세스 수명주기 감시 및 안전 자동 종료
│   └── webview.go                # 네이버 로그인 Edge WebView2 팝업 인증 모듈
├── docs/                         # 프로젝트 가이드 문서 및 이미지 에셋
│   ├── cookie_guide_1.png        # 네이버 쿠키 수동 복사 가이드 스크린샷 1
│   ├── cookie_guide_2.png        # 네이버 쿠키 수동 복사 가이드 스크린샷 2
│   ├── icon.png                  # 프로젝트 대표 아이콘
│   ├── preview.png               # Chzzk OBS 독 실제 구동 미리보기
│   ├── REMOTECONTROL_SPEC.md     # 치지직 공식 리모컨 명세 및 분석 문서
│   └── twitch.jpg                # 트위치 정보 독 비교 레퍼런스 이미지
├── scripts/                      # OBS Studio 연동 스크립트
│   └── chzzk_dock_launcher.lua   # OBS 기동 시 chzzk-dock 자동 실행/종료 초경량 Lua 스크립트
├── app.manifest                  # Windows 고해상도 PerMonitorV2 DPI 애플리케이션 매니페스트
├── BACKLOG.md                    # 애자일 스프린트 개발 트래커 및 작업 히스토리
├── build.bat                     # 단일 바이너리 자동 빌드 스크립트 (PE 리소스 자동 생성)
├── chzzk-dock.exe                # 최종 컴파일된 단일 바이너리 실행 파일
├── chzzk-obs-dock.html           # OBS 브라우저 독 통합 UI 위젯 (바이너리 임베딩)
├── go.mod, go.sum                # Go 모듈 및 의존성 라이브러리 정의
├── icon.ico                      # Windows 실행 파일용 아이콘 리소스
├── installer.iss                 # Inno Setup 기반 Windows 설치 파일 패키징 스크립트
├── main.go, main_test.go         # HTTP 프록시 서버 라우터, 엔드포인트 및 단위 테스트
├── README.md                     # 프로젝트 안내 및 사용자 가이드
└── versioninfo.json              # PE 리소스 메타데이터 정의 (한국어 UTF-16)
```

---

## 🛠️ 개발자 빌드 가이드

Go 1.22 이상 환경에서 `build.bat`을 실행하거나 아래 명령어로 직접 빌드합니다:

```powershell
# 1. PE 리소스 생성 (아이콘, 매니페스트, 메타데이터 번들링)
go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -64 -o resource_windows_amd64.syso

# 2. 단일 바이너리 빌드 (콘솔 창 숨김, 심볼 제거 및 로컬 경로 제거)
go build -trimpath -ldflags="-H windowsgui -s -w" -o chzzk-dock.exe .
```

---

## ⚠️ 면책 조항 (Disclaimer)

본 소프트웨어는 네이버(치지직)의 공식 배포 서비스가 아니며, 개인 방송 환경의 편의를 위해 제작된 독립 오픈소스 도구입니다. 본 프로그램 사용으로 인해 발생하는 모든 책임은 사용자 본인에게 있습니다.

---

> 🤖 **안내**: 본 `README.md` 문서는 AI에 의해 작성 및 정리되었습니다.
