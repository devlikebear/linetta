#!/usr/bin/env python3
"""Regenerates parity_golden.json from scripts/eval-voice.py.

  python3 engine/internal/clio/voice/testdata/gen_golden.py

The Go parity test asserts voice.Features / voice.EndingsOf and the importer
agree with these Python results on parity_posts.txt (invented posts only).
"""
import importlib.util
import json
import pathlib

here = pathlib.Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("evalvoice", here.parents[4] / "scripts" / "eval-voice.py")
ev = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ev)

posts = ev.load(here / "parity_posts.txt")
golden = {
    "posts": posts,
    "features": [ev.features(p) for p in posts],
    "endings": ev.endings_of(posts),
    "segments": [ev.segments(p) for p in posts],
}
(here / "parity_golden.json").write_text(json.dumps(golden, ensure_ascii=False, indent=1) + "\n", encoding="utf-8")
print(len(posts), "posts")
