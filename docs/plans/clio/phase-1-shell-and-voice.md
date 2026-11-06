# Phase 1: 뼈대와 내 말투 — 작업지시서

_작성일: 2026-11-04_
_속한 로드맵: [Clio 로드맵](./roadmap.md)_
_예상 소요: 3일_
_상태: 대기_

## 페이즈 목표

`make clio-dev`로 Clio 창이 뜬다. 내 글 묶음(`x-posts.json`, X 아카이브 `tweets.js`, `---`로 나눈 텍스트)을 가져오면, 맥락(플랫폼 × 글/답글)별 말투 프로필이 보인다. 이 프로필이 Phase 2 검사기의 기준이 된다.

## 전제 조건

- [ ] 브랜치: `feat/clio-phase-1` (`main` 기준. 실험 스크립트 `scripts/eval-voice.py`는 `#319`로 이미 main에 있다)
- [ ] 리네타의 다음 마이그레이션 번호는 `0046`이다(2026-11-06 기준, 마지막은 `0045_char_count_basis.sql`). 이 페이즈는 리네타 체인에 마이그레이션을 **추가하지 않는다**. Clio 체인은 `0001`부터 시작한다
- [ ] 시험 데이터 `~/Documents/voice-eval/x-posts.json`(150편, `kind` 포함)이 있다. 저장소에는 넣지 않는다
- [ ] `engine/`에서 `CGO_ENABLED=0 go test ./...`가 지금 통과한다

## 포함 기능

1. 엔진 공용부를 앱별로 주입할 수 있게 연다(홈, 마이그레이션 체인, 키체인 서비스)
2. 글 가져오기(미리보기 → 저장)
3. 맥락별 말투 프로필
4. Clio 앱 셸(Tauri + React)과 두 화면(가져오기, 내 말투)

## 이 페이즈에서 하지 않는 것

- 검사, 초안 편집(Phase 2)
- 모델 호출, 프로바이더 설정(Phase 3)
- Windows, iOS 빌드. `build.rs`는 macOS와 Linux만 처리한다
- 리네타 동작 변경. 공용부를 고치더라도 리네타의 기본값과 결과는 그대로여야 한다

## 작업 체크리스트

### 작업 그룹 A: 엔진 공용부 열기 (리네타 동작은 그대로)

- [ ] **T1.A.1** — 홈 경로를 앱별로
  - 파일: `engine/internal/paths/paths.go`, `paths_test.go`
  - 내용: `func HomeFor(appID, envVar string) (string, error)`를 추가하고, 기존 `Home()`은 `HomeFor(AppIdentifier, "LINETTA_HOME")`를 부르게 바꾼다. OS별 분기(darwin, linux XDG, windows)는 `HomeFor`로 옮긴다. 테스트: env가 있으면 env를 쓰고, 없으면 `<...>/<appID>`를 쓴다. 기존 `Home()` 테스트는 그대로 통과해야 한다

- [ ] **T1.A.2** — 마이그레이션 체인 주입
  - 파일: `engine/internal/store/store.go`, `migrations.go`, `store_test.go`
  - 내용: `func OpenWith(ctx context.Context, path string, migrations fs.FS) (*Store, error)`를 추가한다. 기존 `Open`은 내장 `migrations/*.sql`로 `OpenWith`를 부른다. `ApplyMigrations`와 `backup.RunPreMigration`의 목표 버전 계산이 주입된 FS를 쓰게 한다. 테스트: `fstest.MapFS`로 `0001_a.sql`, `0002_b.sql` 두 개를 주입해 적용되는지, 리네타 테이블(`projects` 등)은 생기지 않는지 확인한다

- [ ] **T1.A.3** — 키체인 서비스 주입
  - 파일: `engine/internal/settings/secrets.go`, `secrets_darwin.go`, `secrets_windows.go`, `secrets_other.go`
  - 내용: `func PlatformSecretStore(service string) SecretStore`를 export하고, 기존 `platformSecretStore()`는 `PlatformSecretStore("devlikebear.linetta")`로 바꾼다. Windows의 credential target 접두사도 service를 받게 한다
  - 검증: `GOOS=windows go build ./...`와 `GOOS=linux go build ./...` 통과(리네타는 Windows로 배포한다)

### 작업 그룹 B: Clio 엔진

- [ ] **T1.B.1** — Clio 마이그레이션과 글 저장소
  - 파일: `engine/internal/clio/migrations/0001_posts.sql`, `engine/internal/clio/migrations/embed.go`, `engine/internal/clio/post/post.go`, `post/repo.go`, `post/repo_test.go`
  - 내용:
    ```sql
    CREATE TABLE posts (
      id TEXT PRIMARY KEY,
      body TEXT NOT NULL,
      platform TEXT NOT NULL CHECK (platform IN ('x','threads','blog')),
      kind TEXT NOT NULL CHECK (kind IN ('post','reply')),
      source TEXT NOT NULL CHECK (source IN ('import','written')),
      external_id TEXT NOT NULL DEFAULT '',
      posted_at INTEGER NOT NULL DEFAULT 0,
      created_at INTEGER NOT NULL
    );
    CREATE UNIQUE INDEX idx_posts_body ON posts(platform, body);
    CREATE INDEX idx_posts_context ON posts(platform, kind);
    ```
    `Repo`: `NewRepo(*store.Store)`, `InsertMany(ctx, []NewPost) (inserted, skipped int, err error)`(같은 플랫폼에서 본문이 같으면 건너뜀), `List(ctx, ListParams{Platform, Kind, Limit, Offset})`, `Counts(ctx) ([]ContextCount, error)`. 단일 연결이므로 `List`는 `Rows`를 다 읽고 닫은 뒤 반환한다
  - 테스트: 임시 디렉터리에 `store.OpenWith(..., clio 마이그레이션)`로 연다. 중복 건너뛰기, 맥락별 개수, CHECK 위반을 확인한다

- [ ] **T1.B.2** — 글 가져오기 파서
  - 파일: `engine/internal/clio/voiceimport/parse.go`, `parse_test.go`, `testdata/`(합성 글만)
  - 내용: `func Parse(fileName string, content []byte, opts Options) (Result, error)`. 형식은 확장자와 내용으로 판별하고, `scripts/eval-voice.py`의 `load`, `from_tweets`, `from_json`, `from_text`, `unmangle`을 옮긴다
    - `tweets.js`: `window.YTD.tweets.part0 = ` 접두사를 떼고 JSON으로 읽는다. `retweeted`이거나 `full_text`가 `RT @`로 시작하면 버린다. `in_reply_to_user_id_str`가 있고 `opts.OwnUserID`와 다르면 `reply`, 같으면(내 스레드) `post`
    - JSON: 배열 원소의 `text`/`full_text`/`body`와 선택 필드 `kind`, `platform`을 읽는다(`x-posts.json` 형식)
    - 텍스트: `\n---\n`으로 나누고, `kind`는 `opts.DefaultKind`를 쓴다
    - 공통: `https?://\S+`와 t.co 링크를 지우고, 공백을 정리하고, 한글과 영문 글자가 15자 미만인 글은 버리고(`MIN_LETTERS`), 중복을 버린다
    - `Result{Posts []NewPost; Skipped map[string]int}`. 버린 이유별 개수를 담는다(retweet, short, duplicate)
  - 테스트: 형식별 합성 fixture로 개수와 kind를 확인하고, 각 버린 이유를 1건 이상 확인한다

- [ ] **T1.B.3** — 말투 특징과 프로필
  - 파일: `engine/internal/clio/voice/features.go`, `profile.go`, `features_test.go`, `profile_test.go`
  - 내용:
    - `func Features(post string) Feature`: `eval-voice.py`의 `features()`를 옮긴다. 필드는 `Length`, `Lines`, `SegmentLength`, `Punctuated`, `Polite`, `Emoji`, `Laugh`, `Cry`, `Hashtag`, `Ellipsis`, `Exclaim`이다. 문장 조각은 Python의 `SEGMENT` 규칙을 쓴다. `style.Sentences`는 문장부호로 끝나는 문장만 끝맺음을 세서 SNS 글에서 놓치는 것이 많다(PR #319에서 확인). 끝맺음은 마지막 한글 2글자이고, 존댓말 어미는 `요`, `니다`, `세요`, `까요`, `죠`다
    - `func Build(posts []post.Post) Profile`: 맥락 키별로 `N`, 특징마다 `P10/P50/P90`, 끝맺음 분포 전체와 상위 8개, `PoliteShare`, 맥락별 예시 글 id 5개를 만든다. 맥락 키는 `x/post`, `x/reply`, `x/*`, `*`다. `func (p Profile) For(platform, kind string) ContextProfile`는 N이 20 미만이면 `x/*`, 그다음 `*`로 물러나고, 어느 키를 썼는지 `Fallback` 필드로 알려 준다
  - 테스트: 특징별 표 테스트(이모지, ㅋㅋ, 존댓말, 줄 수, 마침표 없는 문장), 물러남 순서, 빈 입력

- [ ] **T1.B.4** — 프로필 비교용 CLI
  - 파일: `engine/cmd/clio-voice/main.go`, `main_test.go`
  - 내용: `clio-voice <file> [--kind post|reply]`가 `voiceimport.Parse` → `voice.Build` → JSON을 stdout에 쓴다. `engine/cmd/style-measure`와 같은 구조. Checkpoint에서 Python 결과와 맞춰 보는 데 쓴다

- [ ] **T1.B.5** — Clio 엔진 앱과 FFI
  - 파일: `engine/internal/clio/app/app.go`, `app_test.go`, `engine/internal/rpc/handlers/clio_posts.go`, `clio_voice.go`(+ `_test.go`), `engine/cmd/clio-ffi/ffi.go`
  - 내용:
    - `app.Open(ctx, Options{Home, Secrets})`: 홈을 만들고, `store.OpenWith(<home>/clio.db, cliomigrations.FS)`로 연 뒤 `rpc.NewServer()`에 핸들러를 등록한다. `engineapp.Open`의 구조를 따르되 리네타 저장소는 하나도 만들지 않는다
    - RPC: `posts.import_preview`(`{file_name, content, default_kind, own_user_id}` → 개수, 맥락별 개수, 버린 이유, 앞의 5편), `posts.import`(같은 입력 → inserted, skipped), `posts.list`, `posts.counts`, `voice.profile`(`{platform, kind}` → `ContextProfile`). 엔진은 디스크를 읽지 않는다. 파일 내용은 프런트엔드가 넘긴다(`imports.markdown`과 같은 원칙)
    - `clio-ffi`: `engine/cmd/linetta-ffi/ffi.go`를 복사하고 `LinettaEngine*` → `ClioEngine*`, `engineapp` → `clio/app`, 홈 기본값을 `paths.HomeFor("com.devlikebear.clio", "CLIO_HOME")`으로, 비밀 저장소를 `settings.PlatformSecretStore("devlikebear.clio")`로 바꾼다
  - 테스트: 핸들러마다 잘못된 params → `CodeInvalidParams`. 가져오기 → 목록 → 프로필을 한 번 왕복한다

### 작업 그룹 C: 앱 셸

- [ ] **T1.C.1** — 프런트엔드 프로젝트
  - 파일: `apps/clio/package.json`, `pnpm-workspace.yaml`(`allowBuilds`만), `vite.config.ts`(포트 1430), `tsconfig*.json`, `eslint.config.js`, `index.html`, `src/main.tsx`, `src/App.tsx`, `src/App.css`, `src/test/setup.ts`
  - 내용: `apps/desktop`의 설정을 복사하고 tiptap 등 쓰지 않는 의존성은 뺀다. `App.css` 토큰은 `--c-paper`, `--c-surface`, `--c-ink`, `--c-muted`, `--c-line`, `--c-accent`, `--c-warn`, `--c-danger`. 라우트는 `/`(가져오기 또는 내 말투로 이동), `/import`, `/voice`

- [ ] **T1.C.2** — RPC 래퍼와 엔진 대기
  - 파일: `apps/clio/src/lib/rpc.ts`, `lib/types.ts`, `components/EngineGate.tsx`, `lib/rpc.test.ts`
  - 내용: `apps/desktop/src/lib/rpc.ts`의 `invoke("engine_call", {method, params})` 패턴을 따르고, `posts.*`와 `voice.profile` 타입을 Go 구조체와 1:1로 맞춘다

- [ ] **T1.C.3** — Tauri 크레이트
  - 파일: `apps/clio/src-tauri/Cargo.toml`, `build.rs`, `tauri.conf.json`, `capabilities/default.json`, `src/main.rs`, `src/lib.rs`, `src/ffi.rs`
  - 내용:
    - `Cargo.toml`: 크레이트 `clio-desktop`, lib `clio_desktop_lib`, Tauri 2.12, 플러그인은 dialog, opener만
    - `build.rs`: `apps/desktop/src-tauri/build.rs`의 `build_go_engine()`을 줄여서 `./cmd/clio-ffi`를 c-archive(`libclio_engine_ffi.a`)로 빌드한다. macOS와 Linux만
    - `ffi.rs`: 데스크톱 `ffi.rs`의 정적 링크 경로만 복사하고 심볼 이름을 바꾼다. `libloading`(Windows) 경로는 뺀다
    - `lib.rs`: 명령은 `engine_call`, `engine_status`, `open_external_url`. `RENDERER_ENGINE_METHODS`는 정렬된 상수 배열로 두고, 정렬 테스트를 복사한다. `EXTERNAL_URL_HOSTS`는 `x.com`, `www.threads.com`
    - `tauri.conf.json`: identifier `com.devlikebear.clio`, 창 크기 900x680, 개발 URL `http://localhost:1430`, CSP는 데스크톱 설정에서 쓰지 않는 출처를 뺀다

- [ ] **T1.C.4** — 가져오기 화면
  - 파일: `apps/clio/src/routes/Import.tsx`, `Import.css`, `Import.test.tsx`
  - 내용: 파일 선택(`tauri-plugin-dialog` + 프런트엔드에서 읽기) → 플랫폼과 기본 종류 선택 → X 사용자 id(선택, 아카이브의 내 스레드 판별용) → `posts.import_preview` 결과(전체, 맥락별, 버린 이유, 예시 5편) → 가져오기 → 결과 토스트

- [ ] **T1.C.5** — 내 말투 화면
  - 파일: `apps/clio/src/routes/Voice.tsx`, `Voice.css`, `Voice.test.tsx`
  - 내용: 맥락 탭(`X 글`, `X 답글`, `전체`) → 특징 표(P10 / P50 / P90), 존댓말 비율, 끝맺음 상위 8개와 비중, 예시 글 5편. 물러난 맥락이면 "표본이 적어 X 전체 기준으로 보여 줌"을 표시한다

### 작업 그룹 D: 빌드 명령

- [ ] **T1.D.1** — Makefile
  - 파일: `Makefile`, `scripts/clio-dev.sh`
  - 내용: `clio-dev`(`cd apps/clio && pnpm tauri dev`), `test-clio`(`cd engine && CGO_ENABLED=0 go test ./internal/clio/... ./cmd/clio-voice` + `apps/clio`의 lint, test, build + `src-tauri`의 `cargo check`). 기존 `test`와 `ci`에는 아직 넣지 않는다. Windows CI가 Clio 크레이트를 빌드하지 않게 하기 위해서다(CI 작업은 Phase 4에서 추가)

---

### ✅ Checkpoint: Phase 1 완료 확인

**구현 확인:**
- [ ] 리네타 쪽 변경은 `paths`, `store`, `settings`의 주입 함수뿐이고, 리네타 기본값이 그대로다
- [ ] Clio DB에 리네타 테이블이 없다: `sqlite3 "$HOME/Library/Application Support/com.devlikebear.clio/clio.db" .tables`에 `posts`, `schema_migrations`만 나온다

**실행 확인:**
- [ ] `cd engine && CGO_ENABLED=0 go test ./...` 통과
- [ ] `cd engine && GOOS=windows go build ./... && GOOS=linux go build ./...` 통과
- [ ] `make test-clio` 통과
- [ ] `scripts/validate-story-core-deps.sh` 통과
- [ ] 프로필 일치: 아래 두 명령의 `x/*` 맥락 값(length, lines, segment_length, punctuated, polite, emoji, hashtag의 평균)이 소수 둘째 자리까지 같다
  ```bash
  cd engine && go run ./cmd/clio-voice ~/Documents/voice-eval/x-posts.json
  ```
  ```bash
  python3 -c "import importlib.util,statistics as s;spec=importlib.util.spec_from_file_location('e','scripts/eval-voice.py');e=importlib.util.module_from_spec(spec);spec.loader.exec_module(e);P=e.load('$HOME/Documents/voice-eval/x-posts.json');F=[e.features(p) for p in P];print(len(P),{k:round(s.mean(f[k] for f in F),2) for k in F[0]})"
  ```
  (Python 함수가 평균 대신 중앙값을 내는 항목이 있으면 Go도 같은 통계로 비교한다.)

**수동 확인:**
- [ ] `make clio-dev` → 가져오기에서 `x-posts.json` 선택 → 미리보기 150편(X 글 112, X 답글 38) → 가져오기
- [ ] 내 말투 → `X 답글` 탭의 존댓말 비율이 `X 글` 탭보다 확실히 높다. 이 차이가 이 앱의 핵심 가정이다
- [ ] 같은 파일을 다시 가져오면 150편이 모두 "중복"으로 건너뛰어진다

**이 체크포인트를 통과하면 사용자에게 확인 요청 후 다음으로 진행.**
만약 실패하면: 실패 항목 보고 → 사용자와 함께 원인 파악 → 수정 → 재검증.
