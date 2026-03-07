# 아키텍처

## 시스템 구성 요소

- 모바일 앱(Flutter): Prompt/Review/Status 화면 제공
- PC 에이전트(Go): 잡 오케스트레이션, 패치 수명주기, 실행 프로파일, 전송 바인딩
- Cursor 브리지(TypeScript): 컨텍스트 조회, 패치 적용, 파일/라인 열기
- Signaling 서버(Go): 페어링 및 WebRTC 시그널링 부트스트랩
- Relay 서버(Go): 폴백 이벤트 라우팅 + 백프레셔 정책

## Flutter UI 베이스라인 (`mobile/flutter_app`)

- `PromptScreen`: 프롬프트 입력, 템플릿 선택, context 옵션 토글
- `ReviewScreen`: 파일/헝크 목록 검토와 전체/선택 적용 액션
- `StatusScreen`: 연결 상태, pending ACK, 히스토리 표시
- `StatusScreen`에서 direct signaling(pairing claim + WS 연결) 스켈레톤 제어/로그 확인 가능

## Flutter API 연동 레이어 (`mobile/flutter_app/lib/state/app_controller.dart`)

- `AppController`가 화면 상태와 agent API 호출을 단일 진입점으로 관리
- `AgentApi`(`mobile/flutter_app/lib/services/agent_api.dart`)가 HTTP 요청/오류 처리 담당
- 제어 메시지 전송 흐름:
  1. `PROMPT_SUBMIT` / `PATCH_APPLY` / `RUN_PROFILE` envelope 전송
  2. 응답 envelope(`responses[]`) 파싱
  3. non-ACK 응답 RID에 대해 `CMD_ACK` 자동 회신
- 상태 화면은 runtime/p2p/ack 조회 API를 주기적 갱신으로 표시
- direct signaling 레이어:
  - `SignalingApi`(`mobile/flutter_app/lib/services/signaling_api.dart`)가 pairing claim + WS URI 생성 담당
  - `MobileDirectSignalingSession`(`mobile/flutter_app/lib/services/mobile_direct_signaling_session.dart`)가 WS 연결/이벤트/수신 envelope 스트림 관리
  - 현재는 `SIGNAL_OFFER`/`SIGNAL_ICE` 수신 로깅까지 구현되어 있으며, 실제 모바일 peer 협상은 후속 단계에서 연동

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

## 프로토콜 전략

모든 제어 경로 메시지는 공통 Envelope를 사용합니다.

- `sid`: session id
- `rid`: request id
- `seq`: sequence number
- `ts`: unix ms timestamp
- `type`: message type
- `payload`: JSON payload

제어 메시지는 반드시 ACK를 보장하고, 터미널 스트림은 best-effort로 처리합니다.

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
- ICE candidate 채널을 통해 트리클 후보를 교환
- 연결/메시지 상태 이벤트를 채널로 노출

핵심 메서드:

- `CreateOffer()`
- `ApplyOfferAndCreateAnswer()`
- `ApplyAnswer()`
- `AddRemoteICECandidate()`
- `WaitForState()` / `WaitDataChannelOpen()`
- `Send()`

## SignalBridge 런타임 (`internal/webrtc/bridge.go`)

- signaling envelope를 WebRTC peer 동작으로 변환하는 어댑터 계층
- PC bridge는 `SIGNAL_READY` 이후 offer를 생성하고 송신
- Mobile bridge는 offer를 받아 answer를 생성해 송신
- 양쪽 모두 `SIGNAL_ICE`를 remote candidate로 적용

핵심 메서드:

- `Run(ctx)`
- `InboundEnvelope()`
- `ProcessEnvelope()`
- `StartOffer()`
- `Outbound()` / `Errors()`

## Agent P2P 오케스트레이터 (`internal/agent/p2p_session.go`)

- signaling REST/WS와 SignalBridge를 결합해 PC 세션을 관리
- `/v1/pairings` 호출로 pairing code/session/deviceKey 발급
- PC role WS 연결 후 bridge 런타임 시작
- `runtime.StateManager`와 연결해 상태를 동기화
- WebRTC DataChannel 메시지를 공통 제어 라우터로 전달

오케스트레이터 흐름:

1. `Start()` -> pairing 생성 -> WS 연결 -> `SIGNALING`
2. `SIGNAL_READY` 수신 후 offer 송신 -> `P2P_CONNECTING`
3. answer 적용 및 peer connected -> `P2P_CONNECTED`
4. DataChannel inbound envelope -> orchestrator 처리 -> 응답 envelope DataChannel 송신
5. WS/bridge/peer 오류 -> `RECONNECTING`
6. `Stop()` -> peer/ws 정리 -> `CLOSED`

## ControlRouter (`internal/agent/control_router.go`)

- HTTP(`/v1/agent/envelope`)와 P2P(DataChannel) 경로가 같은 envelope 처리 규칙을 공유
- 일반 제어 메시지:
  - `Orchestrator.HandleEnvelope()` 실행
  - 응답 envelope를 반환하고 non-ACK 응답은 `AckTracker`에 등록
- `CMD_ACK` 메시지:
  - `AckTracker.Ack(requestRid)`로 pending을 소거
  - 경로별 반환 형식만 다르고 내부 동작은 동일

## 런타임 신뢰성 레이어

### 연결 상태머신 (`internal/runtime/state_manager.go`)

지원 상태:

- `PAIRING`
- `SIGNALING`
- `P2P_CONNECTING`
- `P2P_CONNECTED`
- `RELAY_CONNECTED`
- `RECONNECTING`
- `CLOSED`

정책:

- `BeginP2P()` 후 타임아웃이 나면 자동으로 Relay fallback
- P2P 성공 시 타이머 즉시 정리
- 상태 변경 히스토리를 밀리초 단위로 보관

### ACK 추적기 (`internal/runtime/ack_tracker.go`)

정책:

- 에이전트가 전송한 제어 응답(`PROMPT_ACK`, `PATCH_READY`, `RUN_RESULT` 등)을 pending ACK로 등록
- 모바일가 보낸 `CMD_ACK`를 수신하면 해당 pending을 즉시 제거
- TTL 초과 항목을 `Expired()`로 회수
- 만료 ACK가 발생하면 연결 상태를 `RECONNECTING`으로 전환해 복구 흐름 시작

### 운영 엔드포인트

- `GET /v1/agent/runtime/state`: 현재 상태 + 상태 히스토리 조회
- `POST /v1/agent/runtime/state`: 상태 전환 액션 트리거
- `GET /v1/agent/runtime/acks/pending`: 현재 pending ACK 목록 조회
- `GET /v1/agent/runtime/acks/expired`: 만료 ACK 확인
- `POST /v1/agent/p2p/start`: P2P 세션 시작
- `GET /v1/agent/p2p/status`: P2P 세션 상태 조회
- `POST /v1/agent/p2p/stop`: P2P 세션 종료

## 신뢰성 규칙

- 제어 경로(CMD/PATCH/RUN)를 터미널 스트림보다 우선 처리
- 과부하시 터미널 라인을 드롭하고 `TERM_SUMMARY` 이벤트 전송
- 전송 모드가 바뀌어도 동일 세션 의미론 유지

## 보안 베이스라인

- 페어링 코드는 짧은 TTL 적용
- 클레임 성공 후 디바이스 키 발급
- MVP에서 HIGH 권한 동작은 기본 비활성화
- 서버에는 최소 세션 메타데이터만 저장





