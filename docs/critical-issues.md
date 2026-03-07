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

### 2026-03-07 / TOOLCHAIN-004 / Git safe.directory 소유권 검증 차단

- 증상: CodexSandbox 사용자로 실행 시 `detected dubious ownership` 오류로 `git status` 등 기본 Git 명령 실패
- 영향: 브랜치 상태 확인/커밋/머지 후속 작업이 즉시 중단되어 개발 흐름이 끊김
- 즉시 대응:
  - `git config --global --add safe.directory <repo-path>`로 신뢰 디렉터리 등록
  - 등록 직후 `git status -sb`로 접근 정상 여부 확인
- 영구 대응:
  - 팀 자동화 에이전트/CI 실행 계정이 바뀌는 환경에서는 초기 bootstrap 스크립트에 safe.directory 등록 단계 포함
  - OneDrive/공유 폴더 사용 시 소유권 변경 가능성을 운영 가이드에 명시
- 학습 포인트:
  - 권한/소유권 이슈는 코드와 무관하게 전체 개발 사이클을 멈추므로, 재현 즉시 "즉시 대응 + 영구 대응"을 분리해 기록해야 함

### 2026-03-07 / TOOLCHAIN-005 / flutter_webrtc 도입 후 native assets 훅 재발

- 증상: `flutter_webrtc` 추가 후 `flutter test`가 다시 `objective_c` 훅에서 `'C:\Users\99yoo\OneDrive\바탕' is not recognized`로 실패
- 영향: 모바일 WebRTC 기능 추가 직후 기본 테스트 루프가 다시 중단됨
- 즉시 대응:
  - 경로 공백/한글 우회용 스크립트 `mobile/flutter_app/scripts/flutter_test_safe.ps1` 추가
  - 스크립트에서 `subst V:` 임시 매핑 후 `flutter test` 실행, 완료 후 자동 해제
- 영구 대응:
  - CI/로컬 공통으로 공백 없는 작업 경로 표준화
  - native assets 훅을 사용하는 의존성 도입 시 사전 경로 호환성 체크를 PR 체크리스트에 추가
- 학습 포인트:
  - 기능 의존성 추가는 런타임 기능뿐 아니라 테스트 실행 파이프라인(native assets 포함) 영향까지 함께 검증해야 함

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

### 2026-03-08 / CURSOR-EXT-001 / 공식 Cursor task command 계약 부재

- 증상: extension host에 localhost bridge는 올릴 수 있지만, 저장소 안에 실제 Cursor AI task/patch/run command ID 매핑은 없음
- 영향: `command` mode를 바로 켜도 기본 `vibedeck.*` command가 등록되지 않은 환경에서는 bridge가 시작되지 않음
- 즉시 대응:
  - `extensions/vibedeck-bridge`에 command readiness 검증(`vibedeckBridge.validateCommands`) 추가
  - 필수 command 누락 시 시작을 막고, optional command는 경고만 표시
  - `vibedeckBridge.copyAgentEnv`로 agent 연결 환경변수 복사 경로 제공
- 영구 대응:
  - 공식 Cursor extension API/command 계약이 확인되면 해당 command ID 또는 API 호출로 직접 매핑
  - 공식 계약이 없으면 CLI/MCP 기반 대체 adapter 경로 설계
  - extension host smoke/E2E 자동화로 실제 사용 가능 상태를 지속 검증
- 학습 포인트:
  - editor bridge는 "브리지를 띄우는 것"과 "실제 AI 작업 명령에 연결하는 것"을 분리해서 관리해야 구현 착시를 줄일 수 있음

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
