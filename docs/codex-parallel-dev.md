# Codex 병렬 개발 운영 가이드

이 문서는 `VibeDeck 제품 기능`이 아니라 `VibeDeck을 더 빨리 개발하기 위한 Codex 병렬 작업 방식`을 정리합니다.

## 목표

- coordinator thread 1개가 전체 방향과 최종 통합을 담당
- worker thread 2~3개가 별도 git worktree에서 병렬 구현
- 파일 충돌은 scope 고정으로 줄이고, 최종 적용은 coordinator만 수행

## 추천 기본 구성

가장 먼저는 총 3세션이 적절합니다.

1. `coordinator`
- 현재 메인 thread
- 최종 리뷰, 통합 테스트, 커밋, PR 생성만 담당

2. `agent worker`
- 권장 scope: `cmd/agent`, `internal/agent`, `internal/runtime`

3. `ui worker`
- 권장 scope: `mobile/flutter_app`

4. `extension worker`
- 권장 scope: `extensions/vibedeck-bridge`, `adapters/cursor-bridge`

처음에는 3 worker까지가 적당합니다. 그 이상은 통합 비용이 더 빨리 커집니다.

## 1. worker 세션 자동 생성

저장소 루트에서 아래 명령을 실행합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\setup_codex_workers.ps1
```

생성 결과:
- `.tmp\codex-sessions\<session>` 아래 세션 폴더
- worker별 git worktree
- worker별 시작 프롬프트 markdown
- `SESSION.md`, `session.json`

기본값으로는 **현재 coordinator가 서 있는 브랜치**를 base로 사용합니다. 필요할 때만 `-BaseRef main`처럼 명시하세요.

주의: 아직 commit하지 않은 working tree 변경은 새 worker worktree에 자동으로 복사되지 않습니다. 먼저 commit해 두는 편이 안전합니다.

직접 scope를 바꾸고 싶으면 이렇게 실행합니다.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\setup_codex_workers.ps1 `
  -SessionName vibedeck-fastlane `
  -Workers @(
    'agent=cmd/agent,internal/agent,internal/runtime',
    'mobile=mobile/flutter_app',
    'extension=extensions/vibedeck-bridge,adapters/cursor-bridge'
  )
```

## 2. Codex에서 무엇을 눌러야 하나

Codex 앱 버전에 따라 버튼 이름은 조금 다를 수 있습니다. 중요한 건 **각 worker worktree 폴더를 새 세션/새 창으로 여는 것**입니다.

순서:
1. 현재 thread는 그대로 둡니다. 이게 `coordinator`입니다.
2. `SESSION.md`에 적힌 worker별 `worktreePath`를 확인합니다.
3. Codex 앱에서 새 세션/새 창을 엽니다.
4. 각 세션에서 해당 `worktreePath` 폴더를 workspace로 엽니다.
5. `instructions\*.md` 내용을 그대로 worker 시작 프롬프트로 붙입니다.

즉 버튼 이름이 `Open Folder`, `Open Workspace`, `New Window`, `New Session` 중 무엇이든 상관없고,
핵심 동작은 **해당 worktree 폴더를 별도 세션으로 여는 것**입니다.

## 3. worker 규칙

모든 worker는 아래 규칙을 지킵니다.

- 자기 scope 밖 파일은 수정하지 않음
- 애매한 경계는 수정하지 않고 coordinator에 확인만 남김
- 큰 구조 변경보다 review 가능한 작은 patch 우선
- 결과 보고는 짧게
  - 수정 파일
  - 테스트 명령
  - 핵심 결과

## 4. coordinator 규칙

coordinator만 아래를 수행합니다.

- worker 결과 비교
- 범위 충돌 판단
- 통합 테스트 실행
- 최종 커밋/PR

worker가 만든 변경을 바로 main에 얹지 말고, coordinator가 마지막에 순서대로 반영합니다.

## 5. 세션 종료

worktree 세션을 닫고 정리할 때:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\cleanup_codex_session.ps1 -SessionName <session-name> -DeleteBranches -DeleteSessionFolder
```

## 6. 오래된 codex 브랜치 정리

이미 `main`에 머지된 오래된 `codex/*` 브랜치를 정리할 때:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\cleanup_merged_codex_branches.ps1
```

remote까지 같이 정리하려면:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\cleanup_merged_codex_branches.ps1 -IncludeRemote
```

## 운영 팁

- worker 수를 늘리기보다 scope 경계를 명확히 자르는 게 더 중요합니다.
- 한 worker가 여러 디렉터리를 건드리기 시작하면 바로 병목이 생깁니다.
- 처음 2~3회는 `agent/mobile/extension`처럼 디렉터리 축으로 자르는 게 가장 안전합니다.