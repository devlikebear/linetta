# Phase 1 — Foundation: 토큰 시스템 + 폰트 번들

> 선행: 없음. 나머지 모든 페이즈의 기반.
> 원칙: 색/폰트/지오메트리의 **단일 출처(single source of truth)** 를 `App.css :root`에 만들고, 이후 페이즈는 이 토큰만 참조한다.

## 목표
1. 목업의 토큰 세트를 `App.css :root`에 도입.
2. Newsreader + IBM Plex Mono를 woff2로 로컬 번들 (`@font-face`).
3. 전역 바디/타이포그래피를 토큰 기반으로 교체.
4. 하드코딩 hex → 토큰 **매핑 표**를 확정 (후속 페이즈가 그대로 사용).

## 작업

### 1-A. 폰트 번들
- 디렉토리 생성: `apps/desktop/src/assets/fonts/`
- 다음 woff2를 받아 배치 (라틴 + 라틴-ext subset이면 충분; 한글은 시스템 명조 폴백):
  - Newsreader: Regular(400), Medium(500), SemiBold(600) + Italic 400 — Google Fonts / `@fontsource/newsreader`
  - IBM Plex Mono: Regular(400), Medium(500) — `@fontsource/ibm-plex-mono`
- 권장: `pnpm add @fontsource/newsreader @fontsource/ibm-plex-mono` 후 필요한 weight만 import, 또는 woff2를 직접 assets에 넣고 `@font-face` 작성.
- `App.css` 상단에 `@font-face` 블록(직접 번들 시) 또는 `main.tsx`에 fontsource import.
- **검증**: 빌드 산출물에 woff2가 포함되는지(`apps/desktop/dist/assets`), 오프라인에서 표시되는지 확인.

### 1-B. 토큰 `:root` (App.css)
목업 값 그대로 도입. `--font-*`의 한글 폴백 유지가 중요(`index.html`이 `lang="ko"`).

```css
:root {
  /* Palette */
  --paper: #f0ece2;  --surface: #faf7f0;  --surface-2: #ebe5d8;  --surface-3: #e3dccc;
  --ink: #211e18;    --ink-soft: #4d483e; --muted: #918a79;       --muted-2: #b1a994;
  --line: #d8d0bf;   --line-soft: #e6dfd0;

  --accent: oklch(0.56 0.13 47);        --accent-deep: oklch(0.48 0.13 47);
  --accent-tint: oklch(0.56 0.13 47 / 0.12);  --accent-tint2: oklch(0.56 0.13 47 / 0.06);

  /* Storyline threads */
  --t-sienna: oklch(0.58 0.13 47);  --t-teal: oklch(0.58 0.09 200);  --t-blue: oklch(0.56 0.10 255);
  --t-plum: oklch(0.55 0.12 350);   --t-olive: oklch(0.60 0.10 125);
  --ok: oklch(0.55 0.11 150);       --warn: oklch(0.55 0.15 35);

  /* Type */
  --font-ui:    -apple-system, BlinkMacSystemFont, "Apple SD Gothic Neo", "Malgun Gothic", "Pretendard", sans-serif;
  --font-serif: "Newsreader", "Nanum Myeongjo", "AppleMyungjo", "Apple SD Gothic Neo", Georgia, serif;
  --font-edit:  "Newsreader", "Nanum Myeongjo", "AppleMyungjo", "Apple SD Gothic Neo", Georgia, serif;
  --font-mono:  "IBM Plex Mono", ui-monospace, SFMono-Regular, monospace;

  /* Geometry */
  --r-sm: 6px; --r-md: 10px; --r-lg: 16px; --r-xl: 22px;
  --shadow-sm: 0 1px 2px rgba(33,30,24,0.06);
  --shadow-md: 0 8px 28px -10px rgba(33,30,24,0.22);
  --shadow-lg: 0 30px 80px -24px rgba(33,30,24,0.42);

  color-scheme: light;
}
```

### 1-C. 전역 바디 (App.css)
- `body { background: var(--paper); color: var(--ink); font-family: var(--font-ui); }`
- 본문/에디터 영역은 `--font-edit`, 코드/수치는 `--font-mono`.
- 기존 하드코딩 `#faf9f6` / `#1a1a1a` 제거.

### 1-D. 하드코딩 → 토큰 매핑 표 (후속 페이즈 적용 기준)

| 기존 hex (빈도) | 토큰 | 의미 |
|---|---|---|
| `#1a1a1a` (55) | `--ink` | 본문 텍스트 |
| `#d8d6cf` (42) | `--line` | 보더/구분선 |
| `#6b6b6b` (32), `#6b675e` | `--ink-soft` / `--muted` | 보조 텍스트 |
| `#faf9f6` (28), `#fffefb` (11) | `--surface` | 카드/패널 배경 |
| `#ece9e0` (25) | `--surface-2` / `--line-soft` | 함몰 패널/연한 선 |
| `#9a9a9a` (19), `#9a958b` | `--muted-2` | 흐린 텍스트/플레이스홀더 |
| `#a8312f`, `#a33` (붉은 강조) | `--accent` | 강조/CTA (붉은색 → 번트 시에나) |
| `#2980b9` (파란 강조) | `--t-blue` 또는 `--accent` | 링크/특정 강조 |
| `#f6f4ee` `#f4f1ea` `#f2efe6` `#efece2` 등 페이퍼 계열 | `--paper` / `--surface` | 배경 |
| radius 하드코딩(6/10/16px 류) | `--r-*` | 모서리 |

> 매핑이 애매한 hex는 가장 가까운 토큰으로 흡수하되, 새 토큰이 정말 필요하면 `:root`에 추가하고 이 표를 갱신한다.

## 체크포인트
- [ ] `pnpm add @fontsource/...` 또는 woff2 배치 완료, `@font-face`/import 작성
- [ ] `:root` 토큰 전체 도입
- [ ] `body` 토큰화, 전역 하드코딩 제거
- [ ] 매핑 표 확정 (이 문서에 반영)
- [ ] `make test-desktop` 통과
- [ ] `pnpm tauri dev`에서 폰트가 Newsreader로 렌더, 배경이 따뜻한 페이퍼톤인지 육안 확인

## 검증
```bash
cd apps/desktop && pnpm test && pnpm build
ls apps/desktop/dist/assets | grep -i woff2   # 폰트 번들 확인
```
