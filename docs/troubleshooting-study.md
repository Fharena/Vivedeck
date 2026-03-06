# 트러블슈팅 학습 가이드

이 문서는 크리티컬 이슈가 발생했을 때 원인 파악과 해결 방식을 빠르게 학습하기 위한 체크리스트입니다.

## 1. ACK 누락/지연

관찰 포인트:

- `CMD_ACK`가 특정 타입에서만 누락되는지
- `rid` 매칭이 깨지는지
- 재시도 로직이 중복 실행을 유발하는지

학습 순서:

1. 문제 메시지의 `sid/rid/seq`를 수집한다.
2. 서버/에이전트 로그에서 동일 `rid` 흐름을 추적한다.
3. ACK 생성/전송 경로를 분리해 재현한다.
4. 누락 타입에 대한 회귀 테스트를 작성한다.

## 2. Relay 백프레셔

관찰 포인트:

- `TERM` 대량 유입 시 제어 경로(`PATCH_*`, `RUN_*`) 지연 여부
- `TERM_SUMMARY` 발행 여부

학습 순서:

1. 터미널 스팸 입력으로 큐 포화 상황을 재현한다.
2. control 메시지 timeout 발생 빈도를 측정한다.
3. 큐 크기, timeout, summary 기준값을 조정해 비교한다.
4. 목표: control path 무손실 + 터미널 best-effort 유지.

## 3. Patch 부분 적용 실패

관찰 포인트:

- 선택한 헝크 ID와 실제 patch 데이터 매칭 여부
- Adapter가 partial apply capability를 올바르게 노출하는지

학습 순서:

1. 동일 patch에서 `all`/`selected` 결과를 비교한다.
2. hunk 선택 없는 `selected` 요청을 강제로 재현한다.
3. `partial/conflict/failed` 상태 기준을 명확히 테스트한다.

## 4. 실행 결과 파싱 누락

관찰 포인트:

- topErrors에 path/line이 비는 패턴
- summary와 raw excerpt의 불일치

학습 순서:

1. 실패 로그 샘플을 유형별로 축적한다.
2. 정규식/파서 룰을 케이스별로 테스트한다.
3. 파싱 실패 시 fallback summary 규칙을 적용한다.

## 5. WebRTC 시그널링 불일치

관찰 포인트:

- `SIGNAL_OFFER`/`SIGNAL_ANSWER` 방향이 뒤바뀌는지
- `sid` mismatch로 거절되는 빈도
- 상대 미접속 구간에서 ICE 누락이 발생하는지

학습 순서:

1. 시그널링 로그를 `sid` 기준으로 묶는다.
2. offer/answer/ice payload 최소 필드(sdp/candidate) 유효성을 확인한다.
3. 큐잉된 메시지가 peer 연결 시 재전달되는지 검증한다.
4. `SIGNAL_READY` 이벤트 이후 협상 시작 타이밍을 통일한다.

## 6. DataChannel open 지연/실패

관찰 포인트:

- `connected` 상태인데 data channel open 이벤트가 지연되는지
- 메시지 송신 시 `data channel is not open` 에러 비율

학습 순서:

1. `WaitForState(connected)`와 `WaitDataChannelOpen()`의 시점 차이를 기록한다.
2. ICE candidate 전달 로그를 양쪽(peer A/B)에서 비교한다.
3. open timeout 값을 환경별(CI/로컬)로 분리해 검증한다.

## 실전 기록 규칙

- 장애 하나당 티켓/문서 항목 하나를 유지한다.
- "증상 -> 재현 -> 원인 -> 해결 -> 회귀 테스트" 순서로 기록한다.
- 같은 장애가 2회 이상 반복되면 자동화된 검증을 추가한다.
