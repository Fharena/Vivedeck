# 프로토콜 메시지 타입

## 제어 경로

- `PROMPT_SUBMIT`: 모바일 -> PC 프롬프트 제출
- `PROMPT_ACK`: PC -> 모바일 프롬프트 접수 확인
- `PATCH_READY`: PC -> 모바일 패치 검토 준비 완료
- `PATCH_APPLY`: 모바일 -> PC 패치 적용 요청
- `PATCH_RESULT`: PC -> 모바일 적용 결과
- `RUN_PROFILE`: 모바일 -> PC 실행 프로파일 요청
- `RUN_RESULT`: PC -> 모바일 실행 결과 요약
- `OPEN_LOCATION`: 모바일 -> PC 파일/라인 열기 요청
- `CMD_ACK`: 제어 메시지 ACK

## 시그널링 경로

- `SIGNAL_OFFER`
- `SIGNAL_ANSWER`
- `SIGNAL_ICE`

## 터미널 경로 (best-effort)

- `TERM`
- `TERM_ACK`
- `TERM_SUMMARY` (드롭된 라인 수 요약)

## Envelope

```json
{
  "sid": "session_id",
  "rid": "request_id",
  "seq": 123,
  "ts": 1700000000000,
  "type": "PROMPT_SUBMIT",
  "payload": {}
}
```
