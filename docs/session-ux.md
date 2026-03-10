# Session UX 기준

이 문서는 VibeDeck을 `모바일 + Cursor/Codex가 하나의 작업 도구처럼 느껴지는 제품`으로 만들기 위한 UX 기준을 정리합니다.

## 문제 정의

지금까지의 구현은 shared session과 live sync 기반을 갖췄지만, 사용자 표면은 아직 `정보 카드 모음`에 가깝습니다.
사용자가 실사용에서 원하는 것은 다음 네 가지입니다.

- 채팅창: 지금 어떤 요청을 했고, 에이전트가 어디까지 왔는지 한눈에 보기
- 작업 로그: 에이전트가 지금 무엇을 하고 있는지, 왜 다음 행동으로 넘어가는지 이해하기
- 터미널: 실행 결과와 에러의 원문을 바로 보기
- 파일 트리: 어떤 파일이 현재 맥락의 중심인지, 무엇이 바뀌었는지 잡기

즉, 제품의 중심은 `설정/메트릭/세션 카드`가 아니라 `채팅 + 작업 로그 + 터미널 + 파일 트리`여야 합니다.

## Cursor/Codex UX에서 가져와야 할 핵심

Cursor나 Codex류 도구에서 사용자가 신뢰를 느끼는 이유는 raw chain-of-thought를 길게 읽어서가 아닙니다.
대신 아래 정보가 끊기지 않고 보이기 때문입니다.

- 현재 목표: 무엇을 해결하려는지
- 현재 행동: 어떤 파일을 읽거나, 어떤 명령을 실행하거나, 어떤 패치를 만드는지
- 다음 행동: 무엇을 기다리고 있는지, 다음으로 무엇을 할지
- 실제 근거: 터미널 출력, 에러, 변경 파일, 포커스 파일

따라서 VibeDeck도 `생각 과정`을 그대로 노출하는 대신, 아래처럼 안전하고 유용한 표면으로 바꿔 보여줘야 합니다.

- reasoning summary: 현재 판단을 1~3문장으로 요약
- plan trace: 단계별 진행 상태
- tool/action trace: 파일 읽기, 검색, 패치 생성, 테스트 실행 같은 작업 로그
- terminal output: 원문 출력과 요약
- file focus/tree: 현재 보고 있는 파일과 변경 파일 집합

## 노출 원칙

### 1. raw 내부 사고 대신 작업 로그를 보여준다

제품 표면에는 raw internal monologue를 그대로 노출하지 않습니다.
대신 사용자가 실제로 필요한 것은 아래입니다.

- 왜 지금 이 행동을 하는지에 대한 짧은 설명
- 어떤 도구/명령을 실행 중인지
- 무엇이 완료됐고 무엇이 남았는지
- 실패했다면 왜 실패했는지

즉, 표면 이름도 `생각 과정`보다 `작업 로그`, `진행 상황`, `분석 요약`이 더 맞습니다.

### 2. 터미널은 부가 정보가 아니라 핵심 표면이다

AI 코딩 UX에서 터미널은 진실의 원천입니다.
패치보다 먼저 사용자가 확인하는 경우도 많습니다.
따라서 터미널은 숨겨진 상세 화면이 아니라, 항상 꺼낼 수 있는 1급 표면이어야 합니다.

필수 요구사항:

- 실행 중에는 live stream으로 tail 보기
- 완료 후에는 full output / excerpt / top error 동시 제공
- 에러 라인과 파일 위치를 클릭 또는 탭으로 열기
- 자동 스크롤, 일시정지, 에러만 보기 지원

### 3. 파일 트리는 사용자의 공간 감각을 잡아준다

사용자는 에이전트가 추상적으로 "몇 개 파일을 수정했다"보다,
`어느 폴더 아래 어떤 파일이 바뀌었는지`를 보고 맥락을 이해합니다.

파일 표면은 최소한 아래를 제공해야 합니다.

- workspace tree
- changed files 묶음
- current focus file
- patch file / run error file로 바로 이동

### 4. 메인 피드는 채팅 중심이어야 한다

메인 화면의 기본 축은 채팅과 세션 피드입니다.
패치/터미널/파일은 이 피드와 연결된 보조 표면이어야지, 메인 흐름을 깨는 별도 앱처럼 보이면 안 됩니다.

## 정보 구조

### 공통 정보 구조

- 상단 헤더
  - 현재 세션 제목
  - 현재 phase
  - agent 상태
  - 참여자 수
  - 현재 active file 또는 run 상태
- 메인 피드
  - user prompt
  - assistant response
  - reasoning summary card
  - plan/progress card
  - patch summary card
  - run result summary card
  - stalled/recovery banner
- 보조 표면
  - terminal drawer
  - file tree pane 또는 sheet
  - patch detail pane
  - debug/transport panel

## 데스크톱/IDE 기준 레이아웃

Cursor/IDE에서는 다음 배치가 기준입니다.

- 좌측: 파일 트리
  - current focus file
  - changed files
  - run error files
- 중앙: 채팅 + 작업 로그 피드
  - 대화
  - reasoning summary
  - plan trace
  - patch/run summary
- 하단: 터미널 드로어
  - live output
  - previous command output
  - error filter

여기서 `작업 로그`는 채팅 피드 안에 자연스럽게 섞여야 합니다.
`파일 트리`와 `터미널`은 항상 열 수 있어야 하지만, 메인 흐름을 압도하면 안 됩니다.

## 모바일 기준 레이아웃

모바일은 화면이 좁으므로 한 화면에 전부 펼치기보다, 다음 계층이 적합합니다.

- 기본 화면: 채팅 + 작업 로그 피드
- 하단 고정 composer
- 하단 드로어: 터미널
- 우측 또는 하단 sheet: 파일 트리
- patch detail은 피드 카드에서 확장

모바일 기본 원칙:

- 채팅 피드가 1순위
- 터미널은 한 번 스와이프/탭으로 즉시 열려야 함
- 파일 트리는 항상 현재 focus 중심으로 열려야 함
- bootstrap, ACK, direct signaling, metrics는 기본 화면에서 제거하고 debug 영역으로 숨김

## 색/시각 방향

### Cursor 다크모드 기반

모바일과 extension 모두 Cursor 계열 다크 표면을 기준으로 맞춥니다.
밝은 카드 여러 장을 쌓는 현재 방향은 줄입니다.

기준:

- 기본 배경: 매우 어두운 청회색
- 패널: 본 배경보다 한 톤 밝은 dark panel
- 포커스 색: amber 또는 muted warm accent
- 성공/오류 색: 네온 계열이 아니라 muted green / muted red
- 텍스트: 고대비이되 pure white 남용 금지

### 불필요한 장식 제거

- 그라데이션 hero card 과다 사용 금지
- 메트릭 칩 남발 금지
- 사용자가 매번 보지 않아도 되는 연결 설정/운영 정보는 숨김
- 핵심은 `읽기`, `이해`, `중단`, `재개` 속도

## 제품 표면에서 줄여야 할 것

기본 화면에서 우선 제거 또는 후순위로 내려야 할 것:

- bootstrap 상세 값
- recent hosts
- ACK observability
- direct signaling 상세 로그
- adapter binary path
- runtime notes 전체 목록
- transport/debug 메트릭 전체 카드

이 정보는 debug sheet나 운영 모드에서만 보이게 합니다.

## 세션 이벤트/상태 모델 확장

UX를 위해 session 모델에 아래 표면을 추가합니다.

### live state

- `reasoningSummary`
  - 현재 판단 요약
  - 1~3문장
- `planItems[]`
  - step
  - status(`pending|in_progress|completed|blocked`)
- `toolActivity`
  - kind(`read_file|search|edit|run|apply_patch|open_location`)
  - target
  - status
  - startedAt
- `terminal`
  - activeSessionId
  - activeCommand
  - status
  - tail
- `workspaceView`
  - focusedPath
  - changedPaths[]
  - expandedRoots[]

### durable timeline event

- `reasoning_summarized`
- `plan_updated`
- `tool_started`
- `tool_finished`
- `terminal_started`
- `terminal_finished`
- `terminal_failed`
- `recovery_suggested`

원문 terminal chunk 전체는 live stream으로 흘리고, durable timeline에는 요약/시작/종료/실패만 남기는 쪽이 기본입니다.

## 상호작용 원칙

사용자는 아래 네 가지를 즉시 할 수 있어야 합니다.

- 중단: 지금 작업 멈추기
- 따라가기: 현재 파일/에러/패치를 따라가기
- 열기: 파일 또는 에러 위치 바로 열기
- 다시 시키기: 실패 이유를 보고 다음 지시 보내기

즉, UX는 수동 관람형이 아니라 `interruptible agent UI`여야 합니다.

## 다음 구현 우선순위

### PR48: Session UX foundation

- reasoning summary / plan trace / tool activity schema 추가
- terminal live state 및 event schema 정의
- workspace tree/focus state 모델 정의

### PR49: Cursor panel redesign

- Cursor panel을 `채팅 + 작업 로그 + 터미널 + 파일 트리` 구조로 개편
- 현재 panel의 thread viewer 느낌 제거

### PR50: Mobile dark session shell

- 모바일 기본 테마를 Cursor 계열 dark mode로 전환
- 메인 화면에서 불필요한 카드 제거
- terminal drawer + file tree sheet 추가

### PR51: Recovery/observability polish

- stalled/recovering banner
- 왜 멈췄는지 / 다음에 뭘 해야 하는지 명확히 표시
- debug surface와 기본 surface 완전 분리

## 완료 기준

- 사용자는 채팅 피드만 봐도 현재 세션 상태를 이해할 수 있다.
- 사용자는 터미널을 즉시 열어 raw output을 확인할 수 있다.
- 사용자는 현재 포커스 파일과 변경 파일 집합을 즉시 찾을 수 있다.
- 사용자는 raw 생각 과정 없이도 에이전트의 현재 행동과 다음 행동을 이해할 수 있다.
- 모바일과 Cursor가 시각적으로도 같은 도구의 두 화면처럼 느껴진다.