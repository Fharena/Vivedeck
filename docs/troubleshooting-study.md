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

## 7. SignalBridge 처리 누락/순서 문제

관찰 포인트:

- `SIGNAL_READY`가 왔는데 offer가 생성되지 않는지
- answer 적용 전에 ice만 먼저 와서 오류가 나는지
- bridge errors 채널에 sid mismatch/unsupported type이 누적되는지

학습 순서:

1. bridge의 inbound/outbound envelope를 `sid,type,rid` 기준으로 트레이싱한다.
2. `SIGNAL_READY -> OFFER -> ANSWER -> ICE` 최소 순서가 만족되는지 확인한다.
3. offer 중복 생성 방지 플래그(`offerStarted`) 동작을 검증한다.
4. signaling 서버 ACK와 bridge 내부 에러를 분리해 원인 범위를 좁힌다.

## 8. Flutter Toolchain 경로 불일치

관찰 포인트:

- 전역 `flutter` 명령 인식 여부와 로컬 SDK 경로(`tools/flutter/bin/flutter.bat`) 사용 여부
- `flutter pub get`에서 SDK 버전 제약 충돌 여부
- 로컬 실행(`flutter run`)과 CI(`flutter test`) 결과 차이

학습 순서:

1. `flutter --version` 또는 `tools/flutter/bin/flutter.bat --version`으로 SDK 경로를 확인한다.
2. `dart --version`과 `pubspec.yaml`의 SDK 범위가 일치하는지 검증한다.
3. 최소 smoke test(`flutter pub get`, `flutter test`)를 자동화한다.
4. 도구체인 이슈가 해결되기 전에는 UI 계약/상태 모델과 네트워크 연동 코드를 분리한다.

## 9. Flutter native assets 훅 경로 충돌

관찰 포인트:

- `flutter test`에서 특정 플러그인 훅(`objective_c` 등) 컴파일 단계 실패 여부
- 오류 메시지에 경로 일부(`C:\Users\...\바탕`) 잘림/분리 실행 징후가 있는지
- 동일 코드가 경로가 단순한 환경에서는 통과하는지

학습 순서:

1. 실패 로그에서 훅 패키지와 호출된 명령줄을 먼저 식별한다.
2. transitive dependency 트리(`flutter pub deps`)에서 native assets 유입 경로를 확인한다.
3. 기능 영향이 낮은 의존성부터 제거/대체해 재현 여부를 비교한다.
4. 재현 가능한 최소 케이스를 CI에 넣어 경로/로케일 회귀를 막는다.

## 실전 기록 규칙

- 장애 하나당 티켓/문서 항목 하나를 유지한다.
- "증상 -> 재현 -> 원인 -> 해결 -> 회귀 테스트" 순서로 기록한다.
- 같은 장애가 2회 이상 반복되면 자동화된 검증을 추가한다.






