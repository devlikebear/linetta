# HWPX template

These parts come from `Skeleton.hwpx`, the blank document shipped with
[python-hwpx](https://github.com/airmang/python-hwpx) 6.12.0
(`src/hwpx/data/Skeleton.hwpx`), licensed under the Apache License 2.0.
That file was saved by Hancom Office Hangul; it is used here as the package
shape Hangul is known to open (#383).

Changes: `settings.xml`'s caret position is reset to 0. `header.xml` and
`section0.xml` are read as templates by `hwpx.go`: the writer appends its own
character and paragraph shapes to the header and replaces the body.

python-hwpx NOTICE: Copyright 2025-2026 airmang. Licensed under the Apache
License, Version 2.0.

"본 제품은 한글과컴퓨터의 한글 문서 파일(.hwp) 공개 문서를 참고하여 개발하였습니다."

"HWPX" is a document format specified and trademarked by Hancom Inc. Linetta
is not affiliated with or endorsed by Hancom Inc.
