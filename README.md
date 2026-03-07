# VibeDeck

VibeDeck은 로컬 AI 코딩 세션을 모바일에서 검토/제어하기 위한 모바일 우선 컨트롤 레이어입니다.
코드 실행은 PC에서 수행하고, 모바일은 다음 루프를 담당합니다.

- 프롬프트 제출
- 패치/디프 검토
- 전체/부분 적용
- 테스트/빌드 실행
- 결과 확인 및 재지시

현재 저장소는 MVP 핵심 범위를 구현 완료했고, post-MVP 첫 단계로 실제 Cursor/VS Code extension host가 열어주는 localhost TCP bridge 경로까지 포함합니다.

- Go 기반 Signaling/Relay 서버 골격
- Go 기반 PC Agent 오케스트레이션 + Cursor 브리지 stdio/TCP RPC 연결
- TypeScript 기반 Cursor 브리지 계약/stdio server/socket server/extension runtime helper
- VS Code/Cursor localhost bridge extension package(`extensions/vibedeck-bridge`)
- WebRTC Peer + SignalBridge + Agent P2P 제어 경로(Envelope) 라우팅 + ACK retry/backoff
- Flutter 모바일 앱(Prompt/Review/Status) + direct signaling/WebRTC control path + ACK observability UI
- 공통 프로토콜/데이터 모델 문서
- ACK runtime metrics endpoint(`/v1/agent/runtime/metrics`) + Status 화면 관측 카드

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
  cursor-bridge/ # TypeScript 브리지 계약 + stdio/tcp bridge 서버 + extension runtime helper
extensions/
  vibedeck-bridge/ # VS Code/Cursor localhost bridge extension package
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
- Cursor 또는 VS Code(실제 extension host bridge를 확인할 때)

### 빠른 실행

기본값은 fixture child-process bridge입니다.

```bash
npm --prefix adapters/cursor-bridge install
npm --prefix adapters/cursor-bridge run build
go run ./cmd/signaling
go run ./cmd/relay
go run ./cmd/agent
```

`cmd/agent`는 기본적으로 `adapters/cursor-bridge/dist/fixtureBridgeMain.js`를 child process로 실행합니다.
bridge 명령을 교체할 때는 다음 환경변수를 사용합니다.

- `CURSOR_BRIDGE_BIN`: 기본값 `node`
- `CURSOR_BRIDGE_ENTRY`: 기본값 `adapters/cursor-bridge/dist/fixtureBridgeMain.js`
- `CURSOR_BRIDGE_ARGS_JSON`: 실행 인자를 JSON 배열로 직접 지정할 때 사용
- `CURSOR_BRIDGE_WORKDIR`: bridge 프로세스 working directory override
- `CURSOR_BRIDGE_CALL_TIMEOUT`: RPC 호출 타임아웃, 기본값 `10s`
- `CURSOR_BRIDGE_STARTUP_TIMEOUT`: bridge 초기 handshake 타임아웃, 기본값 `5s`

### 실제 extension host 연결

Cursor/VS Code 안에서 localhost TCP bridge를 열고 agent가 거기에 붙게 하려면 다음 순서를 사용합니다.

```bash
npm --prefix adapters/cursor-bridge install
npm --prefix adapters/cursor-bridge run build
npm --prefix extensions/vibedeck-bridge install
npm --prefix extensions/vibedeck-bridge run build
```

1. `extensions/vibedeck-bridge`를 Cursor/VS Code extension으로 로드
2. extension 설정에서 `vibedeckBridge.mode`를 `mock` 또는 `command`로 지정
3. command mode라면 `VibeDeck: Validate Commands`로 필수 command 준비 상태 확인
4. `VibeDeck: Copy Agent Env`로 agent 환경변수 문자열 복사
5. agent 실행 전에 아래 값을 붙여 넣고 `go run ./cmd/agent` 실행

```powershell
$env:CURSOR_BRIDGE_TCP_ADDR = "127.0.0.1:7797"
$env:CURSOR_BRIDGE_TCP_DIAL_TIMEOUT = "3s"
go run ./cmd/agent
```

- `mock` 모드는 실제 extension host 경로 smoke test용입니다.
- `command` 모드는 필수 command ID가 extension host에 등록되어 있어야 시작됩니다.
- 저장소에는 실제 Cursor AI task command 매핑이 포함되어 있지 않으므로, 실사용하려면 해당 command를 제공하는 환경 또는 별도 adapter가 필요합니다.

## 문서

- 구현 계획: [docs/implementation-plan.md](./docs/implementation-plan.md)
- 아키텍처: [docs/architecture.md](./docs/architecture.md)
- 크리티컬 이슈 로그: [docs/critical-issues.md](./docs/critical-issues.md)
- 해결 학습 가이드: [docs/troubleshooting-study.md](./docs/troubleshooting-study.md)
