# VibeDeck

VibeDeck은 로컬 AI 코딩 세션을 모바일에서 검토/제어하기 위한 모바일 우선 컨트롤 레이어입니다.
코드 실행은 PC에서 수행하고, 모바일은 다음 루프를 담당합니다.

- 프롬프트 제출
- 패치/디프 검토
- 전체/부분 적용
- 테스트/빌드 실행
- 결과 확인 및 재지시

현재 저장소는 MVP 1단계 베이스라인을 포함합니다.

- Go 기반 Signaling/Relay 서버 골격
- Go 기반 PC Agent 오케스트레이션 + Cursor 브리지 stdio RPC 연결
- TypeScript 기반 Cursor 브리지 계약/프로세스 서버
- WebRTC Peer + SignalBridge + Agent P2P 제어 경로(Envelope) 라우팅
- Flutter 모바일 앱 베이스라인(Prompt/Review/Status)
- 공통 프로토콜/데이터 모델 문서

## 디렉터리 구조

```text
cmd/
  agent/      # PC 에이전트 실행 진입점
  signaling/  # 페어링 + 시그널링 서버 진입점
  relay/      # 릴레이 폴백 서버 진입점
internal/
  agent/      # 잡 오케스트레이션, 실행 프로파일, p2p 세션
  relay/      # 릴레이 라우팅, 백프레셔 정책
  signaling/  # 페어링/세션 시그널링 모델
  protocol/   # 공통 Envelope, 메시지 타입
  runtime/    # ACK 추적, 연결 상태머신
  webrtc/     # peer + signaling bridge 스켈레톤
adapters/
  cursor-bridge/ # TypeScript 브리지 계약 + stdio bridge 서버
mobile/
  flutter_app/   # Flutter Prompt/Review/Status 화면 베이스라인
shared/
  protocol/   # JSON Schema, 메시지 레퍼런스
docs/
  implementation-plan.md
  architecture.md
  critical-issues.md
  troubleshooting-study.md
```

## MVP 우선순위

1. 연결 안정성(P2P 우선, Relay 폴백)
2. Prompt -> Patch -> Apply 루프
3. Run -> Result 루프
4. 리뷰 UX 명확성
5. 어댑터 중심 확장성

## 로컬 개발

### 요구사항

- Go 1.23+
- Node.js 22+
- Flutter 3.22+

### 빠른 실행

```bash
npm --prefix adapters/cursor-bridge install
npm --prefix adapters/cursor-bridge run build
go run ./cmd/signaling
go run ./cmd/relay
go run ./cmd/agent
```

`cmd/agent`는 기본적으로 `adapters/cursor-bridge/dist/fixtureBridgeMain.js`를 child process로 실행합니다.
실제 Cursor extension 런처를 붙일 때는 다음 환경변수로 bridge 명령을 교체할 수 있습니다.

- `CURSOR_BRIDGE_BIN`: 기본값 `node`
- `CURSOR_BRIDGE_ENTRY`: 기본값 `adapters/cursor-bridge/dist/fixtureBridgeMain.js`
- `CURSOR_BRIDGE_ARGS_JSON`: 실행 인자를 JSON 배열로 직접 지정할 때 사용
- `CURSOR_BRIDGE_WORKDIR`: bridge 프로세스 working directory override
- `CURSOR_BRIDGE_CALL_TIMEOUT`: RPC 호출 타임아웃, 기본값 `10s`
- `CURSOR_BRIDGE_STARTUP_TIMEOUT`: bridge 초기 handshake 타임아웃, 기본값 `5s`

## 문서

- 구현 계획: [docs/implementation-plan.md](./docs/implementation-plan.md)
- 아키텍처: [docs/architecture.md](./docs/architecture.md)
- 크리티컬 이슈 로그: [docs/critical-issues.md](./docs/critical-issues.md)
- 해결 학습 가이드: [docs/troubleshooting-study.md](./docs/troubleshooting-study.md)