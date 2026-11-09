#!/usr/bin/env python3
"""Measures how well a model finds contradictions between scenes.

Reads the synthetic cases (scripts/gen-eval-cases.py) and asks the Claude Code CLI
about each, with no tools. It sends made-up text only: never point it at a
writer's manuscript, which goes through the app and its data-sharing consent.

  EVAL_MODEL=<model> scripts/eval-consistency.py pair 3 out.json
  EVAL_MODEL=<model> scripts/eval-consistency.py buried 1 out.json
  scripts/eval-consistency.py estimate out.json

pair sets the two scenes side by side, as a sample check does. buried hides them
among unrelated scenes, as reading a long stretch of a book does. estimate takes
two runs of pair and asks how many contradictions there are from how much the
runs overlap (Lincoln-Petersen), which here can be held against the true count.
"""
import concurrent.futures as futures
import json
import os
import pathlib
import random
import subprocess
import sys
import time

CASES = pathlib.Path(__file__).parent.parent / "engine/internal/ledger/testdata/eval/consistency.json"
PROMPT = """아래는 소설의 장면들이다. 함께 참일 수 없는 진술이 있는지 찾아라.
인물의 거짓말, 소문, 시간이 흘러 달라진 것, 나중에 밝혀진 사정은 모순이 아니다.
첫 줄에 YES 또는 NO만 써라.

{scenes}"""
PADDING = ("장터는 아침부터 붐볐다. 소금 장수와 포목 장수가 자리를 두고 실랑이를 벌였고, "
           "떡장수 아낙이 김이 오르는 시루를 내려놓았다. 나루 쪽에서는 배가 들어온다는 소리가 들렸다. ")


def ask(model, prompt):
    # --safe-mode keeps the caller's CLAUDE.md, memory, hooks and MCP servers out
    # of the question; --tools "" alone removes only the built-in tools.
    done = subprocess.run(
        ["claude", "-p", "--model", model, "--tools", "", "--safe-mode", "--no-session-persistence", prompt],
        capture_output=True, text=True, timeout=240,
    )
    if done.returncode != 0:
        raise RuntimeError(f"exit {done.returncode}: {done.stderr.strip()[-200:]}")
    # A failed or off-format reply is an error, not a NO: counting it as NO
    # would lower recall for reasons that have nothing to do with the model.
    first = done.stdout.strip().splitlines()[0] if done.stdout.strip() else ""
    word = first.strip(" *#`>-").upper()
    if word.startswith("YES"):
        return True
    if word.startswith("NO"):
        return False
    raise ValueError(f"unparsed reply: {first[:80]!r}")


def scenes_of(case, mode, rng):
    a, b = case["scenes"]
    if mode == "pair":
        return [a, b]
    filler = [PADDING * 18 for _ in range(10)]
    at = sorted(rng.sample(range(len(filler) + 2), 2))
    out = filler
    out.insert(at[0], a)
    out.insert(at[1], b)
    return out


def estimate(path):
    rows = [r for r in json.loads(pathlib.Path(path).read_text(encoding="utf-8")) if "said" in r]
    truth = {r["id"] for r in rows if r["truth"]}
    found = {}
    for r in rows:
        if r["truth"] and r["said"]:
            found.setdefault(r["run"], set()).add(r["id"])
    runs = sorted(found)
    for i, a in enumerate(runs):
        for b in runs[i + 1:]:
            both = len(found[a] & found[b])
            guess = len(found[a]) * len(found[b]) / both if both else float("inf")
            print(f"runs {a},{b}: n1={len(found[a])} n2={len(found[b])} both={both} estimate={guess:.1f} true={len(truth)}")
    per_type = {}
    for r in rows:
        s = per_type.setdefault(r["type"], [0, 0, 0, 0])
        s[(0 if r["said"] else 2) + (0 if r["truth"] else 1)] += 1
    for kind, (tp, fp, fn, tn) in sorted(per_type.items()):
        print(f"{kind:10s} tp={tp} fp={fp} fn={fn} tn={tn}")


def main():
    if sys.argv[1] == "estimate":
        estimate(sys.argv[2])
        return
    mode, runs, target = sys.argv[1], int(sys.argv[2]), sys.argv[3]
    model = os.environ["EVAL_MODEL"]
    cases = [c for c in json.loads(CASES.read_text(encoding="utf-8")) if c["split"] == "eval"]
    if mode == "buried":
        cases = cases[::3]
    jobs = []
    for run in range(runs):
        rng = random.Random(run)
        for case in cases:
            body = "\n\n".join(f"[장면 {i + 1}]\n{s}" for i, s in enumerate(scenes_of(case, mode, rng)))
            jobs.append((run, case, PROMPT.format(scenes=body)))

    def work(job):
        run, case, prompt = job
        try:
            return {"run": run, "id": case["id"], "type": case["type"], "truth": case["contradiction"], "said": ask(model, prompt)}
        except Exception as err:
            return {"run": run, "id": case["id"], "type": case["type"], "truth": case["contradiction"], "error": str(err)}

    started = time.monotonic()
    with futures.ThreadPoolExecutor(max_workers=4) as pool:
        rows = list(pool.map(work, jobs))
    took = time.monotonic() - started
    pathlib.Path(target).write_text(json.dumps(rows, ensure_ascii=False, indent=0), encoding="utf-8")
    good = [r for r in rows if "said" in r]
    tp = sum(r["truth"] and r["said"] for r in good)
    fp = sum(not r["truth"] and r["said"] for r in good)
    fn = sum(r["truth"] and not r["said"] for r in good)
    timeouts = sum("timed out" in r.get("error", "") for r in rows)
    print(f"{mode}: calls={len(rows)} n={len(good)} errors={len(rows) - len(good)} timeouts={timeouts} seconds={took:.0f} precision={tp / max(tp + fp, 1):.2f} recall={tp / max(tp + fn, 1):.2f}")


if __name__ == "__main__":
    main()
