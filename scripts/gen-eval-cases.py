#!/usr/bin/env python3
"""Writes the synthetic consistency cases (docs/plans/longform-continuity/eval-format.md).

Every case is two short scenes from a made-up story. Half hold a contradiction and
half hold something that only looks like one. Rerunning gives the same file.
"""
import json
import pathlib
import random

OUT = pathlib.Path(__file__).parent.parent / "engine/internal/ledger/testdata/eval/consistency.json"
PEOPLE = ["도윤", "서진", "하람", "태경", "민아", "유건", "소연", "재희", "현우", "지안"]
PLACES = ["강변 객잔", "북문 시장", "산중 암자", "서쪽 나루", "옛 관아"]
ITEMS = ["은장도", "낡은 지도", "청동 인장", "붉은 부채", "쇠피리"]
FILLER = [
    "비는 저녁 무렵에야 그쳤고 처마 끝에서 물방울이 천천히 떨어졌다.",
    "주막 안쪽에서는 장기를 두는 소리가 이어졌다.",
    "길가의 버드나무가 바람에 느리게 흔들렸다.",
    "행상 하나가 짐을 내려놓고 땀을 닦았다.",
    "아이들이 골목을 달려 지나가며 무어라 외쳤다.",
]

# type, contradiction?, attribute, earlier value, later value, earlier text, later text
TEMPLATES = [
    ("date", True, "작중 날짜", "3", "1",
     "그날은 장이 선 지 사흘째 되는 날이었다. {a}는 {place}에 도착했다.",
     "장이 선 첫날이었다. {a}는 {place}를 떠나며 어제 일을 떠올렸다. 사흘째 장에서 본 얼굴이었다."),
    ("date", False, "작중 날짜", "3", "5",
     "그날은 장이 선 지 사흘째 되는 날이었다. {a}는 {place}에 도착했다.",
     "장이 선 지 닷새째였다. {a}는 {place}를 떠나며 이틀 전 일을 떠올렸다."),
    ("death", True, "생사", "죽음", "생존",
     "{a}는 {place}에서 칼에 맞아 그 자리에서 죽었다. 사람들이 시신을 거두어 묻었다.",
     "{b}는 {place}에서 {a}를 만났다. {a}가 멀쩡히 걸어와 술잔을 들고 웃으며 말을 건넸다."),
    ("death", False, "생사", "실종", "생존",
     "{a}는 {place}에서 칼에 맞고 강에 떨어졌다. 아무도 시신을 찾지 못했다.",
     "{b}는 {place}에서 {a}를 만났다. 흉터를 보여 주며 강에서 어부에게 구해졌다고 했다."),
    ("item", True, "소지품", "{item}을 잃음", "{item}을 지님",
     "{a}는 {place}에서 {item}을 강물에 던졌다. 다시는 찾을 수 없는 곳이었다.",
     "{a}는 품에서 {item}을 꺼내 탁자에 올렸다. 강에 던진 적 없는 것처럼 늘 지니던 대로였다."),
    ("item", False, "소지품", "{item}을 맡김", "{item}을 되찾음",
     "{a}는 {place}에서 {item}을 전당포에 맡겼다. 값은 은 석 냥이었다.",
     "{a}는 은 석 냥을 치르고 {item}을 되찾아 품에 넣었다."),
    ("knowledge", True, "앎", "{b}의 정체를 앎", "{b}의 정체를 모름",
     "{a}는 {place}에서 {b}가 왕자라는 말을 직접 듣고 절을 올렸다.",
     "{a}는 {b}가 어떤 사람인지 전혀 몰랐다. 처음 듣는 이름이라고 생각하며 평범한 행상 대하듯 했다."),
    ("knowledge", False, "앎", "{b}의 정체를 모름", "{b}의 정체를 앎",
     "{a}는 {b}가 어떤 사람인지 전혀 몰랐다. 평범한 행상이라고만 생각했다.",
     "{a}는 {place}에서 {b}가 왕자라는 말을 듣고서야 지난날을 떠올리며 절을 올렸다."),
    ("relation", True, "관계", "{b}의 친형", "{b}와 초면",
     "{a}는 {b}의 친형이었다. 둘은 {place}에서 함께 자랐다.",
     "{a}와 {b}는 그날 처음 만났다. 서로 이름도 몰라 통성명부터 했다."),
    ("relation", False, "관계", "{b}의 원수", "{b}의 동맹",
     "{a}와 {b}는 원수였다. {place}에서 칼을 겨눈 적도 있었다.",
     "{a}와 {b}는 공동의 적 앞에서 손을 잡기로 했다. 묵은 원한은 일이 끝난 뒤로 미뤘다."),
    ("alias", True, "나이", "서른", "열일곱",
     "사람들이 청산객이라 부르는 {a}는 서른 살이었다. {place}에서 약을 팔았다.",
     "청산객은 이제 막 열일곱이 된 소년이었다. 그날도 {place}에서 약을 팔았다."),
    ("alias", False, "나이", "서른", "서른",
     "사람들이 청산객이라 부르는 {a}는 서른 살이었다. {place}에서 약을 팔았다.",
     "서른 줄의 청산객은 그날도 {place}에서 약을 팔았다."),
    ("flashback", True, "출신", "남쪽 어촌", "북쪽 산골",
     "{a}는 남쪽 어촌에서 태어나 바다를 보며 자랐다고 했다.",
     "{a}는 북쪽 산골에서 태어나 평생 바다를 본 적이 없었다. 서술자는 그 점을 분명히 해 둔다."),
    ("flashback", False, "출신", "남쪽 어촌", "남쪽 어촌",
     "{a}는 남쪽 어촌에서 태어나 바다를 보며 자랐다.",
     "오래전 일이었다. 어린 {a}는 남쪽 바닷가에서 그물을 깁던 아버지 곁에 앉아 있었다."),
    ("lie", True, "직업", "포졸", "약장수",
     "{a}는 관아의 포졸이었다. 서술자가 보기에도 틀림없는 사실이었다.",
     "{a}는 평생 약장수로만 살았고 관아에 발을 들인 적이 없었다. 이것은 소문이 아니라 사실이었다."),
    ("lie", False, "직업", "포졸", "약장수",
     "{a}는 관아의 포졸이었다.",
     "\"나는 평생 약장수로만 살았소.\" {a}는 {b} 앞에서 태연하게 거짓말을 했다."),
]


def fill(text, a, b, place, item):
    return text.format(a=a, b=b, place=place, item=item)


def main():
    rng = random.Random(20261102)
    cases = []
    for round_no in range(5):
        for t, (kind, contradiction, attribute, v1, v2, s1, s2) in enumerate(TEMPLATES):
            a, b = rng.sample(PEOPLE, 2)
            place = rng.choice(PLACES)
            item = rng.choice(ITEMS)
            pad = lambda: " ".join(rng.sample(FILLER, 2))
            later_soft = kind == "lie" and not contradiction
            cases.append({
                "id": f"{kind}-{'c' if contradiction else 'n'}{round_no}",
                "type": kind,
                "split": "tune" if round_no == 0 else "eval",
                "contradiction": contradiction,
                "scenes": [
                    f"{pad()} {fill(s1, a, b, place, item)} {pad()}",
                    f"{pad()} {fill(s2, a, b, place, item)} {pad()}",
                ],
                "fact": {
                    "subject": "작품" if kind == "date" else a,
                    "attribute": attribute,
                    "earlier": fill(v1, a, b, place, item),
                    "later": fill(v2, a, b, place, item),
                    "later_soft": later_soft,
                    # A fact that legitimately moves on is recorded as a replacement.
                    "replaces": not contradiction and kind not in ("date", "alias", "flashback", "lie"),
                    "number": kind == "date",
                },
            })
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(cases, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
    print(len(cases), "cases", sum(c["contradiction"] for c in cases), "contradictions")


if __name__ == "__main__":
    main()
