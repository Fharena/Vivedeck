# 구현 계획

이 문서는 VibeDeck MVP 구현 순서와 완료 기준을 관리합니다.

## 기본 범위

- 모바일 앱은 AI 코딩 루프를 제어한다.
- PC 에이전트가 실행과 워크스페이스 변경을 담당한다.
- 전송은 P2P 우선, Relay 폴백을 필수로 둔다.
- 핵심 UX는 원격 편집이 아니라 패치/헝크 승인이다.

## 단계별 계획

### Phase 1: 연결 베이스라인

산출물:

- 페어링 코드 생성/클레임 API
- 세션 생명주기 모델
- 시그널링 교환 채널
- Relay 폴백 서버 골격

완료 기준:

- 모바일/PC 피어가 같은 세션에 참여 가능
- 세션 상태가 signaling/relay 모드 전환 가능

### Phase 2: Prompt -> Patch -> Apply

산출물:

- 프롬프트 제출 ACK 플로우
- 패치 번들 정규화(`files[]`, `hunks[]`, `summary`)
- 전체/부분 적용 오케스트레이션

완료 기준:

- 모바일 프롬프트 요청이 검토 가능한 패치 번들로 반환
- 패치 적용 상태(`success|partial|conflict|failed`) 반환

### Phase 3: Run -> Result

산출물:

- 실행 프로파일 로더(`test_last`, `test_all`, `build`, `dev`)
- PC 에이전트 실행 디스패치
- 상위 에러/요약/excerpt 결과 모델

완료 기준:

- 모바일에서 프로파일 실행 후 요약 결과 수신 가능

### Phase 4: 어댑터 고도화

산출물:

- TypeScript Cursor 브리지 계약
- Capability 협상 구조
- 파일/라인 열기 액션 경로

완료 기준:

- PC 에이전트가 어댑터 계약 기반으로 일관되게 호출

### Phase 5: 안정화/복구

산출물:

- 제어 경로 ACK 보장
- 터미널 스트림 백프레셔 정책
- 재연결 동작 정리

완료 기준:

- 로그 과부하 상황에서도 제어 메시지가 안정적으로 처리

## 커밋 전략

기능 단위의 큰 커밋으로 진행:

1. `chore(repo): 모노레포 구조 및 공통 계약 초기화`
2. `feat(signaling,relay): 페어링/시그널링/릴레이 베이스라인`
3. `feat(agent): prompt-patch-run 오케스트레이션 베이스라인`
4. `feat(cursor-bridge): TypeScript WorkspaceAdapter 계약 추가`
5. `docs(ops): 크리티컬 이슈/트러블슈팅 학습 노트`
