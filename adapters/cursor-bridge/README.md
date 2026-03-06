# Cursor Bridge (TypeScript)

이 패키지는 VibeDeck PC Agent가 호출하는 `WorkspaceAdapter` 계약의 TypeScript 기준 구현입니다.
MVP 단계에서는 Mock 기반으로 플로우를 검증하고, 이후 Cursor Extension API 호출로 교체합니다.

## 포함 내용

- `WorkspaceAdapter` 인터페이스
- 공통 데이터 타입
- MockCursorBridge 구현

## 구현 원칙

- 코어 에이전트는 어댑터 구현 세부사항을 몰라야 함
- 패치 데이터는 파일/헝크 구조로 정규화
- 부분 적용 가능한 구조 유지

## 추후 연결 포인트

- active file / selection 조회
- patch apply (all/selected)
- open file/line 액션
- run profile command dispatch
