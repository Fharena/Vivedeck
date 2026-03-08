# VibeDeck Mobile (Flutter)

대화/검토/상태 3개 화면으로 공유 스레드 기반 agent 제어 루프를 실행하는 모바일 앱입니다.

## 포함 화면

- `대화`:
  - 최근 스레드 목록
  - 스레드 타임라인
  - 자연어 프롬프트 작성 및 `PROMPT_SUBMIT`
- `검토`:
  - `PATCH_APPLY`, `RUN_PROFILE`
  - 동적 run profile 목록
  - 전체 실행 출력/stdout/stderr 확인
- `Status`:
  - P2P 시작/종료, runtime 상태/ACK 조회
  - 현재 workspace adapter / 작업 디렉토리 / binary 확인
  - Direct signaling + WebRTC 연결(페어링 claim -> signaling WS -> OFFER/ANSWER/ICE -> DataChannel)

## 주요 연동 API

- `POST /v1/agent/p2p/start`
- `GET /v1/agent/p2p/status`
- `POST /v1/agent/p2p/stop`
- `GET /v1/agent/runtime/state`
- `GET /v1/agent/runtime/acks/pending`
- `GET /v1/agent/runtime/adapter`
- `GET /v1/agent/run-profiles`
- `GET /v1/agent/threads`
- `GET /v1/agent/threads/{id}`
- `POST /v1/agent/envelope`
- `POST /v1/pairings/{code}/claim`
- `GET /v1/sessions/{sid}/ws?deviceKey=...&role=mobile`

화면에서 제어 응답을 받으면 non-ACK 응답에 대해 `CMD_ACK`를 자동 회신합니다.

## 실행 전 준비

```bash
# 시스템 전역 또는 로컬 SDK 경로 중 하나 사용
flutter --version
# 또는 저장소 루트 기준
.\tools\flutter\bin\flutter.bat --version
```

## 로컬 실행

```bash
cd mobile/flutter_app
..\..\tools\flutter\bin\flutter.bat pub get
..\..\tools\flutter\bin\flutter.bat run
```

Windows에서 저장소 경로에 공백/한글이 포함되면 Android Gradle이 `local.properties`의 `flutter.sdk` 경로를 깨뜨릴 수 있습니다. 이 경우 아래 safe 스크립트를 사용하세요.

```bash
# 연결된 Android 기기가 1대면 자동 선택
powershell -ExecutionPolicy Bypass -File .\scripts\flutter_safe.ps1

# 특정 기기를 직접 지정
powershell -ExecutionPolicy Bypass -File .\scripts\flutter_safe.ps1 -DeviceId <device-id>
```

스크립트는 `%TEMP%\vibedeck_flutter_safe\repo` junction 경로를 만들고, 그 ASCII 경로에서 Flutter/Gradle을 실행한 뒤 자동 정리합니다.

## 테스트/분석

```bash
..\..\tools\flutter\bin\flutter.bat analyze
..\..\tools\flutter\bin\flutter.bat test
```

Windows에서 저장소 경로에 공백/한글이 포함되고 `flutter_webrtc` 의존성이 있을 때, `flutter test`가 native-assets 훅(`objective_c`) 문제로 실패할 수 있습니다. 이 경우 아래 스크립트를 사용하세요.

```bash
# mobile/flutter_app
powershell -ExecutionPolicy Bypass -File .\scripts\flutter_test_safe.ps1
```

스크립트는 `%TEMP%\vibedeck_flutter_safe\repo` junction 경로에서 테스트를 실행한 뒤 자동 정리합니다.

## 기본 연결 값

- Agent Base URL: `http://127.0.0.1:8080`
- Signaling Base URL: `http://127.0.0.1:8081`

에뮬레이터 환경에서는 `10.0.2.2` 등 환경별 호스트를 사용하세요.
## 현재 방향

- 앱은 가능한 한 상태/스레드/run profile/workspace 정보를 자동 조회합니다.
- IDE는 extension 또는 패키지 설치만으로 최소 설정 상태를 목표로 합니다.
- Cursor가 첫 번째 provider지만, 모바일 UI는 특정 IDE에 묶이지 않도록 thread/review/run 모델만 기준으로 설계합니다.
