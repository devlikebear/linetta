#!/usr/bin/env python3
"""Measures whether a voice profile makes generated posts read like the writer.

Holds out some of the writer's own posts, reduces each to a dry note of what it
says, and asks the Claude Code CLI (no tools) to write it again three ways:

  A  the note alone, as anyone would ask a model
  B  the note with a profile: counted habits, style notes, and similar posts
  C  B revised once against what the counting found off in it

then scores the originals and all three arms against the posts kept for
training, and writes a blind page for the writer to rate them by hand.

  scripts/eval-voice.py preview <posts>
  EVAL_MODEL=<model> scripts/eval-voice.py run <posts> <outdir> [holdout]
  scripts/eval-voice.py report <outdir>

<posts> is a folder of .txt/.md files (one post each, or several split by a
line of ---), one such file, an X archive's tweets.js, or a JSON export. The
posts go to Anthropic through the writer's own `claude` login, so run it only
on the writer's own posts, and keep <outdir> out of the repository: it holds
them. Every model answer is cached in <outdir>, so a stopped run resumes.
"""
import collections
import concurrent.futures as futures
import html
import json
import math
import os
import pathlib
import random
import re
import statistics
import subprocess
import sys

ENGINE = pathlib.Path(__file__).parent.parent / "engine"
SEED = 42
SAMPLES = 8          # posts shown to arms B and C
NOTE_POSTS = 60      # posts the style notes are drawn from
MIN_LETTERS = 15

URL = re.compile(r"https?://\S+")
EMOJI = re.compile("[\U0001F300-\U0001FAFF☀-➿\U0001F1E6-\U0001F1FF]")
LAUGH = re.compile(r"[ㅋㅎ]{2,}")
CRY = re.compile(r"[ㅠㅜ]{2,}")
HASHTAG = re.compile(r"#[^\s#]+")
ELLIPSIS = re.compile(r"\.{2,}|…")
SEGMENT = re.compile(r"[^.!?…\n]+[.!?…]*")
HANGUL = re.compile(r"[가-힣]")
POLITE = ("요", "니다", "세요", "까요", "죠")

# Phrases a model reaches for and a person posting rarely does. Matched as
# written; a writer may use any of them, which is why the score compares each
# arm with the writer's own rate rather than with zero.
AI_TELLS = [
    "결론적으로", "요약하자면", "핵심은", "중요한 것은", "중요한 건", "다양한", "효과적", "진정한",
    "여정", "인사이트", "에 대해 알아보", "여러분", "해 보세요", "해보세요", "어떠신가요", "단순히",
    "뿐만 아니라", "이처럼", "그렇다면", "무엇보다", "한 걸음", "되새기", "소중한", "깨달았",
    "—", "**", "##", "✨", "🚀", "💡", "👉", "🙌",
]

NOTE_PROMPT = """아래 SNS 글이 전하는 내용만 뽑아라.
사실, 주장, 감정을 개조식 3줄 이내로 적어라.
원문의 문장, 표현, 어미, 이모지, 말투는 옮기지 말고 건조한 메모체(~함)로 써라.
메모만 출력하라.

{post}"""

# The prompts speak as the writer, which is who runs this: asked to write "as
# someone else, indistinguishably", the model reads it as impersonation and
# refuses, and the app will ask the same way.
PROFILE_PROMPT = """아래는 내가 SNS에 직접 쓴 글들이다. 내 글버릇을 관찰 가능한 사실로 정리해 줘.
어미와 존댓말/반말, 문장 길이, 줄바꿈, 문장부호, 이모지와 ㅋㅋ 같은 표현, 자주 쓰는 단어,
글을 시작하고 끝내는 방식, 그리고 내가 하지 않는 것을 다뤄 줘.
항목마다 내 글에서 짧은 예를 붙여 줘. 평가나 칭찬은 빼고 목록만 출력해.

{posts}"""

BASELINE_PROMPT = """다음 내용으로 SNS에 올릴 글을 써줘. {length}자 안팎.

{note}

글만 출력해."""

VOICE_PROMPT = """이번에 SNS에 올릴 글 초안을 내 말투로 써 줘. 다듬은 글보다 내가 평소에 쓰는 그대로가 좋아.

[내 글버릇]
{notes}

[내 글에서 센 값]
{measured}

[내가 전에 쓴 글]
{samples}

[이번에 쓸 내용]
{note}

{length}자 안팎. 글 한 편만 출력해."""

REVISE_PROMPT = """내 SNS 글 초안이야. 내 평소 글과 다른 점을 아래에 적었어. 내용은 그대로 두고 그 부분만 내 말투로 고쳐 줘.

[내가 전에 쓴 글]
{samples}

[초안]
{draft}

[내 평소 글과 다른 점]
{issues}

고친 글 한 편만 출력해."""


# ---------------------------------------------------------------- loading

def unmangle(s):
    # Meta's exports write UTF-8 bytes as Latin-1 code points.
    try:
        return s.encode("latin-1").decode("utf-8")
    except (UnicodeEncodeError, UnicodeDecodeError):
        return s


def from_json(data):
    keys = {"text", "full_text", "content", "body", "caption", "title", "post"}
    out = []

    def walk(v, key=None):
        if isinstance(v, dict):
            for k, x in v.items():
                walk(x, k)
        elif isinstance(v, list):
            for x in v:
                walk(x, key)
        elif isinstance(v, str) and (key in keys or key is None):
            out.append(unmangle(v))

    walk(data)
    return out


def from_tweets(text):
    data = json.loads(text[text.index("["):])
    out = []
    for item in data:
        t = item.get("tweet", item)
        body = t.get("full_text", "")
        if body.startswith("RT @") or t.get("in_reply_to_status_id_str"):
            continue
        out.append(html.unescape(body))
    return out


def from_text(text):
    return re.split(r"\n-{3,}\n", text)


def load(path):
    p = pathlib.Path(path)
    wanted = lambda f: f.suffix in {".txt", ".md", ".json"} or (f.suffix == ".js" and f.name.startswith("tweets"))
    files = sorted(f for f in p.rglob("*") if wanted(f)) if p.is_dir() else [p]
    raw = []
    for f in files:
        text = f.read_text(encoding="utf-8")
        if f.name.startswith("tweets") and f.suffix == ".js":
            raw += from_tweets(text)
        elif f.suffix == ".json":
            raw += from_json(json.loads(text))
        else:
            raw += from_text(text)
    seen, posts = set(), []
    for r in raw:
        post = URL.sub("", r).strip()
        if len(HANGUL.findall(post)) + len(re.findall(r"[A-Za-z]", post)) < MIN_LETTERS or post in seen:
            continue
        seen.add(post)
        posts.append(post)
    return posts


# ---------------------------------------------------------------- counting

def segments(post):
    return [s.strip() for s in SEGMENT.findall(post) if HANGUL.search(s)]


def ending(seg):
    letters = HANGUL.findall(seg)
    return "".join(letters[-2:]) if len(letters) >= 2 else ""


def features(post):
    segs = segments(post)
    n = max(len(segs), 1)
    return {
        "length": len(post),
        "lines": post.count("\n") + 1,
        "segment_length": statistics.mean(len(s) for s in segs) if segs else 0,
        "punctuated": sum(s[-1] in ".!?…" for s in segs) / n,
        "polite": sum(ending(s).endswith(POLITE) for s in segs) / n,
        "emoji": len(EMOJI.findall(post)),
        "laugh": len(LAUGH.findall(post)),
        "cry": len(CRY.findall(post)),
        "hashtag": len(HASHTAG.findall(post)),
        "ellipsis": len(ELLIPSIS.findall(post)),
        "exclaim": post.count("!"),
        "ai_tells": sum(post.count(t) for t in AI_TELLS),
    }


def endings_of(posts):
    c = collections.Counter(e for p in posts for s in segments(p) if (e := ending(s)))
    total = sum(c.values()) or 1
    return {e: n / total for e, n in c.items()}


def jsd(p, q):
    keys = set(p) | set(q)
    m = {k: (p.get(k, 0) + q.get(k, 0)) / 2 for k in keys}

    def kl(a):
        return sum(a[k] * math.log2(a[k] / m[k]) for k in keys if a.get(k, 0) > 0)

    return (kl(p) + kl(q)) / 2


def bigrams(s):
    s = re.sub(r"\s+", "", s)
    return {s[i:i + 2] for i in range(len(s) - 1)}


def similar(note, train, rng):
    grams = bigrams(note)
    ranked = sorted(train, key=lambda p: -len(grams & bigrams(p)) / (len(grams | bigrams(p)) or 1))
    near = ranked[:SAMPLES - 3]
    rest = [p for p in train if p not in near]
    return near + rng.sample(rest, min(3, len(rest)))


def measured(train):
    f = [features(p) for p in train]

    def med(k):
        return statistics.median(x[k] for x in f)

    def per(k):
        return sum(x[k] for x in f) / len(f)

    top = sorted(endings_of(train).items(), key=lambda kv: -kv[1])[:8]
    return "\n".join([
        f"- 글 길이 중앙값 {med('length'):.0f}자, 줄 수 중앙값 {med('lines'):.0f}",
        f"- 문장 평균 {statistics.mean(x['segment_length'] for x in f):.0f}자",
        f"- 문장부호로 끝나는 문장 비율 {per('punctuated'):.0%}, 존댓말 어미 비율 {per('polite'):.0%}",
        f"- 글 하나당 이모지 {per('emoji'):.1f}, ㅋㅋ·ㅎㅎ {per('laugh'):.1f}, ㅠㅠ {per('cry'):.1f}, "
        f"해시태그 {per('hashtag'):.1f}, 말줄임 {per('ellipsis'):.1f}, 느낌표 {per('exclaim'):.1f}",
        "- 자주 쓰는 문장 끝: " + ", ".join(f"'{e}' {s:.0%}" for e, s in top),
    ])


def issues(draft, train):
    """What the counting finds off in a draft, as notes a reviser can act on."""
    f = [features(p) for p in train]
    d = features(draft)
    out = []
    for t in AI_TELLS:
        if t in draft and not any(t in p for p in train):
            out.append(f"작성자는 '{t}'를 쓰지 않는다.")
    for key, label in [("emoji", "이모지"), ("laugh", "ㅋㅋ·ㅎㅎ"), ("hashtag", "해시태그"),
                       ("exclaim", "느낌표"), ("ellipsis", "말줄임"), ("lines", "줄 수")]:
        vals = sorted(x[key] for x in f)
        lo, hi = vals[len(vals) // 10], vals[-1 - len(vals) // 10]
        if not lo <= d[key] <= hi:
            out.append(f"{label}가 {d[key]}개다. 작성자의 글은 대개 {lo}~{hi}개다.")
    for key, label in [("polite", "존댓말 어미"), ("punctuated", "문장부호로 끝나는 문장")]:
        mine = sum(x[key] for x in f) / len(f)
        if abs(d[key] - mine) > 0.3:
            out.append(f"{label} 비율이 {d[key]:.0%}다. 작성자는 {mine:.0%}다.")
    common = endings_of(train)
    odd = [e for s in segments(draft) if (e := ending(s)) and common.get(e, 0) == 0]
    if odd:
        out.append("작성자가 쓴 적 없는 문장 끝: " + ", ".join(sorted(set(odd))))
    return out


# ---------------------------------------------------------------- model

def ask(model, prompt, cache):
    if cache.exists():
        return cache.read_text(encoding="utf-8")
    done = subprocess.run(
        ["claude", "-p", "--model", model, "--tools", "", "--no-session-persistence", prompt],
        capture_output=True, text=True, timeout=300,
    )
    if done.returncode != 0 or not done.stdout.strip():
        raise RuntimeError(done.stderr.strip() or "empty answer")
    out = done.stdout.strip()
    if not HANGUL.search(out):
        raise RuntimeError("no Korean in the answer, likely a refusal: " + out[:120])
    cache.write_text(out, encoding="utf-8")
    return out


def numbered(posts):
    return "\n\n".join(f"[글 {i + 1}]\n{p}" for i, p in enumerate(posts))


# ---------------------------------------------------------------- commands

def preview(path):
    posts = load(path)
    print(f"{len(posts)} posts")
    for p in random.Random(SEED).sample(posts, min(5, len(posts))):
        print("---")
        print(p)


def run(path, outdir, holdout):
    model = os.environ["EVAL_MODEL"]
    out = pathlib.Path(outdir)
    (out / "cache").mkdir(parents=True, exist_ok=True)
    posts = load(path)
    if len(posts) < holdout + 20:
        sys.exit(f"{len(posts)} posts: need at least {holdout + 20} to hold out {holdout}")
    rng = random.Random(SEED)
    posts = rng.sample(posts, len(posts))
    test, train = posts[:holdout], posts[holdout:]
    cache = out / "cache"

    notes = ask(model, PROFILE_PROMPT.format(posts=numbered(rng.sample(train, min(NOTE_POSTS, len(train))))),
                cache / "profile.txt")
    counted = measured(train)

    def work(i):
        post = test[i]
        note = ask(model, NOTE_PROMPT.format(post=post), cache / f"{i}.note.txt")
        samples = numbered(similar(note, train, random.Random(SEED + i)))
        length = round(len(post), -1) or 10
        a = ask(model, BASELINE_PROMPT.format(note=note, length=length), cache / f"{i}.a.txt")
        b = ask(model, VOICE_PROMPT.format(notes=notes, measured=counted, samples=samples, note=note, length=length),
                cache / f"{i}.b.txt")
        found = issues(b, train)
        c = b if not found else ask(
            model, REVISE_PROMPT.format(samples=samples, draft=b, issues="\n".join(f"- {x}" for x in found)),
            cache / f"{i}.c.txt")
        return {"id": i, "note": note, "original": post, "A": a, "B": b, "C": c, "issues_b": found}

    with futures.ThreadPoolExecutor(max_workers=4) as pool:
        rows = list(pool.map(work, range(len(test))))
    (out / "train.json").write_text(json.dumps(train, ensure_ascii=False, indent=1), encoding="utf-8")
    (out / "rows.json").write_text(json.dumps(rows, ensure_ascii=False, indent=1), encoding="utf-8")
    report(outdir)


ARMS = ["original", "A", "B", "C"]


def report(outdir):
    out = pathlib.Path(outdir)
    train = json.loads((out / "train.json").read_text(encoding="utf-8"))
    rows = json.loads((out / "rows.json").read_text(encoding="utf-8"))
    groups = {arm: [r[arm] for r in rows] for arm in ARMS}

    base = [features(p) for p in train]
    keys = list(base[0])
    mean = {k: statistics.mean(x[k] for x in base) for k in keys}
    sd = {k: statistics.pstdev(x[k] for x in base) or 1 for k in keys}
    mine = endings_of(train)
    engine = measure({"train": train, **groups})

    lines = [f"학습 {len(train)}편, 시험 {len(rows)}편. 원문 열은 같은 사람의 다른 글이 보이는 흔들림이다.", ""]
    lines.append("| 지표 | 학습 글 | " + " | ".join(ARMS) + " |")
    lines.append("|---|---|" + "---|" * len(ARMS))
    zsum = collections.Counter()
    for k in keys:
        cells = []
        for arm in ARMS:
            v = statistics.mean(features(p)[k] for p in groups[arm])
            zsum[arm] += abs(v - mean[k]) / sd[k]
            cells.append(f"{v:.2f}")
        lines.append(f"| {k} | {mean[k]:.2f} | " + " | ".join(cells) + " |")
    lines.append("| **평균 \\|z\\|** (낮을수록 나와 가까움) | 0 | "
                 + " | ".join(f"**{zsum[a] / len(keys):.2f}**" for a in ARMS) + " |")
    lines.append("| **문장 끝 분포 JSD** (0~1, 낮을수록 가까움) | 0 | "
                 + " | ".join(f"**{jsd(mine, endings_of(groups[a])):.2f}**" for a in ARMS) + " |")
    lines += ["", "엔진(internal/style) 측정:", ""]
    lines.append("| 그룹 | 문장 수 | 문장 중앙값 | 끝맺음이 잡힌 문장 | 패턴 검사 |")
    lines.append("|---|---|---|---|---|")
    for name in ["train", *ARMS]:
        st, chk = engine[name]["stats"], engine[name]["check"]
        caught = sum(e["count"] for e in st["endings"])
        lines.append(f"| {name} | {st['sentences']} | {st['sentence_median']} | "
                     f"{caught / max(st['sentences'], 1):.0%} (상위 8개) | "
                     f"{', '.join(f'{k} {v}' for k, v in sorted(chk.items())) or '-'} |")
    text = "\n".join(lines)
    (out / "report.md").write_text(text + "\n", encoding="utf-8")
    blind(out, rows)
    print(text)
    print(f"\n블라인드 평가: {out / 'blind.html'}")


def measure(groups):
    done = subprocess.run(["go", "run", "./cmd/style-measure"], cwd=ENGINE, input=json.dumps({"groups": groups}),
                          capture_output=True, text=True, env={**os.environ, "CGO_ENABLED": "0"}, check=True)
    return json.loads(done.stdout)


def blind(out, rows):
    rng = random.Random(SEED)
    items = []
    for r in rows:
        arms = ARMS[:]
        rng.shuffle(arms)
        items.append({"note": r["note"], "cands": [{"arm": a, "text": r[a]} for a in arms]})
    page = BLIND_PAGE.replace("__DATA__", json.dumps(items, ensure_ascii=False).replace("</", "<\\/"))
    (out / "blind.html").write_text(page, encoding="utf-8")


BLIND_PAGE = """<!doctype html>
<html lang="ko"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>말투 블라인드 평가</title>
<style>
:root{--bg:#faf9f6;--fg:#1f1d1a;--muted:#6b665e;--card:#fff;--line:#e4e0d8;--accent:#3b5bdb}
@media (prefers-color-scheme:dark){:root{--bg:#171614;--fg:#ece8e1;--muted:#a39d93;--card:#211f1c;--line:#36332e;--accent:#8ea2ff}}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.6 -apple-system,"Apple SD Gothic Neo",sans-serif}
main{max-width:760px;margin:0 auto;padding:24px 16px 80px}
h1{font-size:22px;margin:0 0 4px}p.lead{color:var(--muted);margin:0 0 24px}
section{border-top:1px solid var(--line);padding:20px 0}
.note{color:var(--muted);font-size:14px;white-space:pre-wrap;margin-bottom:12px}
.cand{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:12px 14px;margin:10px 0}
.cand pre{white-space:pre-wrap;font:inherit;margin:0 0 10px}
.row{display:flex;flex-wrap:wrap;gap:12px;font-size:14px;color:var(--muted)}
.tag{display:none;font-weight:600;color:var(--accent)}.revealed .tag{display:inline}
button{font:inherit;padding:10px 16px;border-radius:8px;border:0;background:var(--accent);color:#fff;cursor:pointer}
table{border-collapse:collapse;margin-top:16px}td,th{border:1px solid var(--line);padding:6px 10px}
</style></head><body><main>
<h1>어느 글이 나 같은가</h1>
<p class="lead">각 묶음에는 내가 쓴 원문 하나와 AI가 쓴 글 셋이 섞여 있다. 글마다 '나 같음'을 1~5로 매기고, 원문이라고 생각하는 글 하나를 고른다.</p>
<div id="items"></div><button id="reveal">결과 보기</button><div id="result"></div>
<script>
const items = __DATA__;
let saved = {};
try { saved = JSON.parse(localStorage.getItem("voice-blind") || "{}"); } catch (e) {}
const save = () => { try { localStorage.setItem("voice-blind", JSON.stringify(saved)); } catch (e) {} };
const root = document.getElementById("items");
items.forEach((it, i) => {
  const s = document.createElement("section");
  s.innerHTML = `<div class="note"><b>${i + 1}.</b> 내용 메모\\n${esc(it.note)}</div>` + it.cands.map((c, j) => `
    <div class="cand"><pre>${esc(c.text)}</pre><div class="row">
      ${[1,2,3,4,5].map(v => `<label><input type="radio" name="r${i}_${j}" value="${v}"> ${v}</label>`).join("")}
      <label><input type="radio" name="o${i}" value="${j}"> 이게 원문</label>
      <span class="tag">${c.arm}</span></div></div>`).join("");
  root.appendChild(s);
});
root.querySelectorAll("input").forEach(el => {
  if (saved[el.name] === el.value) el.checked = true;
  el.addEventListener("change", () => { saved[el.name] = el.value; save(); });
});
function esc(t) { return t.replace(/[&<>]/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;"}[c])); }
document.getElementById("reveal").onclick = () => {
  document.body.classList.add("revealed");
  const score = {}, picked = {};
  items.forEach((it, i) => it.cands.forEach((c, j) => {
    const v = saved[`r${i}_${j}`];
    if (v) (score[c.arm] = score[c.arm] || []).push(+v);
    if (saved[`o${i}`] === String(j)) picked[c.arm] = (picked[c.arm] || 0) + 1;
  }));
  const arms = ["original", "A", "B", "C"];
  document.getElementById("result").innerHTML = `<table><tr><th>글</th><th>나 같음 평균</th><th>원문으로 고른 횟수</th></tr>` +
    arms.map(a => `<tr><td>${a}</td><td>${score[a] ? (score[a].reduce((x, y) => x + y) / score[a].length).toFixed(2) : "-"}</td><td>${picked[a] || 0}</td></tr>`).join("") +
    `</table><p class="lead">A: 그냥 맡긴 글 · B: 말투 프로필 · C: B를 검사 결과로 고친 글</p>`;
};
</script></main></body></html>
"""


def main():
    cmd = sys.argv[1]
    if cmd == "preview":
        preview(sys.argv[2])
    elif cmd == "run":
        run(sys.argv[2], sys.argv[3], int(sys.argv[4]) if len(sys.argv) > 4 else 12)
    elif cmd == "report":
        report(sys.argv[2])
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
