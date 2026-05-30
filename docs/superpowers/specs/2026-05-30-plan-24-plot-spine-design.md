# Plan 24 — 플롯 스파인 (Plot Spine) 설계

> 작성일: 2026-05-30
> 상위 맥락: 사용자가 요청한 4개 서브시스템(① 플롯 관리, ② 문체·톤·서식 일관성, ③ 일관성 검증, ④ AI 대화로 설정 관리) 중 **첫 번째 서브시스템**. 나머지(②③④)는 각자 별도 spec → plan → 구현 사이클로 진행한다.

## 목표 (한 문장)

작품 개요와 씬 단위 플롯 비트를 1급 데이터로 만들고, AI 컨텍스트를 "개요 + 전/현/후 씬 플롯 + 등장 엔티티·관계 + 현재 씬 본문"이라는 **상한이 고정된 플롯 스파인**으로 재편한다.

## 배경 / 동기

- 기존에 `threads`(스토리라인) + `beats`(비트) 구조가 이미 있으나, beat가 한 줄 `label` + `intensity`(1~3)뿐이라 실질 플롯을 담지 못하고, UX가 직관적이지 않아 사용자가 거의 사용하지 않았다.
- 현재 AI 주입은 계층 요약(다른 부/장 요약), 토폴로지 RAG, 같은 장 다른 씬 등으로 작품이 길어질수록 커진다. 사용자는 "개요 + 전/후 씬 + 플롯 + 엔티티·관계만 들어가도 일관성 유지에 충분하며 주입 상한을 관리할 수 있다"고 판단했다.
- 따라서 본 작업은 **맨바닥 신규 개발이 아니라 기존 threads/beats를 플롯의 중추로 끌어올리고, 주입 전략을 플롯 중심으로 슬림화**하는 일이다.

## 확정된 설계 결정 (브레인스토밍 합의)

1. **기반**: 새 플롯 레이어를 만들지 않고 기존 `threads`/`beats`를 보강한다. (thread = 플롯 줄기, beat = 플롯 점)
2. **컨텍스트 전략**: 플롯 중심으로 슬림화한다. 거시 구조는 "요약 더미"가 아니라 "개요 + 플롯 비트"로 표현한다.
3. **beat 풍성도**: 기존 한 줄 `label`을 제목으로 유지하고, "무슨 일이 일어나는지"를 담는 `description` 본문 필드를 추가한다.
4. **개요 형태**: 프로젝트 단일 편집 텍스트(`outline`). 자동 파생 synopsis는 폴백용으로 공존.
5. **UX 표면**: 집필 화면 우측 인라인 패널(별도 라우트/스플릿 없음, 미니멀 유지).

## 아키텍처 개요

플롯 스파인은 문서 순서(DFS leaf order) 기준으로 다음을 한 묶음으로 본다:

```
┌─ 개요 (작품 전체 아크, 작가 편집)
├─ [이전 씬] 플롯 beat들   ← 무슨 일이 있었나
├─ [현재 씬] 플롯 beat들   ← 이 씬이 다뤄야 할 것
├─ [다음 씬] 플롯 beat들   ← 어디로 가는가
├─ 등장 엔티티 + 관계       ← 누가/어디서/어떤 사이
└─ 현재 씬 본문
```

작품이 N씬으로 커져도 이 창(window)은 항상 "개요 1 + 전/후 발췌 + 플롯 3씬 + 현재 씬"이므로 주입량이 상한에서 안정된다. 거시 흐름은 개요·플롯이, 미시 문체는 직전 씬 발췌가 담당한다.

## 컴포넌트 / 데이터 흐름

### 1. 데이터 모델 변경

마이그레이션 `0005_plot_spine.sql` 한 개 (현재 최신 마이그레이션이 `0004_node_summary_cache.sql`이므로 다음 번호는 `0005`. 구현 시 실제 마지막 번호를 재확인할 것):

```sql
ALTER TABLE beats ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN outline TEXT NOT NULL DEFAULT '';
```

- 두 컬럼 모두 `NOT NULL DEFAULT ''` → 기존 데이터 안전(파괴 없음).
- Go 구조체: `beat.Beat`에 `Description string`, `project.Project`에 `Outline string` 추가.
- 리포지토리:
  - `beat.Repo.Create` / `beat.Repo.Update`가 `description`을 다룬다. **부분 패치 규칙**: 기존 `Update`는 빈 `Label`/`Intensity==0`을 "변경 없음"으로 취급한다. `description`도 같은 사상을 따르되, 빈 문자열로 명시적 클리어가 가능해야 하므로 `*string`(nil=변경없음, ""=클리어) 패턴을 사용한다. (Label/Intensity의 기존 동작은 변경하지 않는다.)
  - `project.Repo.Update`(또는 동등 메서드)가 `outline`을 다룬다. 기존 `style_notes` 업데이트 패턴을 따른다.
- RPC params 확장:
  - `beats.create`: `NewInput`에 `description` 추가(optional).
  - `beats.update`: `UpdateInput`에 `description *string` 추가.
  - `projects.update`: `outline` 추가(기존 style_notes와 동일 취급).
- FE 동기화: `types.ts`의 `Beat`에 `description: string`, `Project`에 `outline: string`; `NewBeatInput`/`UpdateBeatInput`/프로젝트 업데이트 입력에 필드 추가; `rpc.ts` 클라이언트가 전달.

### 2. AI 주입 재편 (슬림화)

`engine/internal/ai/context.go` + `prompts.go` 변경. 섹션별 처리:

| 현재 섹션 | 변경 |
|---|---|
| `## 작품 설정` (장르/분량/시점) | 유지 |
| `## 작품 전반` (파생 synopsis) | **개요로 교체** — 작가 `outline` 우선, 비었으면 기존 synopsis 폴백 |
| `## 인근 줄거리` (다른 부/장 요약) | **제거** |
| `## 같은 장 다른 씬` | **제거** |
| `## 직전·직후 씬 발췌` | **1전 + 1후로 축소** (기존 2전+1후) |
| `## 관련 과거 씬` (토폴로지 RAG) | **유지, 3→2개로 축소** |
| `## 현재 씬` (전문) | 유지 |
| `## 선택 영역` | 유지 |
| `## 활성 스토리라인` (현재 씬 beat만) | **`## 플롯`으로 확장** (아래) |
| (없음) | **`## 관계` 신설** (아래) |
| `## 등장 인물·장소` | 유지 |
| `## 작가 주석` / `## 작가 메모` / `## 작가의 지시` | 유지 |

**새 `## 플롯` 섹션:**
- 문서 순서(DFS leaf order)로 현재 leaf의 **직전 leaf / 현재 leaf / 직후 leaf** 3개를 잡는다.
- 각 leaf에 묶인 beat(`node_id` = 그 leaf)를 thread별로 묶어, `label` + `description`으로 렌더한다.
- 렌더 형식 예:
  ```
  ## 플롯
  [이전 씬]
    [메인플롯] #3 재회 — 십 년 만에 항구에서 마주친다. 서로 못 알아본 척.
  [현재 씬]
    [메인플롯] #4 정체 발각 — 편지로 신분이 드러난다.
    [복수극] #1 결심 — 주인공이 복수를 다짐.
  [다음 씬]
    [메인플롯] #5 추격 시작
  ```
- **상한**: 플롯 섹션 전체 char budget(상수, 예: 2000자). 초과 시 `description`부터 잘라낸다(`label`·thread명은 보존).
- beat가 없는 씬 구역은 생략. 직전/직후 leaf가 없으면(첫/마지막 씬) 그 구역 생략.

**새 `## 관계` 섹션:**
- 현재 씬에 등장(mention)한 엔티티들 사이의 `relationship`만 로드해 렌더한다.
- 형식 예: `- 주인공 ↔ 적대자: 라이벌` (양방향 pair는 한 줄로). `notes`가 있으면 ` — {notes}` 덧붙임.
- 등장하지 않은 엔티티 간 관계는 제외(주입 상한 관리).

**엔진 구현 포인트:**
- `findNextLeaf` 추가(이미 `findPreviousLeaf` 존재) — DFS leaf 인덱스 +1, 마지막이면 nil.
- `loadActiveThreads` → `loadPlotSpine`로 재작성: 현재 씬에 묶인 thread만 보던 것을, 전/현/후 leaf의 beat를 모두 모으도록 확장. thread 메타(name/color)는 beat→thread 조회로 채운다. **thread의 open/closed 상태와 무관하게 해당 씬에 묶인 beat는 모두 포함**한다(닫힌 thread는 종결된 줄거리일 뿐, 그 과거 비트는 플롯 기록으로 남아야 함). 즉 플롯 스파인은 "씬에 묶인 beat" 기준이지 "열린 thread" 기준이 아니다.
- 관계: `relationship.Repo`로 프로젝트 관계를 로드 후, 현재 씬 등장 엔티티 id 집합으로 필터.
- `CountsFromContext` / `PreviewCounts` 갱신: 제거된 항목(other part/chapter, same chapter) 제거, 플롯(전/현/후 beat 수)·관계 카운트 추가.
- FE `AIContextChecklist` 항목 갱신: 제거 항목 빼고 "플롯 (전/현/후 씬)", "관계" 추가.

### 3. 인라인 플롯 패널 (FE)

집필 화면 우측 `ContextPanel`의 기존 `ActiveThreadsPanel`을 **`PlotPanel`로 확장**(별도 라우트/스플릿 없음).

패널 구성(위→아래, 문서 순서):
```
플롯
─────────────────────────
▸ 개요                       ← 접힘 기본. 펼치면 textarea 편집 + 디바운스 자동 저장
─────────────────────────
이전 씬 · {label}             ← 읽기 전용, 옅은 색
  · [메인플롯] 재회
      십 년 만에 항구에서…      ← description 1~2줄 미리보기

현재 씬                        ← 편집 영역(강조)
  · [메인플롯] 정체 발각  [✎]
      편지로 신분이 드러난다.
  · [복수극] 결심        [✎]
  + 비트 추가                  ← thread 선택 + label → 현재 씬에 묶인 beat 생성

다음 씬 · {label}             ← 읽기 전용, 옅은 색
  · [메인플롯] 추격 시작
  + 다음 씬에 비트 추가         ← 다음 leaf에 묶인 beat 생성(미리 계획용)
```

상호작용:
- **개요**: 접이식. 펼치면 textarea, blur/디바운스 시 `projects.update({outline})` 저장(기존 style_notes 저장 패턴 재사용).
- **비트 추가(현재 씬)**: 인라인 입력 — 열린 thread 드롭다운 + label. 생성 시 `node_id`=현재 씬. description은 생성 후 [✎]로 채움.
- **비트 편집 [✎]**: 인라인 확장 — label + description(textarea) + intensity(1~3) → `beats.update`.
- **이전/다음 씬 beat**: 읽기 전용 표시. "다음 씬에 비트 추가"만 허용(미리 계획용).
- 데이터 로드: 새 RPC `plot.spine_panel(node_id)` 한 번으로 전·현·후 3씬 묶음을 받는다(왕복 최소화).

ThreadSheet 보강:
- 기존 beat 목록 각 항목에 **description textarea** 추가(현재 label+intensity만).
- ThreadView(타임라인 라우트)는 이번 범위에서 손대지 않는다.

### 4. 새 RPC: `plot.spine_panel`

- 파라미터: `{ node_id: string }`
- 반환:
  ```
  {
    prev:    SceneBeats | null,
    current: SceneBeats,
    next:    SceneBeats | null
  }
  SceneBeats = {
    node_id: string,
    label:   string,
    beats: [{
      id, thread_id, thread_name, thread_color,
      label, description, intensity, ordinal
    }]
  }
  ```
- 패널 표시 전용. AI 주입은 엔진 내부 `loadPlotSpine`가 별도 처리하되, **둘 다 같은 prev/next leaf 탐색 로직을 공유**(중복·불일치 방지).

## 에러 처리

- `plot.spine_panel`: 빈 `node_id` → InvalidParams. 컨테이너(leaf 아님) id → current만 채우고 prev/next는 leaf 탐색 결과대로(없으면 null).
- 직전/직후 leaf 없음(첫/마지막 씬) → 해당 필드 `null`, 패널은 그 구역 생략. beat 0개 → `beats: []`(널 아님).
- 개요 미설정 → 빈 문자열, AI엔 synopsis 폴백.
- 고아 beat(`node_id` null) → 패널·주입 모두 무시(기존 `ListByNode`가 제외).
- 플롯 섹션 char budget 초과 → description부터 truncate, label·thread명 보존.
- FE 저장 실패(개요/비트) → 인라인/토스트 에러, 낙관적 업데이트 롤백. 빠른 씬 전환 시 cancelled-guard(Plan 18 패턴) 적용.

## 테스트 전략

엔진(Go TDD):
- `beat` repo: description CRUD; Update 부분 패치 — `description=nil`은 변경 없음, `description=""`(포인터)는 클리어. 기존 Label/Intensity 동작 불변 확인.
- `project` repo: outline update.
- `findNextLeaf`: DFS +1, 마지막 leaf 경계.
- `loadPlotSpine`: 전/현/후 leaf beat 수집, 문서 순서, 첫/마지막 씬 경계(null), beat 0개, char budget truncate(description부터).
- 관계 주입: 현재 씬 등장 엔티티 쌍만, 미등장 엔티티 관계 제외.
- `prompts.go`: `## 플롯`/`## 관계` 렌더, 제거 섹션(other part/chapter, same chapter) 미출력, 개요 폴백(outline 우선, 없으면 synopsis).
- `CountsFromContext`: 갱신된 카운트.
- handler `plot.spine_panel`: 정상/빈 id/컨테이너 id/첫·마지막 씬 경계.

FE(테스트 인프라 없음):
- `npx tsc --noEmit` 클린 + 수동 스모크: 다중 씬 프로젝트에서 전/현/후 beat 표시, 개요 편집 저장, 비트 추가·편집·description 입력, AI 생성 시 플롯·관계 주입을 컨텍스트 체크리스트로 확인.

검증 스윕: `go test ./...` 전 통과 + 엔진 빌드(repo root에서 `build-engine.sh`).

## 범위 밖 (이번 플랜 아님)

- 문체·톤·서식 일관성 규칙 시스템(서브시스템 ②)
- 일관성 검증(서브시스템 ③)
- AI 대화로 설정 관리(서브시스템 ④)
- ThreadView 타임라인 라우트의 description 시각화(추후 hover 툴팁 정도만 검토)
- 개요의 구조화(막/섹션) — 단일 텍스트로 시작

## 성공 기준

1. beat에 description, project에 outline이 저장·편집된다.
2. AI 생성 시 컨텍스트가 "개요 + 전/현/후 플롯 + 관계 + 슬림화된 발췌/RAG"로 구성되고, 제거 대상 섹션이 출력되지 않는다.
3. 작품 길이와 무관하게 플롯·개요 주입이 char budget 상한 안에서 안정적이다.
4. 우측 인라인 패널에서 현재 씬 beat를 description까지 보고/편집하고, 전·후 씬 beat를 글로 확인하며, 개요를 펼쳐 편집할 수 있다.
5. `go test ./...` 전 통과, `tsc --noEmit` 클린, 엔진 빌드 성공.
