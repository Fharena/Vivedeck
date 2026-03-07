# 크리티컬 이슈 및 학습 노트

개발 중 발생하는 장애/고위험 이슈를 기록합니다.

## 현재 상태

- 런타임 크리티컬 버그는 아직 식별되지 않았습니다.

## 현재 환경 리스크(관측)

### 2026-03-06 / TOOLCHAIN-002 / Flutter 전역 PATH 미설정

- 증상: 시스템 전역 `flutter` 명령 인식 실패
- 영향: 팀원/CI 환경마다 Flutter 실행 경로 불일치 가능성
- 즉시 대응:
  - 로컬 SDK를 `tools/flutter`에 설치
  - `tools/flutter/bin/flutter.bat` 기준으로 `pub get`, `analyze`, `test` 검증 수행
- 영구 대응:
  - Flutter SDK 전역 PATH 표준화 또는 `fvm` 도입
  - CI에 모바일 smoke test(`flutter pub get && flutter test`) 추가
- 학습 포인트:
  - 도구체인은 "설치 여부"뿐 아니라 "실행 경로 표준화"까지 완료돼야 팀 생산성이 안정화됨

### 2026-03-07 / TOOLCHAIN-003 / 경로(공백/한글) + native assets 훅 충돌

- 증상: `flutter test` 실행 시 `objective_c` native assets 훅이 `'C:\Users\99yoo\OneDrive\바탕' is not recognized` 오류로 실패
- 영향: 모바일 테스트 루프가 환경 경로에 따라 불안정해질 수 있음
- 즉시 대응:
  - `google_fonts` 의존성 제거로 transitive native assets 훅 제거
  - 의존성 단순화 후 `flutter analyze`, `flutter test` 재검증 통과
- 영구 대응:
  - 경로 공백/로케일 이슈를 포함한 Flutter CI 매트릭스 테스트 추가
  - 필요 시 폰트/플러그인 의존성은 native assets 영향 범위를 기준으로 선택
- 학습 포인트:
  - 모바일 툴체인 이슈는 기능 코드보다 실행 환경(경로/권한/훅) 검증을 먼저 자동화해야 재발을 줄일 수 있음

## 런타임 리스크(설계)

### 2026-03-06 / RUNTIME-001 / ACK 만료 오탐 가능성

- 증상 가능성: 네트워크 지연이 큰 환경에서 정상 응답도 timeout 처리될 수 있음
- 영향: 불필요한 `RECONNECTING` 상태 전환 증가
- 즉시 대응: 현재는 만료 항목 조회 시에만 reconnect 전환
- 영구 대응:
  - 메시지 타입별 timeout 차등화
  - 재전송 횟수/지수 백오프 도입
  - 실제 RTT 관측값 기반 동적 timeout 조정
- 학습 포인트:
  - ACK 시스템은 "엄격함"보다 "관측 기반 튜닝"이 중요함

### 2026-03-06 / RUNTIME-002 / Inbound ACK 미처리 시 pending 누적

- 증상 가능성: 모바일에서 `CMD_ACK`를 보내도 서버가 미처리하면 pending ACK가 계속 쌓임
- 영향: 만료 감지 오탐 증가, reconnect 과다
- 즉시 대응: `/v1/agent/envelope`에서 `CMD_ACK`를 분기 처리해 pending 제거
- 영구 대응:
  - ACK 중복/순서 뒤바뀜 시나리오 통합 테스트 추가
  - 메시지 타입별 ACK 필요 여부를 명시한 정책 테이블 도입
- 학습 포인트:
  - ACK 추적은 등록/만료뿐 아니라 "수신 소거 경로"가 있어야 완결됨

### 2026-03-06 / SIGNAL-001 / 시그널 큐 포화 시 후보 누락 가능성

- 증상 가능성: 상대 피어 지연 접속 + ICE 폭주 시 oldest candidate가 큐에서 탈락
- 영향: NAT 환경에 따라 P2P 연결 성공률 저하 가능
- 즉시 대응: 큐 길이 상한을 두고 overflow 시 oldest 제거(서버 메모리 보호)
- 영구 대응:
  - candidate 우선순위 룰 적용(host/srflx/relay)
  - 큐 정책을 세션 품질 지표 기반으로 동적 조정
  - fallback 판단 로직(빠른 relay 전환)과 함께 튜닝
- 학습 포인트:
  - 시그널링 설계는 완전보존보다 연결 성공률/지연의 균형이 핵심

### 2026-03-06 / WEBRTC-001 / 로컬 통합 테스트 타이밍 플래키 가능성

- 증상 가능성: CI 환경 성능 편차에 따라 connected/open 이벤트 타임아웃 발생
- 영향: 실제 코드 이상 없이 테스트가 실패할 수 있음
- 즉시 대응: 상태/채널 open 대기 시간을 명시하고, ICE 후보 전달은 best-effort 로그 처리
- 영구 대응:
  - 테스트 전용 구성값(타임아웃, 버퍼) 분리
  - 재시도 가능한 헬퍼 및 flaky detector 도입
- 학습 포인트:
  - WebRTC 테스트는 기능 검증과 안정성 검증을 분리해야 유지보수가 쉬움

### 2026-03-06 / WEBRTC-002 / SIGNAL_READY 중복 수신 시 offer 중복 생성 리스크

- 증상 가능성: 재연결 또는 중복 이벤트에서 PC가 offer를 여러 번 생성하려 시도
- 영향: 협상 충돌, 연결 실패 확률 증가
- 즉시 대응: bridge에서 `offerStarted` 원자 플래그로 1회만 offer 시작
- 영구 대응:
  - renegotiation 전용 상태머신 분리
  - session version과 glare handling 정책 도입
- 학습 포인트:
  - 초기 협상 제어와 재협상 제어는 별도 설계가 필요함

### 2026-03-06 / AGENT-P2P-001 / WS read/write 루프 오류 시 자동 재시작 미구현

- 증상 가능성: 일시 네트워크 단절 후 세션이 `RECONNECTING`에 머물고 자동복구 실패
- 영향: 수동 `p2p/start` 재호출 필요
- 즉시 대응: read/write/bridge 오류 시 상태를 `RECONNECTING`으로 전환하고 오류 메시지 기록
- 영구 대응:
  - 지수 백오프 기반 자동 재연결 정책 추가
  - 실패 횟수 제한 + fallback relay 전환 정책 결합
- 학습 포인트:
  - 상태 전이 표시와 실제 복구 실행은 분리해서 설계해야 함

### 2026-03-06 / AGENT-P2P-002 / DataChannel 제어 메시지 burst 시 단일 루프 병목 가능성

- 증상 가능성: 모바일에서 제어 요청을 매우 빠르게 연속 전송하면 처리 지연이 누적될 수 있음
- 영향: `PROMPT_ACK`/`PATCH_READY`/`RUN_RESULT` 응답 지연, ACK timeout 오탐 위험 증가
- 즉시 대응: 현재는 단일 소비 루프 + bounded channel로 안정성 우선, 파싱 실패는 연결을 끊지 않고 에러 기록
- 영구 대응:
  - 제어 메시지 worker pool + 순서 보장 정책 분리
  - 메시지 타입별 우선순위 큐 도입(`CMD_ACK` 고우선)
  - queue depth/latency 메트릭 기반 autoscaling 임계치 정의
- 학습 포인트:
  - 제어 경로는 처리량보다 순서/신뢰성 보장이 먼저이며, 병렬화는 명시적 순서 정책과 함께 도입해야 함

## 해결 방식 학습 체크리스트

1. 문제 재현 명령을 문서화했는가?
2. 임시 우회와 영구 해결을 분리해 기록했는가?
3. 해결 후 회귀 테스트를 추가했는가?
4. 설계 원칙(ACK 보장, backpressure, adapter 분리) 위반이 없는가?

## 사고 기록 템플릿

- 날짜:
- 컴포넌트:
- 증상:
- 근본 원인:
- 즉시 완화 조치:
- 영구 해결 방식:
- 추가한 회귀 테스트:
- 학습 포인트:



