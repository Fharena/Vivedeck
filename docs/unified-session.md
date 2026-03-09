# Unified Session 계획

이 문서는 VibeDeck을 `모바일 + Cursor가 하나의 도구처럼 움직이는 제품`으로 끌어올리기 위한 다음 단계 설계를 정리합니다.

## 목표

- 모바일과 Cursor가 같은 대화 내용, 같은 세션, 같은 작업 상태를 본다.
- 두 클라이언트가 같은 세션의 `움직임`까지 공유한다.
- 사용자는 "모바일 원격 컨트롤 앱"이 아니라 "한 세션의 다른 화면"처럼 느껴야 한다.
- Cursor provider 완성 전까지는 provider 확장보다 unified session 완성도를 우선한다.

## 현재 상태와 차이

현재 구조는 `shared thread`와 `shared patch/run history`는 이미 갖추고 있다.
하지만 아래 요소는 아직 분리돼 있다.

- 채팅/패치/실행 기록은 공유되지만 live draft, typing, focus, selection, 현재 보고 있는 파일은 공유되지 않는다.
- 모바일 UI는 `대화/검토/상태` 3분할 탭이라 세션 중심 제품 경험이 약하다.
- Cursor extension panel도 세션 participant라기보다 thread viewer/controller에 가깝다.
- thread history는 영속화되지만 live job/task는 재시작 뒤 그대로 이어지지 않는다.

즉 지금은 `같은 데이터를 보는 두 클라이언트`에 가깝고, 목표는 `같은 세션을 함께 조작하는 하나의 도구`다.

## 제품 원칙

- canonical object는 `thread`가 아니라 `shared session`이다.
- `thread`는 session의 durable timeline view로 유지하고, 기존 API 호환은 점진적으로 보존한다.
- Cursor native chat/session과 직접 결합할 수 있으면 연결하되, 제품의 소스 오브 트루스는 VibeDeck session으로 둔다.
- 모바일과 IDE는 같은 session state를 소비하고, UI만 각자 다르게 렌더링한다.
- 영속 기록과 순간 상태를 분리한다.

## 세션 모델

`SharedSession`

- `sessionId`
- `workspaceRoot`
- `provider`
- `threadId`
- `participants[]`
- `timeline[]`
- `liveState`
- `operationState`
- `transportState`
- `updatedAt`

`participants[]`

- `participantId`
- `clientType`: `mobile | cursor_panel | cursor_runtime | agent`
- `displayName`
- `active`
- `lastSeenAt`

`liveState`

- `presence`: 누가 현재 참여 중인지
- `composer`: draft text, typing 여부, 마지막 draft 주체
- `focus`: active file, selection, highlighted patch hunk, highlighted run error
- `activity`: 누가 현재 prompt 생성/patch review/run 중인지
- `followMode`: 모바일이 Cursor focus를 따라갈지, 반대로 Cursor가 session focus를 따라갈지

`operationState`

- `currentJobId`
- `currentTaskId`
- `phase`: `idle | prompting | reviewing | applying | running | waiting_input | stalled | recovering`
- `patchSummary`
- `patchFiles`
- `runProfileId`
- `runStatus`
- `lastError`
- `resumeToken` 또는 복구용 handle

## 이벤트 분리

### 1. Durable Events

디스크에 저장되고 timeline에 남는 이벤트.

- `prompt_submitted`
- `prompt_accepted`
- `patch_ready`
- `patch_apply_requested`
- `patch_applied`
- `run_requested`
- `run_finished`
- `session_note_added`
- `session_recovered`
- `session_stalled`

### 2. Ephemeral State Events

timeline에는 길게 남기지 않지만 실시간 협업에 필요한 상태.

- `presence_updated`
- `composer_updated`
- `typing_updated`
- `focus_updated`
- `selection_updated`
- `activity_updated`
- `transport_updated`
- `cursor_context_updated`

핵심 원칙은 다음과 같다.

- durable event는 복원과 감사에 사용한다.
- ephemeral event는 실시간 동기화와 UI 반응성에 사용한다.
- 동일한 정보가 둘 다 필요하면 ephemeral -> durable 승격 규칙을 둔다.

## 동기화 모델

기존 HTTP envelope 제어 경로는 유지한다.
다만 세션 상태 동기화는 polling 중심에서 stream 중심으로 바꾼다.

### 제안 API

- `GET /v1/agent/sessions/current`
- `GET /v1/agent/sessions`
- `GET /v1/agent/sessions/{id}`
- `GET /v1/agent/sessions/{id}/stream`
- `POST /v1/agent/sessions/{id}/presence`
- `POST /v1/agent/sessions/{id}/focus`
- `POST /v1/agent/sessions/{id}/composer`

초기 구현은 SSE 또는 WebSocket 둘 다 가능하지만, 기준은 다음과 같다.

- 읽기 전용 push는 SSE로도 충분하다.
- 양방향 live intent까지 한 채널로 묶고 싶으면 WebSocket이 더 자연스럽다.
- 기존 envelope 체계가 이미 있으므로, 장기적으로는 session stream도 envelope/event 프레임을 재사용하는 쪽이 일관적이다.

## Cursor 쪽 목표

Cursor extension은 단순 panel이 아니라 session participant가 된다.

- 현재 active file/selection/diagnostics를 session liveState에 publish
- 현재 review 중인 patch file/hunk를 publish
- 현재 run 상태와 상위 에러 focus를 publish
- 모바일이 보낸 `open_location`, `focus_patch`, `follow_session_focus` intent를 즉시 반영
- 가능하면 Cursor native 세션 메타데이터를 session에 연결

단, Cursor가 native AI chat 내부 session 제어를 충분히 열어주지 않으면 아래를 기준으로 간다.

- VibeDeck session을 canonical로 유지
- Cursor panel/runtime이 그 세션에 참여
- native chat은 optional integration으로 취급

## 모바일 UI 리디자인

현재 3탭 구조는 baseline으로 유지한 채, 다음 단계에서 단일 session 화면으로 통합한다.

### 새 정보 구조

- 상단: 세션 헤더
  - 현재 phase, 참여 중인 클라이언트, active workspace, 연결 상태
- 본문: session feed
  - 대화
  - patch review card
  - run result card
  - stall/recovery card
  - agent note/log summary
- 하단: composer + quick actions
  - prompt 입력
  - patch 검토 열기
  - run profile 선택/실행
  - follow Cursor focus 토글
- 보조 sheet
  - 연결 설정
  - signaling/direct 상태
  - ACK/runtime metrics
  - debug logs

### UI 목표

- 사용자가 탭 이동 없이 현재 세션의 전체 문맥을 이해할 수 있어야 한다.
- `채팅`, `검토`, `실행`, `복구`가 모두 한 피드 안에서 이어져 보여야 한다.
- Cursor에서 움직이면 모바일이 즉시 따라오고, 모바일에서 누르면 Cursor가 즉시 반응해야 한다.

## 복구와 관측성

Unified session이 성립하려면 "멈춘 세션" 경험이 먼저 해결돼야 한다.

- live job/task를 재시작 뒤 복원 가능한 범위까지 저장
- 완전 복원이 불가능한 provider라도 `마지막 phase`, `중단 지점`, `사용자 다음 액션`은 복원
- `stalled`, `recovering`, `needs_reissue` 같은 상태를 명시적으로 노출
- 모바일과 Cursor 모두 같은 recovery banner를 표시

## 구현 단계

### Phase A. 세션 모델 고정

- session schema 정의
- durable/ephemeral event 분리
- 기존 ThreadStore와 호환 전략 정의

완료 기준:

- architecture/implementation-plan/README가 unified session을 기준 용어로 정렬된다.
- 이후 구현이 참조할 필드와 상태 전이가 확정된다.

### Phase B. Agent live session foundation

- session read model
- session stream endpoint
- presence/composer/focus update API
- thread API와 session API 병행 제공

완료 기준:

- 모바일/extension이 polling 없이 같은 세션 변경을 즉시 반영할 수 있다.

### Phase C. Cursor session participant

- extension이 focus/presence/context를 publish
- session intent를 Cursor UI 동작으로 반영
- panel을 session feed 기반으로 재정렬

완료 기준:

- Cursor에서 파일/선택/실행 상태가 모바일에 실시간으로 보인다.

### Phase D. 모바일 단일 Session UI

- 3탭 구조를 session feed 중심으로 개편
- 상태 화면의 debug 항목은 보조 sheet로 이동
- patch/run card를 timeline 안에 통합

완료 기준:

- 모바일 단독으로도 세션 전체를 읽고 조작할 수 있다.

### Phase E. 복구/로그/마감

- stalled recovery
- session resume banner
- cross-client error/log presentation 정리

완료 기준:

- agent/extension 재시작 뒤에도 사용자가 "어디서 끊겼는지" 즉시 이해할 수 있다.

## 비범위

다음 항목은 unified session 핵심이 정리되기 전까지 후순위로 둔다.

- Cursor 외 provider 확장
- 배포 채널 다변화
- UI cosmetic polish만을 위한 대규모 리디자인

## 최종 완료 기준

- 모바일에서 쓰는 draft와 Cursor에서 보는 draft가 사실상 하나다.
- 한쪽에서 선택한 세션 focus가 다른 쪽에 즉시 보인다.
- patch review와 run result가 동일한 session feed에 누적된다.
- 세션이 멈춰도 이유와 다음 액션이 두 클라이언트 모두에 동일하게 나타난다.