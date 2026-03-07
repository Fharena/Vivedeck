# VibeDeck Mobile (Flutter)

Prompt/Review/Status 3개 화면으로 agent 제어 루프를 실행하는 모바일 앱입니다.

## 포함 화면

- `Prompt`: `PROMPT_SUBMIT` 전송
- `Review`: `PATCH_APPLY`, `RUN_PROFILE` 전송
- `Status`:
  - P2P 시작/종료, runtime 상태/ACK 조회
  - Direct signaling + WebRTC 연결(페어링 claim -> signaling WS -> OFFER/ANSWER/ICE -> DataChannel)

## 주요 연동 API

- `POST /v1/agent/p2p/start`
- `GET /v1/agent/p2p/status`
- `POST /v1/agent/p2p/stop`
- `GET /v1/agent/runtime/state`
- `GET /v1/agent/runtime/acks/pending`
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

스크립트는 임시 드라이브(`V:`)를 매핑해 공백 없는 경로에서 테스트를 실행한 뒤 자동 정리합니다.

## 기본 연결 값

- Agent Base URL: `http://127.0.0.1:8080`
- Signaling Base URL: `http://127.0.0.1:8081`

에뮬레이터 환경에서는 `10.0.2.2` 등 환경별 호스트를 사용하세요.
