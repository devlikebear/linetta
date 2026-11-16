#!/usr/bin/env python3
"""Writes the meaning-search eval corpus (#384): 40 scenes, each holding three
target paragraphs from pairs.json among longer distractor paragraphs drawn
from one sentence pool, so every scene talks about the same people. A target
is found only by what it says, not by a name. Seeded: rerunning gives the same
file. The questions (pairs.json) were written by hand for the #384 spike: 110
say the target in other words, 10 share a word or a name with it."""
import json, random, pathlib
EVAL = pathlib.Path(__file__).parent.parent / "engine/internal/semantic/testdata/eval"
PAIRS = [(p["paragraph"], p["query"], p["kind"]) for p in json.load(open(EVAL / "pairs.json", encoding="utf-8"))]
rng = random.Random(384)
SENT = [
 "하린은 창가에 앉아 오래도록 말이 없었다.", "유나는 칼집을 고쳐 매고 길을 나섰다.", "도현은 장터의 소란 속을 천천히 걸었다.",
 "서진은 찻잔을 내려놓고 마당을 내다보았다.", "바람이 불어 처마 끝의 풍경이 흔들렸다.", "저녁 무렵 하늘이 붉게 물들었다.",
 "객잔 안에서는 장기 두는 소리가 이어졌다.", "누군가 멀리서 이름을 불렀지만 돌아보지 않았다.", "길가의 버드나무 가지가 느리게 흔들렸다.",
 "그날 밤은 유난히 별이 많았다.", "하린은 누나와 함께 걷던 골목을 지나쳤다.", "유나는 하린의 뒷모습을 잠시 바라보았다.",
 "도현은 주머니 속 동전을 세어 보았다.", "서진은 제자들의 발소리를 들으며 눈을 감았다.", "북쪽에서 내려온 상인들이 짐을 풀었다.",
 "아이들이 골목을 달려 지나가며 무어라 외쳤다.", "빗방울이 지붕을 두드리기 시작했다.", "하린은 손끝에 남은 굳은살을 문질렀다.",
 "유나는 오늘 들은 소문을 곱씹었다.", "도현은 어머니가 싸 준 주먹밥을 꺼냈다.", "서진은 오래된 서책의 먼지를 털었다.",
 "성문 앞에는 통행을 기다리는 줄이 길었다.", "강물은 어제보다 조금 불어 있었다.", "등불 아래서 누군가 편지를 읽고 있었다.",
 "하린은 피곤했지만 잠이 오지 않았다.", "유나는 검 손잡이의 감긴 끈을 다시 감았다.", "도현은 낯선 사람과 눈이 마주치자 고개를 숙였다.",
 "서진의 기침 소리가 복도 끝에서 들렸다.", "시장 한쪽에서 약초 장수가 목청을 높였다.", "새벽 안개가 마을을 덮고 있었다.",
]
def filler():
    return " ".join(rng.sample(SENT, rng.randint(3, 5)))
pairs = list(enumerate(PAIRS))
rng.shuffle(pairs)
scenes, gold = [], []
for s in range(40):
    chunk = pairs[s*3:(s+1)*3]
    paras = [filler() for _ in range(rng.randint(5, 8))]
    for idx, (text, query, kind) in chunk:
        pos = rng.randint(0, len(paras))
        paras.insert(pos, text)
    for idx, (text, query, kind) in chunk:
        gold.append({"id": idx, "scene": s, "paragraph": paras.index(text) + 1, "query": query, "kind": kind})
    scenes.append({"title": f"{s+1}화", "paragraphs": paras})
gold.sort(key=lambda g: g["id"])
json.dump({"scenes": scenes, "gold": gold}, open(EVAL / "corpus.json", "w", encoding="utf-8"), ensure_ascii=False, indent=1)
chars = sum(len(p) for sc in scenes for p in sc["paragraphs"])
print(len(scenes), "scenes,", sum(len(sc["paragraphs"]) for sc in scenes), "paragraphs,", chars, "chars,", len(gold), "queries")
