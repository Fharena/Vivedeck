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
- Cursor CLI (cursor-agent, 실제 AI patch 생성을 확인할 때, optional)

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
`WORKSPACE_ADAPTER_MODE=cursor_agent_cli`를 설정하면 bridge 대신 공식 `cursor-agent` CLI를 임시 git worktree에서 실행하고, 생성된 diff만 review/apply 흐름으로 반환합니다.
bridge 명령을 교체할 때는 다음 환경변수를 사용합니다.

- `CURSOR_BRIDGE_BIN`: 기본값 `node`
- `CURSOR_BRIDGE_ENTRY`: 기본값 `adapters/cursor-bridge/dist/fixtureBridgeMain.js`
- `CURSOR_BRIDGE_ARGS_JSON`: 실행 인자를 JSON 배열로 직접 지정할 때 사용
- `CURSOR_BRIDGE_WORKDIR`: bridge 프로세스 working directory override
- `CURSOR_BRIDGE_CALL_TIMEOUT`: RPC 호출 타임아웃, 기본값 `10s`
- `CURSOR_BRIDGE_STARTUP_TIMEOUT`: bridge 초기 handshake 타임아웃, 기본값 `5s`

### Cursor Agent CLI 대체 경로

bridge command 매핑 대신 공식 Cursor CLI를 쓰려면 다음 환경변수를 사용합니다.

```powershell
$env:WORKSPACE_ADAPTER_MODE = "cursor_agent_cli"
$env:CURSOR_AGENT_BIN = "cursor-agent"
go run ./cmd/agent
```

Windows에서 네이티브 `cursor-agent`가 없고 WSL에만 설치돼 있다면 아래처럼 실행할 수 있습니다.

```powershell
$env:WORKSPACE_ADAPTER_MODE = "cursor_agent_cli"
$env:CURSOR_AGENT_USE_WSL = "true"
$env:CURSOR_AGENT_WSL_DISTRO = "Ubuntu" # optional, 비우면 agent가 설치된 distro를 자동 탐지
go run ./cmd/agent
```

기본 동작:

- 네이티브 `cursor-agent` 또는 WSL distro 안의 `~/.local/bin/cursor-agent`/`~/.local/bin/agent`를 탐지
- 감지한 CLI를 임시 git worktree에서 실행
- 현재 workspace의 tracked 변경과 untracked 파일을 temp worktree에 동기화
- agent가 만든 diff만 `PATCH_READY`로 반환
- review 승인 후 실제 workspace에는 `git apply`로 반영

추가 환경변수:

- `CURSOR_AGENT_BIN`: 기본값 `cursor-agent`, WSL 모드에서는 기본값 `wsl.exe`
- `CURSOR_AGENT_ARGS_JSON`: CLI 인자를 JSON 배열로 직접 지정할 때 사용, 기본값은 `["--print","--output-format","json"]`이고 `--trust`, `--model <값>`이 없으면 agent가 자동 주입
- `CURSOR_AGENT_TRUST_WORKSPACE`: 기본값 `true`, CLI 인자에 `--trust`가 없으면 자동 추가
- `CURSOR_AGENT_MODEL`: 기본값 `auto`, CLI 인자에 model이 없으면 `--model <값>` 자동 추가
- `CURSOR_AGENT_ENV_JSON`: CLI 실행 환경변수를 JSON 배열로 지정할 때 사용
- `CURSOR_AGENT_USE_WSL`: Windows에서 WSL 안의 Cursor CLI를 사용할 때 `true`
- `CURSOR_AGENT_WSL_DISTRO`: 특정 WSL distro를 강제로 지정할 때 사용, 비우면 자동 탐지
- `CURSOR_AGENT_WORKSPACE_ROOT`: workspace root override
- `CURSOR_AGENT_TEMP_ROOT`: 임시 worktree parent directory override
- `CURSOR_AGENT_PROMPT_TIMEOUT`: 프롬프트 실행 타임아웃, 기본값 `2m`
- `CURSOR_AGENT_RUN_TIMEOUT`: run profile 실행 타임아웃, 기본값 `2m`

### Cursor Agent Smoke Script

네이티브 `cursor-agent` 또는 WSL에 설치된 Cursor CLI가 있으면 아래 스크립트로 temp repo 기준 smoke 테스트를 바로 돌릴 수 있습니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\cursor_agent_smoke.ps1
```

스크립트가 하는 일:

- temp git repo 생성
- `WORKSPACE_ADAPTER_MODE=cursor_agent_cli`로 agent 실행
- `GET /v1/agent/runtime/adapter`로 adapter/binary 상태 확인
- `PROMPT_SUBMIT -> PATCH_APPLY -> RUN_PROFILE(smoke)` 순서 실행
- 실제 변경이 temp repo 파일에 반영됐는지 확인

현재 스크립트는 네이티브 `cursor-agent`가 없어도 WSL 안의 `cursor-agent`/`agent`를 자동 탐지합니다. 이 PC에서는 `cursor-agent login` 완료 후 실제 `PROMPT_SUBMIT -> PATCH_APPLY -> RUN_PROFILE` smoke가 통과했습니다. headless 실행 안정성을 위해 agent는 `--trust`와 `--model auto`를 기본 주입하고, HTTP/P2P control 경로는 message type별 timeout(`PROMPT_SUBMIT`/`RUN_PROFILE`: 2분, `PATCH_APPLY`: 30초)을 사용합니다.

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
