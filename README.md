# VibeDeck

VibeDeck은 로컬 AI 코딩 세션을 모바일에서 검토/제어하기 위한 모바일 우선 컨트롤 레이어입니다.
코드 실행은 PC에서 수행하고, 모바일은 다음 루프를 담당합니다.

- 프롬프트 제출
- 패치/디프 검토
- 전체/부분 적용
- 테스트/빌드 실행
- 결과 확인 및 재지시

현재 저장소는 MVP 핵심 범위를 구현 완료했고, post-MVP 단계에서 실제 Cursor GUI extension host + built-in cursor-agent provider real smoke proof까지 확보했습니다.

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

현재 스크립트는 네이티브 `cursor-agent`가 없어도 WSL 안의 `cursor-agent`/`agent`를 자동 탐지합니다. 이 PC에서는 `cursor-agent login` 완료 후 실제 `PROMPT_SUBMIT -> PATCH_APPLY -> RUN_PROFILE` smoke가 통과했습니다. headless 실행 안정성을 위해 agent는 `--print`, `--output-format json`, `--trust`, `--model auto`를 기본 주입하고, HTTP/P2P control 경로는 message type별 timeout(`PROMPT_SUBMIT`/`RUN_PROFILE`: 5분, `PATCH_APPLY`: 30초)을 사용합니다.

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
3. command mode를 쓸 때는 기본값 `vibedeckBridge.commandProvider=builtin_cursor_agent` 그대로 두면 extension이 기본 `vibedeck.*` 명령을 직접 등록
4. Windows에서 Cursor CLI가 WSL에만 있으면 `vibedeckBridge.cursorAgent.useWsl=true`, 필요하면 `vibedeckBridge.cursorAgent.wslDistro=Ubuntu` 설정
5. `VibeDeck: Validate Commands`로 command registry readiness 확인
6. `VibeDeck: Copy Agent Env`로 bridge 주소를 복사해 agent 실행 터미널에 붙여 넣기

```powershell
$env:CURSOR_BRIDGE_TCP_ADDR = "127.0.0.1:7797"
$env:CURSOR_BRIDGE_TCP_DIAL_TIMEOUT = "3s"
go run ./cmd/agent
```

mock mode smoke는 아래 스크립트로 바로 검증할 수 있습니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\extension_host_smoke.ps1 -BridgeAddress "127.0.0.1:7797"
```

command mode built-in provider smoke는 두 단계로 볼 수 있습니다.

```powershell
npm --prefix extensions/vibedeck-bridge run smoke:provider
npm --prefix extensions/vibedeck-bridge run smoke:extension
```

- `smoke:provider`는 built-in provider와 command bridge의 low-level 경로를 검증합니다.
- `smoke:extension`은 `extension.ts -> bridgeExtensionController -> built-in provider -> TCP bridge` 활성화 경로를 검증합니다.
- `VibeDeck: Copy Smoke Command`는 mock mode에서는 `extension_host_smoke.ps1`, built-in command mode에서는 `npm --prefix extensions/vibedeck-bridge run smoke:extension` 명령을 복사합니다.

실제 GUI extension host + built-in provider smoke는 아래 스크립트로 검증합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\gui_extension_host_smoke.ps1 -Editor cursor -UseWslCursorAgent true -CursorAgentBin /home/fharena/.local/bin/cursor-agent -CursorAgentWslDistro Ubuntu
```

현재 상태:

- `mock` 모드는 실제 extension host 안에서 등록된 mock command를 통해 `Prompt -> Patch -> Apply -> Run` smoke를 검증합니다.
- `command` 모드의 기본값은 built-in `cursor-agent` provider이며, 별도 외부 command ID 없이도 기본 `vibedeck.*` 매핑으로 시작할 수 있습니다.
- `smoke:extension`으로 저장소 안의 activation path를 자동 검증하고, `gui_extension_host_smoke.ps1`로 실제 Cursor GUI extension host + real `cursor-agent` 경로까지 검증했습니다.
- 현재 남은 큰 과제는 ignored/generated 파일 sync 정책, 패키징/온보딩, Windows cleanup warning 정리입니다.
- Windows에서는 smoke 종료 직후 `agent.exe` 잠금 때문에 temp root cleanup warning이 남을 수 있습니다.

## 문서

- 구현 계획: [docs/implementation-plan.md](./docs/implementation-plan.md)
- 아키텍처: [docs/architecture.md](./docs/architecture.md)
- 크리티컬 이슈 로그: [docs/critical-issues.md](./docs/critical-issues.md)
- 해결 학습 가이드: [docs/troubleshooting-study.md](./docs/troubleshooting-study.md)
