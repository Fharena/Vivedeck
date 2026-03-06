# 아키텍처

## 시스템 구성 요소

- 모바일 앱(Flutter): Prompt/Review/Status 화면 제공
- PC 에이전트(Go): 잡 오케스트레이션, 패치 수명주기, 실행 프로파일, 전송 바인딩
- Cursor 브리지(TypeScript): 컨텍스트 조회, 패치 적용, 파일/라인 열기
- Signaling 서버(Go): 페어링 및 WebRTC 시그널링 부트스트랩
- Relay 서버(Go): 폴백 이벤트 라우팅 + 백프레셔 정책

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
- 모바일이 보낸 `CMD_ACK`를 수신하면 해당 pending을 즉시 제거
- TTL 초과 항목을 `Expired()`로 회수
- 만료 ACK가 발생하면 연결 상태를 `RECONNECTING`으로 전환해 복구 흐름 시작

### 운영 엔드포인트

- `GET /v1/agent/runtime/state`: 현재 상태 + 상태 히스토리 조회
- `POST /v1/agent/runtime/state`: 상태 전환 액션 트리거
- `GET /v1/agent/runtime/acks/pending`: 현재 pending ACK 목록 조회
- `GET /v1/agent/runtime/acks/expired`: 만료 ACK 확인

## 신뢰성 규칙

- 제어 경로(CMD/PATCH/RUN)를 터미널 스트림보다 우선 처리
- 과부하시 터미널 라인을 드롭하고 `TERM_SUMMARY` 이벤트 전송
- 전송 모드가 바뀌어도 동일 세션 의미론 유지

## 보안 베이스라인

- 페어링 코드는 짧은 TTL 적용
- 클레임 성공 후 디바이스 키 발급
- MVP에서 HIGH 권한 동작은 기본 비활성화
- 서버에는 최소 세션 메타데이터만 저장
