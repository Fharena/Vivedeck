# VibeDeck Mobile (Flutter)

Prompt/Review/Status 3개 화면 베이스라인을 제공하는 모바일 앱 스캐폴드입니다.

## 포함 화면

- `Prompt`: 프롬프트 입력, 템플릿 선택, context 옵션 토글
- `Review`: 파일/헝크 단위 패치 검토 및 전체/선택 적용 액션
- `Status`: 연결 상태, pending ACK, 상태 히스토리 확인

## 실행 전 준비

Flutter SDK가 PATH에 있어야 합니다.

```bash
flutter --version
# 또는 저장소 루트 기준
.\tools\flutter\bin\flutter.bat --version
```

## 로컬 실행

```bash
cd mobile/flutter_app
..\..\tools\flutter\bin\flutter.bat pub get
..\..\tools\flutter\bin\flutter.bat run
```

## 메모

현재는 UI 베이스라인 단계이며, 실제 signaling/p2p/runtime API 연동은 다음 단계에서 추가합니다.
