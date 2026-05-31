import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import { settings as settingsApi, gitSync, opsStatus as opsStatusApi } from "../lib/rpc";
import type { OpsStatus, ProviderID, Settings as SettingsRow, WebSearchProvider } from "../lib/types";

const JOB_BACKUP = "backup.daily";
const JOB_GIT_SYNC = "git_sync";
const JOB_SUMMARIZER = "summarizer";
const JOB_COMPANION = "companion.persistence";

export function Settings() {
  const [current, setCurrent] = useState<SettingsRow | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedAt, setSavedAt] = useState<number | null>(null);
  const [opsRows, setOpsRows] = useState<OpsStatus[]>([]);

  // Local draft for text-input fields so the cursor doesn't bounce while
  // typing. We commit to the engine on blur (or when the folder picker
  // returns), not on every keystroke.
  const [gitDirDraft, setGitDirDraft] = useState("");
  const [gitTmplDraft, setGitTmplDraft] = useState("");
  const [webSearchKeyDraft, setWebSearchKeyDraft] = useState("");

  useEffect(() => {
    let cancelled = false;
    Promise.all([settingsApi.get(), opsStatusApi.get()])
      .then(([s, rows]) => {
        if (cancelled) return;
        setCurrent(s);
        setGitDirDraft(s.git_sync_dir);
        setGitTmplDraft(s.git_sync_commit_template);
        setWebSearchKeyDraft(s.web_search_api_key);
        setOpsRows(rows);
      })
      .catch((e) => { if (!cancelled) setError(String(e)); });
    return () => { cancelled = true; };
  }, []);

  const opsByJob = useMemo(() => {
    return new Map(opsRows.map((row) => [row.job_name, row]));
  }, [opsRows]);

  const refreshOps = async () => {
    const rows = await opsStatusApi.get();
    setOpsRows(rows);
  };

  const clearOpsError = async (jobName: string) => {
    setSaving(true);
    setError(null);
    try {
      await opsStatusApi.clearError(jobName);
      await refreshOps();
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  const apply = async (patch: Partial<SettingsRow>) => {
    if (!current) return;
    setSaving(true);
    setError(null);
    try {
      const next = await settingsApi.set(patch);
      setCurrent(next);
      setSavedAt(Date.now());
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <main className="shell">
      <p><Link to="/">← Library</Link></p>
      <h2>설정</h2>
      {error && <p className="error">{error}</p>}
      {!current ? (
        <p className="hint">불러오는 중…</p>
      ) : (
        <div className="settings-form">
          <section className="settings-section">
            <h3>AI 제공자</h3>
            <p className="hint">변경은 다음 AI 호출부터 적용됩니다.</p>
            <label className="radio-row">
              <input
                type="radio"
                name="provider"
                value="claude-code-cli"
                checked={current.provider === "claude-code-cli"}
                onChange={() => apply({ provider: "claude-code-cli" as ProviderID })}
                disabled={saving}
              />
              <span>Claude Code CLI</span>
            </label>
            <label className="radio-row">
              <input
                type="radio"
                name="provider"
                value="openai-codex"
                checked={current.provider === "openai-codex"}
                onChange={() => apply({ provider: "openai-codex" as ProviderID })}
                disabled={saving}
              />
              <span>OpenAI Codex CLI</span>
            </label>
          </section>

          <section className="settings-section">
            <h3>LLM 도구</h3>
            <p className="hint">
              cmd+j 채팅에서 web_search, web_fetch, linetta_apply_ops 도구를 사용할 수 있습니다.
              web_fetch는 키 없이 동작합니다.
            </p>
            <label className="field">
              <span>web_search 제공자</span>
              <select
                value={current.web_search_provider}
                onChange={(e) => apply({ web_search_provider: e.target.value as WebSearchProvider })}
                disabled={saving}
              >
                <option value="brave">Brave Search</option>
                <option value="perplexity">Perplexity Sonar</option>
              </select>
            </label>
            <label className="field">
              <span>web_search API 키</span>
              <input
                type="password"
                value={webSearchKeyDraft}
                onChange={(e) => setWebSearchKeyDraft(e.target.value)}
                onBlur={() => {
                  if (webSearchKeyDraft !== current.web_search_api_key) {
                    apply({ web_search_api_key: webSearchKeyDraft });
                  }
                }}
                placeholder={current.web_search_provider === "perplexity" ? "pplx-..." : "BSA..."}
                autoComplete="off"
              />
            </label>
            <p className="hint">키는 로컬 settings.json에 저장됩니다.</p>
          </section>

          <section className="settings-section">
            <h3>에디터</h3>
            <label className="check-row">
              <input
                type="checkbox"
                checked={current.typewriter_default}
                onChange={(e) => apply({ typewriter_default: e.target.checked })}
                disabled={saving}
              />
              <span>새 씬을 열 때 타이프라이터 스크롤 켜기</span>
            </label>
            <label className="check-row">
              <input
                type="checkbox"
                checked={current.focus_default}
                onChange={(e) => apply({ focus_default: e.target.checked })}
                disabled={saving}
              />
              <span>새 씬을 열 때 Focus 모드(현재 단락 외 디밍) 켜기</span>
            </label>
          </section>

          <section className="settings-section">
            <h3>GitHub 동기화</h3>
            <p className="hint">
              하루 한 번 모든 작품을 마크다운으로 내보내 지정한 git 폴더에 커밋·푸시합니다.
              경로를 비워두면 비활성화됩니다. 인증은 시스템 git 설정(SSH 키, 자격 증명 도우미)을 그대로 사용합니다.
            </p>
            <label className="field">
              <span>git 폴더</span>
              <div className="field-row">
                <input
                  type="text"
                  value={gitDirDraft}
                  onChange={(e) => setGitDirDraft(e.target.value)}
                  onBlur={() => {
                    if (gitDirDraft !== current.git_sync_dir) {
                      apply({ git_sync_dir: gitDirDraft });
                    }
                  }}
                  placeholder="예: /Users/me/notes/linetta"
                />
                <button
                  type="button"
                  onClick={async () => {
                    const picked = await openDialog({ directory: true, multiple: false });
                    if (typeof picked === "string") {
                      setGitDirDraft(picked);
                      await apply({ git_sync_dir: picked });
                    }
                  }}
                  disabled={saving}
                >
                  폴더 선택…
                </button>
              </div>
            </label>
            <label className="field">
              <span>커밋 메시지</span>
              <input
                type="text"
                value={gitTmplDraft}
                onChange={(e) => setGitTmplDraft(e.target.value)}
                onBlur={() => {
                  if (gitTmplDraft !== current.git_sync_commit_template) {
                    apply({ git_sync_commit_template: gitTmplDraft });
                  }
                }}
                placeholder="Linetta sync {date}"
              />
            </label>
            <p className="hint">
              <code>{"{date}"}</code> 자리표시자만 지원됩니다 (YYYY-MM-DD HH:MM 으로 치환).
            </p>
            <p className="hint">
              지정한 폴더가 아직 git 저장소가 아닐 때 아래 버튼으로 초기화할 수 있습니다.
              초기화 후 <code>git remote add origin &lt;URL&gt;</code> 로 GitHub remote를 추가하세요.
            </p>
            <button
              type="button"
              onClick={async () => {
                try {
                  const res = await gitSync.init();
                  if (res.skipped) {
                    setError("git 폴더를 먼저 지정하세요");
                    return;
                  }
                  if (res.error) {
                    setError(`초기화 실패: ${res.error}`);
                    return;
                  }
                  if (res.already_repo) {
                    setSavedAt(Date.now());
                    setError("이미 git 저장소입니다");
                    return;
                  }
                  if (res.created) {
                    setError(null);
                    setSavedAt(Date.now());
                  }
                } catch (e) {
                  setError(String(e));
                }
              }}
              disabled={saving}
            >
              이 폴더를 git 저장소로 초기화
            </button>
            <OpsStatusCard
              title="최근 Git 동기화"
              status={opsByJob.get(JOB_GIT_SYNC)}
              okText="Git 동기화 성공"
              idleText="아직 동기화 기록 없음"
              onClearError={() => clearOpsError(JOB_GIT_SYNC)}
              disabled={saving}
            />
          </section>

          <section className="settings-section">
            <h3>백업</h3>
            <p className="hint">하루 한 번 자동 백업이 다음 경로에 저장됩니다 (14일 보관).</p>
            <p className="backup-path"><code>{current.backup_dir}</code></p>
            <OpsStatusCard
              title="최근 백업 상태"
              status={opsByJob.get(JOB_BACKUP)}
              okText="백업 성공"
              idleText="아직 백업 기록 없음"
              onClearError={() => clearOpsError(JOB_BACKUP)}
              disabled={saving}
            />
          </section>

          {isDegraded(opsByJob.get(JOB_SUMMARIZER)) && (
            <section className="settings-section">
              <h3>요약기 상태</h3>
              <OpsStatusCard
                title="최근 요약 실패"
                status={opsByJob.get(JOB_SUMMARIZER)}
                okText="요약 정상"
                idleText="최근 요약 기록 없음"
                onClearError={() => clearOpsError(JOB_SUMMARIZER)}
                disabled={saving}
              />
            </section>
          )}

          {isDegraded(opsByJob.get(JOB_COMPANION)) && (
            <section className="settings-section">
              <h3>Companion 기록 상태</h3>
              <OpsStatusCard
                title="최근 기록 실패"
                status={opsByJob.get(JOB_COMPANION)}
                okText="기록 정상"
                idleText="최근 기록 없음"
                onClearError={() => clearOpsError(JOB_COMPANION)}
                disabled={saving}
              />
            </section>
          )}

          {savedAt && <p className="settings-saved">저장됨</p>}
        </div>
      )}
    </main>
  );
}

function isDegraded(status?: OpsStatus): boolean {
  return Boolean(status?.last_error);
}

function OpsStatusCard({
  title,
  status,
  okText,
  idleText,
  onClearError,
  disabled,
}: {
  title: string;
  status?: OpsStatus;
  okText: string;
  idleText: string;
  onClearError: () => void;
  disabled: boolean;
}) {
  const metadata = parseMetadata(status?.metadata_json);
  const failed = Boolean(status?.last_error);
  const body = failed ? status?.last_error : status?.last_ok ? okText : idleText;
  const finished = formatMillis(status?.last_finished_at);
  const metadataLabel = formatMetadata(metadata);

  return (
    <div className={`ops-status ${failed ? "is-error" : status?.last_ok ? "is-ok" : ""}`}>
      <div className="ops-status-head">
        <h4>{title}</h4>
        {failed && (
          <button type="button" onClick={onClearError} disabled={disabled}>
            오류 지우기
          </button>
        )}
      </div>
      <p className="ops-status-line">{body}</p>
      {finished && <p className="hint">마지막 완료: {finished}</p>}
      {metadataLabel && <p className="hint">{metadataLabel}</p>}
    </div>
  );
}

function parseMetadata(raw?: string): Record<string, unknown> {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }
  return {};
}

function formatMillis(value?: number): string {
  if (!value) return "";
  return new Date(value).toLocaleString("ko-KR");
}

function formatMetadata(metadata: Record<string, unknown>): string {
  const parts: string[] = [];
  if (typeof metadata.files_written === "number") {
    parts.push(`파일 ${metadata.files_written}개`);
  }
  if (metadata.committed === true) parts.push("커밋 완료");
  if (metadata.pushed === true) parts.push("푸시 완료");
  if (metadata.backup_ran === true) parts.push("새 백업 생성");
  if (typeof metadata.failure_count === "number" && metadata.failure_count > 0) {
    parts.push(`연속 실패 ${metadata.failure_count}회`);
  }
  if (typeof metadata.path === "string" && metadata.path !== "") {
    parts.push(metadata.path);
  }
  return parts.join(" · ");
}
