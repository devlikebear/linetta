# 일관성 평가 사례 형식

_속한 로드맵: [`roadmap.md`](./roadmap.md)_

합성 사례는 `engine/internal/ledger/testdata/eval/consistency.json`에 있다. `scripts/gen-eval-cases.py`로 다시 만들면 같은 파일이 나온다. 실제 원고와 실제 사례는 저장소에 넣지 않는다.

## 사례

```json
{
  "id": "date-c1",
  "type": "date",
  "split": "eval",
  "contradiction": true,
  "scenes": ["앞 장면", "뒤 장면"],
  "fact": {
    "subject": "작품",
    "attribute": "작중 날짜",
    "earlier": "3",
    "later": "1",
    "later_soft": false,
    "replaces": false,
    "number": true
  }
}
```

- `type`: 날짜(`date`), 죽음과 부활(`death`), 소지품(`item`), 인물별 지식(`knowledge`), 관계(`relation`), 별명(`alias`), 회상(`flashback`), 거짓말(`lie`).
- `split`: `tune`은 도구를 고칠 때 보는 16건, `eval`은 점수를 내는 64건이다.
- `contradiction`: 두 장면이 함께 참일 수 없으면 참이다. 유형마다 모순과 정상이 절반씩이다.
- `fact`: 에이전트가 정산했을 때 원장에 남길 사실이다. 값 비교 평가에 쓴다.

## 세 가지 평가

| 평가 | 실행 | 보는 것 |
|---|---|---|
| 값 비교 | `go test ./internal/ledger -run ConsistencyCases -v` | 사실이 바르게 기록됐을 때 코드가 충돌을 찾는가 |
| 영향 후보 | 같은 명령 | 바뀐 사실의 상대 회차가 후보에 드는가 |
| 모델 대조 | `EVAL_MODEL=<모델> scripts/eval-consistency.py pair 3 out.json` | 모델이 두 장면만 보고 모순을 찾는가 |

`buried` 모드는 두 장면을 무관한 장면 사이에 숨겨 긴 구간 읽기와 비교한다. `eval-consistency.py`는 합성 문장만 보낸다. 작가의 원고에 쓰지 않는다. 원고는 앱과 앱의 데이터 공유 동의를 거친다.

## 한계

합성 사례는 문장이 짧고 사실이 깔끔해서 실제보다 성적이 좋게 나온다. 실제 작품 성적과 나누어 본다. 작은 평가셋 통과가 일관성을 보장하지 않는다.
