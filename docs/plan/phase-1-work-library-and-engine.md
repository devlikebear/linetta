# Phase 1: 작품 갤러리와 로컬 엔진

## 목표

여러 작품을 만들고 다시 열 수 있는 기반을 만든다. 이 phase가 끝나면 Linetta는 단일 CLI가 아니라 "작품 갤러리 + 로컬 Go engine + 빈 작업실"을 가진 Mac 앱의 첫 형태가 된다.

## 범위

- SQLite 기반 local library
- 작품 CRUD 최소 기능
- 에피소드 목록 최소 기능
- `linetta serve` 로컬 HTTP API
- SwiftUI 작품 갤러리와 새 작품 생성

## 작업 목록

### 1. Go 저장소 기반 만들기

- [ ] `internal/store` 패키지 추가
  - `type DB struct { ... }`
  - `func Open(path string) (*DB, error)`
  - `func (db *DB) Close() error`
  - `func (db *DB) Migrate(ctx context.Context) error`
  - SQLite driver는 구현 시점에 하나를 선택한다. 기본 후보는 pure Go driver다.
- [ ] migration 추가
  - `works`
  - `episodes`
  - `agent_runs`
  - `agent_run_events`
- [ ] 테스트 추가
  - `internal/store/store_test.go`
  - temp DB에서 migrate가 idempotent인지 확인
  - works table 존재 확인

검증:

```sh
go test ./internal/store/...
```

### 2. 작품 도메인 추가

- [ ] `internal/work` 패키지 추가
  - `type Work struct`
  - `type CreateWorkInput struct`
  - `type Episode struct`
  - `type Repository struct`
- [ ] 함수 구현
  - `func (r *Repository) CreateWork(ctx context.Context, input CreateWorkInput) (Work, error)`
  - `func (r *Repository) ListWorks(ctx context.Context) ([]Work, error)`
  - `func (r *Repository) GetWork(ctx context.Context, id string) (Work, error)`
  - `func (r *Repository) CreateEpisode(ctx context.Context, workID string, title string) (Episode, error)`
  - `func (r *Repository) ListEpisodes(ctx context.Context, workID string) ([]Episode, error)`
- [ ] 테스트 추가
  - 작품 생성 후 목록에 나타나는지
  - 여러 작품이 섞이지 않는지
  - 작품별 에피소드가 분리되는지

검증:

```sh
go test ./internal/work/...
```

### 3. 로컬 HTTP API 추가

- [ ] `internal/server` 패키지 추가
  - `func New(repo *work.Repository, opts Options) http.Handler`
  - `GET /health`
  - `GET /api/works`
  - `POST /api/works`
  - `GET /api/works/{workID}`
  - `GET /api/works/{workID}/episodes`
  - `POST /api/works/{workID}/episodes`
- [ ] JSON response envelope는 과하게 만들지 말고, 도메인 객체를 명확한 JSON으로 반환한다.
- [ ] 실패 응답은 `{ "error": "..." }` 형태로 통일한다.
- [ ] 테스트 추가
  - `internal/server/server_test.go`
  - health 200
  - 작품 생성/목록/상세
  - 잘못된 work id 404

검증:

```sh
go test ./internal/server/...
```

### 4. `linetta serve` 추가

- [ ] [cmd/linetta/main.go](../../cmd/linetta/main.go)에 `serve` subcommand 추가
  - `go run ./cmd/linetta serve --db .linetta/linetta.db --addr 127.0.0.1:43190`
- [ ] 기본 DB 경로는 `~/.linetta/linetta.db`로 잡되, 테스트/개발에서는 `--db`로 바꿀 수 있게 한다.
- [ ] server startup log는 stderr에 짧게 출력한다.
- [ ] 테스트 추가
  - argument parser 단위 테스트
  - server handler는 `internal/server`에서 테스트하므로 CLI는 얇게 유지

검증:

```sh
go test ./...
go run ./cmd/linetta serve --db .linetta/dev.db --addr 127.0.0.1:43190
curl http://127.0.0.1:43190/health
```

### 5. SwiftUI 앱 scaffold

- [ ] `macos/Linetta` 아래 macOS SwiftUI app 생성
- [ ] 기본 구조
  - `LinettaApp.swift`
  - `AppState.swift`
  - `APIClient.swift`
  - `Views/WorkGalleryView.swift`
  - `Views/NewWorkSheet.swift`
  - `Views/WorkspaceView.swift`
- [ ] `APIClient`는 `GET /health`, `GET /api/works`, `POST /api/works`만 먼저 구현한다.
- [ ] 앱 첫 화면은 작품 갤러리다.
- [ ] 새 작품 생성 sheet에서 제목/장르/한 줄 premise를 입력한다.
- [ ] 작품 카드를 누르면 빈 `WorkspaceView`로 이동한다.

검증:

```sh
xcodebuild -project macos/Linetta/Linetta.xcodeproj -scheme Linetta -destination 'platform=macOS' test
```

수동 확인:

- [ ] 앱 실행 시 작품 갤러리가 보인다.
- [ ] 새 작품을 만들면 카드가 추가된다.
- [ ] 앱 재실행 후에도 작품이 남아 있다.
- [ ] 작품을 열면 해당 작품의 빈 작업실이 열린다.

---

### Checkpoint: Phase 1 완료 확인

**구현 확인:**
- [ ] Go API로 작품 생성/목록/상세가 가능하다.
- [ ] SwiftUI 앱에서 새 작품과 작품 갤러리가 동작한다.
- [ ] 작품별 에피소드 데이터가 분리되어 있다.

**실행 확인:**
- [ ] `go test ./...` 통과
- [ ] `curl http://127.0.0.1:43190/health`가 200 반환
- [ ] `xcodebuild ... test` 통과

**사용자 확인:**
- [ ] "여러 작품을 만들고 다시 열 수 있다"는 첫 경험이 자연스러운지 확인받는다.

이 체크포인트를 통과하면 사용자에게 확인 요청 후 Phase 2로 진행한다.
