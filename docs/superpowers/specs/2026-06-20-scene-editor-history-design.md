# 씬 편집기 히스토리 — 설계 문서

- **날짜**: 2026-06-20
- **상태**: 승인됨 (구현 계획 대기)
- **범위**: 씬(leaf node) 편집기의 버전 히스토리 — 롤백, 컴패니언 체크포인트, 아이들 자동 체크포인트

---

## 1. 배경

씬 편집기에 히스토리 기능이 필요하다. 세 가지 요구사항:

1. **이전 버전 롤백** — 특정 시점 스냅샷으로 복원
2. **컴패니언(AI) 사용 전후 체크포인트** — AI 변경을 되돌릴 수 있어야 함
3. **아이들 타임 자동 체크포인트** — 편집 중 입력이 멈추면 자동 스냅샷

탐색 결과, 요구의 상당 부분이 **이미 구현되어 있다**. 따라서 본 작업은 0→1 신규 구축이 아니라 **기존 스냅샷 인프라의 확장 + 정리**다.

### 기존 인프라 (재사용)

- `node_snapshots` 테이블 (`engine/internal/store/migrations/0001_init.sql`): `id, node_id, content_doc, reason, created_at`
- `engine/internal/snapshot/` — `snapshot.go`(reason 상수/`ValidReason`), `repo.go`(`Create`/`LatestForNode`/`ListForNode`/`GetByID`/`LatestAutosaveTime`), `retention.go`(`Thin()`)
- RPC: `snapshots.create_manual`, `snapshots.list_for_node`, `snapshots.restore` (`engine/internal/rpc/handlers/snapshots.go`)
- 프론트: `apps/desktop/src/components/VersionSheet.tsx`, `lib/rpc.ts` 래퍼, `lib/types.ts` 타입
- **자동 스냅샷**: `nodes.update_content` 핸들러(`rpc/handlers/nodes.go` 60-64행)가 저장 시 60초 간격으로 `autosave` 스냅샷 생성
- **TipTap 내장 undo/redo**: 세션 내 Ctrl+Z / Ctrl+Shift+Z (영속화 안 됨)

### 기존 갭

- 컴패니언 `set_scene_text`(`companion/tools.go`)가 스냅샷을 **전혀 생성하지 않음**
- 자동 스냅샷이 "아이들 트리거"가 아니라 "60초 간격 스로틀"
- `ai-replace` reason은 정의·검증만 되고 **아무도 생성하지 않는 죽은 코드**

---

## 2. 설계 원칙: 체크포인트 3대 테스트

새 체크포인트 종류는 아래 3개를 **모두** 통과해야만 추가한다. 하나라도 실패하면 자른다. 이는 `ai-replace` 같은 방치된 기능의 재발을 막기 위함이다.

1. **단일 생성자** — 정확히 한 코드 경로만 생성한다.
2. **고유 복원 가치** — 다른 체크포인트가 못 잡는 상태를 잡는다 (중복이면 자른다).
3. **UI 노출** — VersionSheet에 라벨이 있다. 라벨 없는 reason은 존재하지 않는다.

이 테스트를 적용한 결과, 초안의 5개 reason이 **3개로 축소**되었다.

---

## 3. 최종 reason 체계

| reason | 생성자 (단일) | 리텐션 | UI 라벨 |
|---|---|---|---|
| `manual` | 사용자 "버전 저장" (기존) | 영구 | "수동 저장" |
| `autosave` | **프론트 2분 아이들 타이머** (간격 로직 대체) | 정리 대상 (`Thin()`) | "자동 저장" |
| `companion-before` ✨ | 컴패니언 적용 직전 (`tools.go`) | 영구 | "AI 적용 전" |

**제거/추가 안 함:**
- `companion-after` — 추가 안 함. 적용 직후 = 현재 에디터 상태라 고유 가치 부족(테스트 #2 실패). 이후 작업은 idle 스냅샷이, 되돌리기는 `companion-before`가 담당. "AI 원본으로 정확 복귀"가 필요하면 `manual`로 충분.
- `idle` (별도 reason) — 추가 안 함. 자동 스냅샷을 idle 트리거로 일원화하되 reason 이름은 `autosave` 재사용 → 기존 리텐션·UI 그대로, 신규 reason 0개.
- `ai-replace` — **삭제**. 현재 고아 reason. 이번 작업에서 enum/`ValidReason`에서 제거.

---

## 4. 기능별 설계

### 4.1 즉시 undo/redo (세션 휘발)

- **신규 영속화 없음.** TipTap 내장 history 그대로 사용.
- Ctrl+Z / Ctrl+Shift+Z 동작 확인. 필요 시 에디터 툴바에 undo/redo 버튼 노출 (선택).
- 앱 재시작 시 사라지는 동작이 의도된 사양.

### 4.2 컴패니언 전 체크포인트

- 위치: `engine/internal/companion/tools.go`의 `set_scene_text` 적용 경로 (이미 `before`/`after` 노드를 읽고 있음).
- 적용 **직전** 원본 doc으로 `snaps.Create(nodeID, beforeDoc, "companion-before", now)` 호출.
- **범위: 씬 텍스트(`content_doc`)만.** 컴패니언이 수정하는 플롯/엔티티/아웃라인 등 다른 테이블은 스냅샷 범위 외.
  - 코드 주석 + VersionSheet 안내문에 이 한계를 **명시**한다.
- 중복 제거: 생성 직전 `LatestForNode` 해시 비교, 동일하면 skip.

### 4.3 아이들 자동 체크포인트

- 서버측 60초 간격 autosave 로직(`rpc/handlers/nodes.go`)을 **제거**하고, 트리거를 프론트 아이들 타이머로 일원화.
- `apps/desktop/src/routes/Workspace.tsx`:
  - 키 입력마다 리셋되는 2분(`IDLE_CHECKPOINT_MS = 120_000`) 타이머.
  - 만료 시 **더티**(마지막 체크포인트 이후 변경 있음)인 경우에만 스냅샷 생성.
  - `companion-before` 생성 시 더티 플래그 리셋 → 같은 pre-AI 상태를 idle이 중복 생성하지 않음.
- 스냅샷 생성 경로: 신규 thin RPC `snapshots.create_auto(nodeId, docJSON)` — reason `autosave` 고정, 해시 중복 체크 내장. (기존 `create_manual` 핸들러와 대칭 구조, reason 파라미터를 외부에 노출하지 않아 임의 reason 주입 불가.)
- 데이터 유실 위험 없음: 800ms debounce `update_content` 저장은 그대로 유지. 스냅샷은 롤백 지점용이지 크래시 복구용이 아님.

### 4.4 롤백 UX

- 기존 `VersionSheet.tsx` 확장:
  - reason별 라벨/아이콘 구분 (수동 저장 / 자동 저장 / AI 적용 전).
  - "AI 적용 전" 항목 시각적 강조.
  - 복원은 기존 `snapshots.restore` 그대로 (변경 없음).
- 컴패니언 스냅샷이 씬 텍스트만 담는다는 안내문 노출.

---

## 5. 가드레일 (재발 방지)

- **내용 해시 중복 제거**: 자동/컴패니언 스냅샷 생성 직전 `LatestForNode`와 content 해시 비교, 같으면 생성 skip.
- **더티 플래그 리셋**: `companion-before` 생성 시 idle 더티 플래그 리셋.
- **enum-UI 동기화**: `ValidReason()`에 reason을 추가/제거하는 변경은 VersionSheet 라벨맵도 같은 변경에서 수정. 라벨 없는 reason은 리뷰에서 차단.

---

## 6. 데이터 흐름

```
[편집] → TipTap undo (메모리, 세션 휘발)
       → 800ms debounce → nodes.update_content (content_doc 저장, 스냅샷 트리거 제거)

[2분 아이들 + 더티] → snapshots.create_auto → autosave 스냅샷 (해시 중복 체크)

[컴패니언 적용] → companion.apply_ops → tools.go set_scene_text
                → companion-before 스냅샷 (적용 직전, 해시 중복 체크, idle 더티 리셋)
                → nodes.UpdateContent (AI 결과 적용)

[롤백] VersionSheet → snapshots.restore → nodes.update_content (선택 스냅샷 content_doc 복원)
```

---

## 7. 변경 파일

**엔진 (Go):**
- `engine/internal/snapshot/snapshot.go` — `ReasonAIReplace` 제거, `ReasonCompanionBefore` 추가, `ValidReason()` 갱신
- `engine/internal/snapshot/retention.go` — `Thin()`이 `autosave`만 정리(현행 유지), `companion-before`는 영구 보존 확인
- `engine/internal/companion/tools.go` — `set_scene_text` 경로에 `companion-before` 스냅샷 + 해시 중복 체크
- `engine/internal/rpc/handlers/nodes.go` — 60초 간격 autosave 스냅샷 로직 **제거**
- `engine/internal/rpc/handlers/snapshots.go` — `snapshots.create_auto` 핸들러 추가 (또는 create 일반화)
- `engine/cmd/linetta-engine/main.go` — 신규 핸들러 등록
- `manuscriptedit/replace.go` — `ai-replace` 미사용 확인(현재 `manual` 사용 중이므로 영향 없음 검증)

**프론트 (TypeScript):**
- `apps/desktop/src/routes/Workspace.tsx` — 2분 아이들 타이머, 더티 추적, create_auto 호출
- `apps/desktop/src/lib/rpc.ts` — `snapshots.createAuto` 래퍼
- `apps/desktop/src/lib/types.ts` — `SnapshotReason` 타입 갱신 (`ai-replace` 제거, `companion-before` 추가)
- `apps/desktop/src/components/VersionSheet.tsx` — reason별 라벨/아이콘, 컴패니언 범위 안내문

---

## 8. 테스트 전략

- **엔진 단위 테스트**: `companion-before` 스냅샷 생성(적용 직전 원본 캡처), 해시 중복 시 skip, `set_scene_text` 비교; autosave 간격 로직 제거 후 회귀; `ValidReason`에서 `ai-replace` 거부.
- **롤백 회귀**: 기존 `snapshots.restore` 동작 유지 검증.
- **프론트**: 아이들 타이머가 2분 후 더티일 때만 create_auto 호출, 입력 시 리셋, companion-before 후 더티 리셋(중복 방지) 검증.
- **수동 검증**: 씬 편집 → 2분 대기 → 버전 시트에 자동 저장 항목; 컴패니언 적용 → "AI 적용 전" 항목 → 복원으로 원복.

---

## 9. 범위 밖 (YAGNI)

- undo/redo 영속화 (세션 휘발이 확정 사양)
- 컴패니언의 비-씬 변경(플롯/엔티티/아웃라인) 스냅샷
- `companion-after` 스냅샷
- 별도 `idle` reason
- diff 뷰어, 버전 간 비교 UI (요구 외)
