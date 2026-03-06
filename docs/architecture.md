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

## 신뢰성 규칙

- 제어 경로(CMD/PATCH/RUN)를 터미널 스트림보다 우선 처리
- 과부하시 터미널 라인을 드롭하고 synthetic summary 이벤트 전송
- 전송 모드가 바뀌어도 동일 세션 의미론 유지

## 보안 베이스라인

- 페어링 코드는 짧은 TTL 적용
- 클레임 성공 후 디바이스 키 발급
- MVP에서 HIGH 권한 동작은 기본 비활성화
- 서버에는 최소 세션 메타데이터만 저장
