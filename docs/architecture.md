# 아키텍처

## 시스템 구성 요소

- 모바일 앱(Flutter): Prompt/Review/Status 화면 제공
- PC 에이전트(Go): 잡 오케스트레이션, 패치 수명주기, 실행 프로파일, 전송 바인딩, Cursor 브리지 child-process/TCP 연결 관리, cursor-agent CLI worktree adapter 관리
- Cursor 브리지(TypeScript): Cursor extension host 추상화, extension runtime helper, stdio/TCP RPC 서버, 컨텍스트 조회, 패치 적용, 파일/라인 열기
- VibeDeck Bridge Extension(VS Code/Cursor): localhost TCP bridge package, mock mode와 built-in/external command provider 기반 command mode 제공
- Signaling 서버(Go): 페어링 및 WebRTC 시그널링 부트스트랩
- Relay 서버(Go): 폴백 이벤트 라우팅 + 백프레셔 정책

## Flutter UI 베이스라인 (`mobile/flutter_app`)

- `PromptScreen`: 프롬프트 입력, 템플릿 선택, context 옵션 토글
- `ReviewScreen`: 파일/헝크 목록 검토와 전체/선택 적용 액션
- `StatusScreen`: 연결 상태, pending ACK, ACK observability(RTT/queue depth), 히스토리 표시
- `StatusScreen`에서 direct signaling + WebRTC 상태(ws/peer/datachannel) 제어/로그 확인 가능

## Flutter API/전송 레이어 (`mobile/flutter_app/lib/state/app_controller.dart`)

- `AppController`가 화면 상태와 제어 경로(HTTP/DIRECT) 라우팅을 단일 진입점으로 관리
- `AgentApi`(`mobile/flutter_app/lib/services/agent_api.dart`)가 HTTP 요청/오류 처리 담당
- `AppController`가 `/v1/agent/runtime/metrics`를 조회해 ACK RTT/queue depth/transport split 메트릭을 상태 화면에 반영
- 기본 제어 메시지 전송 흐름:
  1. `PROMPT_SUBMIT` / `PATCH_APPLY` / `RUN_PROFILE` envelope 전송
  2. 응답 envelope 파싱
  3. non-ACK 응답 RID에 대해 `CMD_ACK` 자동 회신
- direct 제어 경로:
  - `SignalingApi`(`mobile/flutter_app/lib/services/signaling_api.dart`)가 pairing claim + WS URI 생성 담당
  - `MobileDirectSignalingSession`(`mobile/flutter_app/lib/services/mobile_direct_signaling_session.dart`)가 signaling WS + `flutter_webrtc` peer를 결합
  - `SIGNAL_OFFER` 수신 시 answer 생성/송신, `SIGNAL_ICE` 적용, local ICE 송신 처리
  - DataChannel open(Control Ready) 시 제어 envelope를 DIRECT 경로로 전송, 실패 시 HTTP 경로로 폴백

## 핵심 인터페이스

### WorkspaceAdapter

에이전트 코어는 IDE 종속 로직을 몰라야 하므로 어댑터 추상화에 의존합니다.

주요 메서드:

- `GetContext`
- `SubmitTask`
- `GetPatch`
- `ApplyPatch`
- `RunProfile`
- `GetRunResult`
- `OpenLocation`

TypeScript 브리지 패키지 구성:

- `CursorExtensionBridge`가 Cursor command 결과를 `WorkspaceAdapter` 계약으로 정규화
- `CursorCommandHost`가 에디터 상태 조회/명령 호출 추상화를 제공
- `createVSCodeCursorHost`가 VS Code/Cursor runtime과 브리지 계약을 연결
- `createCursorExtensionRuntime`이 extension activation 시 command registration과 last-run metadata 추적을 담당
- `serveCursorExtensionBridge`, `serveStdioBridge`, `serveSocketBridge`가 newline-delimited JSON RPC over stdio/TCP 서버를 구성
- extensions/vibedeck-bridge는 mock mode, built-in cursor-agent provider, external command registry 연동, 설정 기반 command ID 매핑, command registry readiness 검증, agent env 복사 명령을 제공하는 설치 가능한 extension package입니다.
- `bridgeExtensionController`는 extension 활성화 로직을 주입 가능한 controller로 분리해 fake host 기반 smoke에서도 같은 시작 경로를 재사용합니다.
- CursorAgentCLIAdapter는 네이티브 cursor-agent 또는 Windows WSL distro 안의 `~/.local/bin/cursor-agent`/`agent`를 감지해 임시 git worktree snapshot에서 실행하고, tracked 변경 + untracked 파일 + 명시 allowlist와 일치하는 ignored 파일만 snapshot에 반영한 뒤 생성된 diff를 PatchReadyPayload로 파싱합니다. 실제 workspace 반영은 review 승인 후 git apply로만 수행합니다.

## 에이전트-어댑터 연결

- `cmd/agent`는 기본적으로 Node child process로 Cursor 브리지를 실행합니다.
- 기본 엔트리포인트는 `adapters/cursor-bridge/dist/fixtureBridgeMain.js`이며, 환경변수로 실제 extension 런처 명령으로 교체할 수 있습니다.
- `CURSOR_BRIDGE_TCP_ADDR`가 설정되면 agent는 child process 대신 기존 localhost TCP bridge에 직접 연결합니다.
- `WORKSPACE_ADAPTER_MODE=cursor_agent_cli`가 설정되면 bridge 대신 `CursorAgentCLIAdapter`를 사용합니다.
- `CursorBridgeAdapter`(`internal/agent/cursor_bridge_adapter.go`)가 `name`, `capabilities`, `getContext`, `submitTask`, `getPatch`, `applyPatch`, `runProfile`, `getRunResult`, `openLocation` RPC를 담당합니다.
- `CursorAgentCLIAdapter`(`internal/agent/cursor_agent_cli_adapter.go`)는 현재 workspace 상태를 temp worktree에 동기화하고, 네이티브 CLI 또는 WSL distro 내부의 실제 binary를 직접 실행해 diff만 회수합니다. ignored 파일은 기본 제외하고 `CURSOR_AGENT_SYNC_IGNORED_JSON` allowlist와 일치하는 항목만 snapshot에 포함합니다.
- `GET /v1/agent/runtime/adapter`는 현재 adapter 이름, capability, mode, workspace root, binary 경로 같은 smoke 진단 정보를 노출합니다.
- `scripts/cursor_agent_smoke.ps1`는 temp repo를 만들고 `PROMPT_SUBMIT -> PATCH_APPLY -> RUN_PROFILE` smoke를 자동 수행합니다. 현재 Windows + WSL + login 완료 환경에서 실제 smoke proof를 확보했습니다.
- `scripts/extension_host_smoke.ps1`는 이미 떠 있는 extension host TCP bridge에 대해 bridge preflight 후 agent mock smoke를 수행합니다.
- `scripts/gui_extension_host_smoke.ps1`는 실제 Cursor GUI extension host를 dev extension 모드로 띄우고, built-in cursor-agent provider 경로를 end-to-end로 검증합니다.
- `npm --prefix extensions/vibedeck-bridge run smoke:provider`는 fake cursor-agent를 사용해 built-in command provider와 command bridge 경로를 결정적으로 검증합니다. ignored allowlist에 포함한 파일이 snapshot에 반영되는지도 함께 확인합니다.
- `npm --prefix extensions/vibedeck-bridge run smoke:extension`는 fake VS Code host + fake cursor-agent를 사용해 `extension.ts -> controller -> TCP bridge -> JSON-RPC` 활성화 경로를 검증합니다. extension 설정의 `vibedeckBridge.cursorAgent.syncIgnoredPaths`도 이 smoke에서 같이 검증합니다.

## 프로토콜 전략

모든 제어 경로 메시지는 공통 Envelope를 사용합니다.

- HTTP/P2P control handler는 message type별 timeout budget을 사용합니다. `PROMPT_SUBMIT`/`RUN_PROFILE`는 5분, `PATCH_APPLY`는 30초, 나머지는 5초입니다.

- `sid`: session id
- `rid`: request id
- `seq`: sequence number
- `ts`: unix ms timestamp
- `type`: message type
- `payload`: JSON payload

제어 메시지는 반드시 ACK를 보장하고, 터미널 스트림은 best-effort로 처리합니다.

## ACK 추적 정책

- `AckTracker`가 transport별 pending ACK를 추적합니다.
- HTTP 응답은 observe-only pending ACK로 등록하고, 상태/타임아웃 관찰만 수행합니다.
- P2P(DataChannel) 응답은 retryable pending ACK로 등록하고, 원본 envelope와 마지막 전송 시각을 함께 보존합니다.
- `P2PSessionManager`가 backoff 간격으로 재전송을 수행하고, 최대 재시도 초과 또는 재전송 실패 시 세션 상태를 `reconnecting`으로 전이합니다.
- 세션 종료 시 transport별 pending ACK를 정리해 stale 상태를 남기지 않습니다.
- `AckTracker.Metrics()`가 pending count/max queue depth, transport split, ack RTT(last/avg/max), retry/expired/exhausted 집계를 제공하고 HTTP `/v1/agent/runtime/metrics`로 노출됩니다.

## 시그널링 레이어(WebRTC 연동)

### 메시지 방향성

- PC -> Mobile: `SIGNAL_OFFER`, `SIGNAL_ICE`
- Mobile -> PC: `SIGNAL_ANSWER`, `SIGNAL_ICE`
- Server -> 양쪽: `SIGNAL_READY`

### 검증 정책

- `sid`와 연결된 세션 ID가 반드시 일치해야 함
- `offer/answer/ice` payload 필수 필드 검증
- 역할(role)에 맞지 않는 시그널링 타입은 즉시 거절

### 큐잉 정책

- 상대 피어가 아직 없을 때 시그널링 메시지를 세션 큐에 임시 보관
- 피어가 연결되면 queued signaling 메시지를 먼저 재전달
- 큐 길이는 상한을 두고 초과 시 oldest 항목 제거

## WebRTC 데이터채널 스켈레톤 (`internal/webrtc`)

- `SidePC`는 offerer 역할로 data channel을 선생성
- `SideMobile`는 offer 수신 후 answer 생성
- `SignalBridge`가 signaling envelope과 peer 동작을 결합
- `ControlRouter`가 HTTP/P2P 공통 제어 메시지 해석과 `CMD_ACK` 소거를 담당합니다.
- HTTP server와 `P2PSessionManager`가 transport 특성에 맞는 ACK 등록/재전송 정책을 적용합니다.
