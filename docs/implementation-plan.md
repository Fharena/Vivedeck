# 구현 계획

이 문서는 VibeDeck MVP 구현 순서와 완료 기준을 관리합니다.

## 기본 범위

- 모바일 앱은 AI 코딩 루프를 제어한다.
- PC 에이전트가 실행과 워크스페이스 변경을 담당한다.
- 전송은 P2P 우선, Relay 폴백을 필수로 둔다.
- 핵심 UX는 원격 편집이 아니라 패치/헝크 승인이다.

## 단계별 계획

### Phase 1: 연결 베이스라인

상태: `완료(베이스라인)`

산출물:

- 페어링 코드 생성/클레임 API
- 세션 생명주기 모델
- 시그널링 교환 채널
- Relay 폴백 서버 골격

완료 기준:

- 모바일/PC 피어가 같은 세션에 참여 가능
- 세션 상태가 signaling/relay 모드 전환 가능

### Phase 2: Prompt -> Patch -> Apply

상태: `베이스라인 완료`

산출물:

- 프롬프트 제출 ACK 플로우
- 패치 번들 정규화(`files[]`, `hunks[]`, `summary`)
- 전체/부분 적용 오케스트레이션

완료 기준:

- 모바일 프롬프트 요청이 검토 가능한 패치 번들로 반환
- 패치 적용 상태(`success|partial|conflict|failed`) 반환

### Phase 3: Run -> Result

상태: `베이스라인 완료`

산출물:

- 실행 프로파일 로더(`test_last`, `test_all`, `build`, `dev`)
- PC 에이전트 실행 디스패치
- 상위 에러/요약/excerpt 결과 모델

완료 기준:

- 모바일에서 프로파일 실행 후 요약 결과 수신 가능

### Phase 4: 런타임 신뢰성 강화

상태: `완료(베이스라인)`

완료된 산출물:

- 연결 상태머신(`PAIRING`~`CLOSED`)
- Outbound ACK 등록/만료 추적
- Inbound `CMD_ACK` 처리 및 pending 제거
- 런타임 상태/ACK 조회 엔드포인트

남은 작업:

- ACK 재전송 정책 구현
- RTT 기반 timeout 튜닝
- 통합 장애 시나리오 자동화

### Phase 5: 어댑터/시그널링 고도화

상태: `진행 중`

완료된 산출물:

- TypeScript Cursor 브리지 계약 + Mock 구현
- 시그널링 기본 offer/answer/ice 라우팅
- 시그널링 방향성 검증(PC: OFFER/ICE, Mobile: ANSWER/ICE)
- 상대 미접속 시 시그널 메시지 큐잉/재전달
- `SIGNAL_READY` 이벤트 추가

남은 작업:

- Cursor 실제 Extension API 연동
- WebRTC 피어 생성/SDP/ICE 실제 핸들러 결합

## 다음 작업 우선순위

1. Go 런타임 설치 후 `go test ./...`와 실제 프로세스 실행 검증
2. 모바일/PC 양쪽 WebRTC 클라이언트 스켈레톤 연결
3. Flutter Prompt/Review/Status 화면 베이스라인 추가
4. MockCursorBridge를 실제 Cursor Extension API로 교체

## 커밋 전략

기능 단위의 큰 커밋으로 진행:

1. `chore(repo): 모노레포 구조 및 공통 계약 초기화`
2. `feat(signaling,relay): 페어링/시그널링/릴레이 베이스라인`
3. `feat(agent): prompt-patch-run 오케스트레이션 베이스라인`
4. `feat(cursor-bridge): TypeScript WorkspaceAdapter 계약 추가`
5. `feat(runtime): 연결 상태머신/ACK 추적기 추가`
6. `feat(signaling): webrtc signaling 검증/큐잉 강화`
7. `docs(ops): 크리티컬 이슈/트러블슈팅 학습 노트`
