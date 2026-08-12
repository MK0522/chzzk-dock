# <img src="icon.png" width="40" alt="icon"> CHZZK OBS Dock
OBS Studio 안에서 CHZZK 방송 정보를 관리할 수 있는 작은 도크 프로그램입니다.

방송 제목, 카테고리, 태그와 채팅 설정을 OBS 화면에서 바로 확인하고 변경할 수 있습니다.
<p align="center">
    <img src="preview.png" width="300" alt="icon">
</p>

> 아직 정식 버전이 아닌 개발 중인 프로젝트입니다.

## 주요 기능

- 방송 제목·카테고리·태그 수정
- 채팅 참여 범위, 구독자 전용, 느린 모드 등 채팅 설정 변경
- CHZZK 계정 로그인 및 인증
- OBS 사용자 지정 브라우저 도크 지원
- 트레이 아이콘에서 도크 주소 복사 및 서버 종료

## 사용 방법

1. [Releases](../../releases)에서 `server.exe`를 다운로드합니다.
2. `server.exe`를 실행합니다.
3. OBS Studio에서 **보기 → 독 → 사용자 지정 브라우저 독**을 엽니다.
4. URL에 아래 주소를 입력합니다.

```text
http://localhost:8081
```

5. 열린 도크의 **설정** 버튼에서 CHZZK 계정을 인증합니다.

이후 도크에서 방송 정보를 수정하고 **설정 저장**을 누르면 됩니다.

## 참고

- `https://localhost:8080`은 CHZZK 로그인 인증에만 사용됩니다. OBS에는 `http://localhost:8081`을 등록하세요.
- 처음 인증할 때 `localhost` 인증서 경고가 나타날 수 있습니다. 로컬 인증용이므로 주소가 `localhost`인지 확인한 후 진행하세요.
- 서버를 종료하려면 작업 표시줄 알림 영역의 아이콘을 우클릭한 뒤 **서버 종료**를 선택합니다.
- [치지직 공식 API](https://chzzk.gitbook.io/chzzk)을 이용했습니다. [치지직 Developers](https://developers.chzzk.naver.com/application)에서 현재 애플리케이션을 등록하고 `Client ID`, `Client Secret`을 수동으로 입력해 사용합니다.

## 문제 해결

실행 시 “포트를 사용할 수 없습니다”라는 오류가 보이면, 이미 실행 중인 `server.exe` 또는 테스트용 Python 서버를 종료한 후 다시 실행하세요.
