# VibeDeck Mobile (Flutter)

Prompt/Review/Status 3개 화면을 agent HTTP API에 연결한 모바일 앱입니다.

## 포함 화면

- `Prompt`: `PROMPT_SUBMIT` 전송
- `Review`: `PATCH_APPLY`, `RUN_PROFILE` 전송
- `Status`: P2P 시작/종료, runtime 상태/ACK 조회

## 주요 연동 API

- `POST /v1/agent/p2p/start`
- `GET /v1/agent/p2p/status`
- `POST /v1/agent/p2p/stop`
- `GET /v1/agent/runtime/state`
- `GET /v1/agent/runtime/acks/pending`
- `POST /v1/agent/envelope`

화면에서 제어 응답을 받으면 non-ACK 응답에 대해 `CMD_ACK`를 자동 회신합니다.

## 실행 전 준비

```bash
# 시스템 전역 또는 로컬 SDK 경로 중 하나 사용
flutter --version
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

## 기본 연결 값

- Agent Base URL: `http://127.0.0.1:8080`
- Signaling Base URL: `http://127.0.0.1:8081`

에뮬레이터 환경에서는 `10.0.2.2` 등 환경별 호스트를 사용하세요.
