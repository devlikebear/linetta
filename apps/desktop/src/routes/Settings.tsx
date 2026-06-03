import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { ChevronLeft, ExternalLink } from "lucide-react";
import { open as openDialog } from "@tauri-apps/plugin-dialog";
import {
  settings as settingsApi,
  gitSync,
  opsStatus as opsStatusApi,
  providers as providersApi,
} from "../lib/rpc";
import "./Settings.css";
import type {
  OpsStatus,
  ProviderConfig,
  ProviderID,
  Settings as SettingsRow,
  WebSearchProvider,
} from "../lib/types";

const JOB_BACKUP = "backup.daily";
const JOB_GIT_SYNC = "git_sync";
const JOB_SUMMARIZER = "summarizer";
const JOB_COMPANION = "companion.persistence";

interface ProviderMeta {
  id: ProviderID;
  label: string;
  desc: string;
  /** "key" => API key field, "cli" => CLI path field, "oauth" => no field (uses OAuth login). */
  credential: "key" | "cli" | "oauth";
  /** Whether a custom base URL may be set (OpenAI/Anthropic-compatible endpoints). */
  endpoint?: boolean;
  legacy?: boolean;
}

const PROVIDERS: ProviderMeta[] = [
  { id: "openai-codex", label: "ChatGPT 계정 (OpenAI Codex)", desc: "공식 Codex 로그인으로 연결 · API 키 복붙 없음", credential: "oauth" },
  { id: "openai", label: "OpenAI API", desc: "OpenAI API 키 또는 호환 엔드포인트(Kimi, MiniMax 등)", credential: "key", endpoint: true },
  { id: "anthropic", label: "Claude API", desc: "Anthropic Console API 키로 연결", credential: "key", endpoint: true },
  { id: "gemini-native", label: "Gemini API", desc: "Google AI Studio API 키로 연결", credential: "key", endpoint: true },
  { id: "claude-code-cli", label: "Claude Code CLI (기존/고급)", desc: "기존 설정 유지용 · 신규 사용자는 Claude API 키 권장", credential: "cli", legacy: true },
];

type GuideID = "chatgpt-subscription" | "openai-api" | "claude-api" | "gemini-api";

interface SetupGuide {
  id: GuideID;
  provider: ProviderID;
  title: string;
  badge: string;
  summary: string;
  policy: string;
  steps: string[];
  action: string;
  links: Array<{ label: string; href: string }>;
}

const SETUP_GUIDES: SetupGuide[] = [
  {
    id: "chatgpt-subscription",
    provider: "openai-codex",
    title: "ChatGPT 구독으로 연결",
    badge: "가장 쉬움",
    summary: "ChatGPT 계정을 OpenAI Codex에 로그인해 Linetta에서 사용합니다.",
    policy: "구독형 연결은 OpenAI Codex 공식 로그인 경로만 지원합니다.",
    action: "ChatGPT 계정 선택",
    steps: [
      "OpenAI Codex를 설치합니다.",
      "터미널에서 codex를 실행합니다.",
      "처음 실행할 때 뜨는 로그인 안내에서 ChatGPT 계정으로 인증합니다.",
      "Linetta에서 이 연결 방식을 선택합니다.",
      "아래 연결 테스트를 눌러 짧은 응답이 오는지 확인합니다.",
    ],
    links: [
      { label: "OpenAI Codex CLI 안내", href: "https://developers.openai.com/codex/cli" },
      { label: "ChatGPT와 API 과금 차이", href: "https://help.openai.com/en/articles/9039756-managing-billing-settings-on-chatgpt-web-and-platform" },
    ],
  },
  {
    id: "openai-api",
    provider: "openai",
    title: "OpenAI API 키로 연결",
    badge: "직접 과금",
    summary: "OpenAI Platform에서 API 키를 만든 뒤 Linetta에 붙여넣습니다.",
    policy: "ChatGPT 구독과 API 사용량 과금은 별도일 수 있습니다.",
    action: "OpenAI API 선택",
    steps: [
      "OpenAI Platform의 API Keys 페이지를 엽니다.",
      "새 secret key를 만들고 한 번만 표시되는 키를 복사합니다.",
      "Linetta에서 OpenAI API를 선택합니다.",
      "아래 API 키 입력란에 붙여넣고 저장합니다.",
      "모델 새로고침 또는 직접 입력 후 연결 테스트를 누릅니다.",
    ],
    links: [
      { label: "OpenAI API 키 만들기", href: "https://help.openai.com/en/articles/4936850-where-do-i-find-my-openai-api-key" },
      { label: "OpenAI API 가격", href: "https://openai.com/api/pricing/" },
    ],
  },
  {
    id: "claude-api",
    provider: "anthropic",
    title: "Claude API 키로 연결",
    badge: "API 전용",
    summary: "Claude 구독 로그인 대신 Anthropic Console API 키를 사용합니다.",
    policy: "Claude 구독 하네스는 Linetta에서 지원하지 않습니다. 정책 리스크를 피하기 위해 API 키만 안내합니다.",
    action: "Claude API 선택",
    steps: [
      "Anthropic Console에 접속합니다.",
      "Billing 또는 크레딧 설정을 완료합니다.",
      "API Keys 메뉴에서 새 키를 만들고 복사합니다.",
      "Linetta에서 Claude API를 선택합니다.",
      "아래 API 키 입력란에 붙여넣고 모델을 선택한 뒤 연결 테스트를 누릅니다.",
    ],
    links: [
      { label: "Claude API 접근 안내", href: "https://support.claude.com/en/articles/8114521-how-can-i-access-the-claude-api" },
      { label: "Claude API 인증 문서", href: "https://platform.claude.com/docs/en/manage-claude/authentication" },
      { label: "Claude Code 정책 참고", href: "https://code.claude.com/docs/en/legal-and-compliance" },
    ],
  },
  {
    id: "gemini-api",
    provider: "gemini-native",
    title: "Gemini API 키로 연결",
    badge: "API 전용",
    summary: "Google AI Studio에서 Gemini API 키를 만든 뒤 Linetta에 붙여넣습니다.",
    policy: "Gemini/Google AI 구독 로그인은 Linetta에서 지원하지 않습니다. API 키 연결만 안내합니다.",
    action: "Gemini API 선택",
    steps: [
      "Google AI Studio의 API Keys 페이지를 엽니다.",
      "기존 프로젝트를 선택하거나 새 프로젝트와 API 키를 만듭니다.",
      "무료 tier 또는 유료 billing 상태를 확인합니다.",
      "Linetta에서 Gemini API를 선택합니다.",
      "아래 API 키 입력란에 붙여넣고 모델을 선택한 뒤 연결 테스트를 누릅니다.",
    ],
    links: [
      { label: "Gemini API 키 만들기", href: "https://ai.google.dev/gemini-api/docs/api-key" },
      { label: "Gemini API 과금 안내", href: "https://ai.google.dev/gemini-api/docs/billing" },
    ],
  },
];

function guideForProvider(provider: ProviderID): GuideID {
  switch (provider) {
    case "openai":
      return "openai-api";
    case "anthropic":
      return "claude-api";
    case "gemini-native":
      return "gemini-api";
    case "openai-codex":
    case "claude-code-cli":
    default:
      return "chatgpt-subscription";
  }
}

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

  // Per-provider config drafts (re-synced when the active provider changes).
  const [modelDraft, setModelDraft] = useState("");
  const [apiKeyDraft, setApiKeyDraft] = useState("");
  const [baseUrlDraft, setBaseUrlDraft] = useState("");
  const [cliPathDraft, setCliPathDraft] = useState("");
  const [modelOptions, setModelOptions] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [cliDetecting, setCliDetecting] = useState(false);
  const [cliDetectMsg, setCliDetectMsg] = useState<string | null>(null);
  const [guideId, setGuideId] = useState<GuideID>("chatgpt-subscription");
  const [providerTesting, setProviderTesting] = useState(false);
  const [providerTestMsg, setProviderTestMsg] = useState<{ kind: "ok" | "error"; text: string } | null>(null);

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

  // Reset the per-provider drafts whenever the active provider changes (or on
  // first load) so each provider's stored config shows in the fields.
  const activeProvider = current?.provider;
  useEffect(() => {
    if (!current) return;
    const pc = current.providers?.[current.provider] ?? {};
    setGuideId(guideForProvider(current.provider));
    setModelDraft(pc.model ?? "");
    setApiKeyDraft(pc.api_key ?? "");
    setBaseUrlDraft(pc.base_url ?? "");
    setCliPathDraft(pc.cli_path ?? "");
    setModelOptions([]);
    setModelsError(null);
    setCliDetectMsg(null);
    setProviderTestMsg(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeProvider]);

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

  const applyProviderConfig = async (id: ProviderID, partial: ProviderConfig) => {
    if (!current) return;
    const existing = current.providers?.[id] ?? {};
    await apply({ providers: { [id]: { ...existing, ...partial } } } as Partial<SettingsRow>);
  };

  const detectCliPath = async (id: ProviderID) => {
    setCliDetecting(true);
    setCliDetectMsg(null);
    try {
      const { path } = await providersApi.detectCli();
      if (path) {
        setCliPathDraft(path);
        await applyProviderConfig(id, { cli_path: path });
        setCliDetectMsg(`찾음: ${path}`);
      } else {
        setCliDetectMsg("claude 실행 파일을 찾지 못했습니다. 경로를 직접 입력하세요.");
      }
    } catch (e) {
      setCliDetectMsg(String(e));
    } finally {
      setCliDetecting(false);
    }
  };

  const fetchModels = async (id: ProviderID) => {
    setModelsLoading(true);
    setModelsError(null);
    try {
      const res = await providersApi.listModels(id);
      setModelOptions(res.models);
    } catch (e) {
      setModelsError(String(e));
    } finally {
      setModelsLoading(false);
    }
  };

  const persistActiveProviderDrafts = async (meta: ProviderMeta) => {
    if (!current) return;
    const stored = current.providers?.[meta.id] ?? {};
    const patch: ProviderConfig = {};
    if (modelDraft !== (stored.model ?? "")) {
      patch.model = modelDraft;
    }
    if (meta.credential === "key" && apiKeyDraft !== (stored.api_key ?? "")) {
      patch.api_key = apiKeyDraft;
    }
    if (meta.endpoint && baseUrlDraft !== (stored.base_url ?? "")) {
      patch.base_url = baseUrlDraft;
    }
    if (meta.credential === "cli" && cliPathDraft !== (stored.cli_path ?? "")) {
      patch.cli_path = cliPathDraft;
    }
    if (Object.keys(patch).length > 0) {
      await applyProviderConfig(meta.id, patch);
    }
  };

  const testActiveProvider = async (meta: ProviderMeta) => {
    setProviderTesting(true);
    setProviderTestMsg(null);
    try {
      await persistActiveProviderDrafts(meta);
      const res = await providersApi.test(meta.id);
      setProviderTestMsg({ kind: "ok", text: `연결 성공: ${res.message}` });
    } catch (e) {
      setProviderTestMsg({ kind: "error", text: `연결 실패: ${String(e)}` });
    } finally {
      setProviderTesting(false);
    }
  };

  const activeMeta = current ? PROVIDERS.find((m) => m.id === current.provider) : undefined;
  const activeConfig = current?.providers?.[current.provider] ?? {};
  const selectedGuide = SETUP_GUIDES.find((g) => g.id === guideId) ?? SETUP_GUIDES[0];
  const credentialState = getCredentialState(activeMeta, activeConfig);
  const webSearchKeyPlaceholder = current ? getWebSearchKeyPlaceholder(current) : "BSA...";

  return (
    <div className="settings">
      <div className="lib-top">
        <Link to="/" className="btn ghost sm">
          <ChevronLeft size={15} /> 라이브러리
        </Link>
        <div className="lib-brandmark">설정</div>
        <span style={{ width: 90 }} />
      </div>
      <div className="settings-inner">
        <h1>설정</h1>
        {error && <p className="error">{error}</p>}
        {!current ? (
          <p className="hint">불러오는 중…</p>
        ) : (
          <>
            <section className="settings-section">
              <div className="settings-section-head">
                <h3>AI 연결 마법사</h3>
                <span className="setup-pill">초보자용</span>
              </div>
              <p className="sd">
                API 키나 모델 이름을 몰라도 괜찮습니다. 아래에서 쓰고 싶은 AI를 고르면 Linetta가 필요한 단계만 보여줍니다.
              </p>
              <div className="setup-current">
                <span>
                  현재 선택: <strong>{activeMeta?.label ?? current.provider}</strong>
                </span>
                <span>{credentialState}</span>
              </div>
              <div className="setup-policy">
                Claude와 Gemini 구독 로그인은 각 회사의 공식 제품 안에서 쓰는 흐름이라 Linetta에서는 제공하지 않습니다.
                ChatGPT 구독은 OpenAI Codex 공식 로그인만 지원하고, Claude/Gemini는 API 키로 연결합니다.
              </div>
              <div className="setup-choice-list">
                {SETUP_GUIDES.map((guide) => (
                  <button
                    key={guide.id}
                    type="button"
                    className={`setup-choice${guide.id === guideId ? " is-selected" : ""}`}
                    onClick={() => setGuideId(guide.id)}
                  >
                    <span className="setup-choice-main">
                      <span className="setup-choice-title">{guide.title}</span>
                      <span className="setup-choice-summary">{guide.summary}</span>
                    </span>
                    <span className="setup-choice-badge">{guide.badge}</span>
                  </button>
                ))}
              </div>
              <div className="setup-guide">
                <div className="setup-guide-head">
                  <div>
                    <h4>{selectedGuide.title}</h4>
                    <p>{selectedGuide.policy}</p>
                  </div>
                  <button
                    type="button"
                    className="btn sm"
                    onClick={() => !saving && apply({ provider: selectedGuide.provider })}
                    disabled={saving}
                  >
                    {current.provider === selectedGuide.provider ? "선택됨" : selectedGuide.action}
                  </button>
                </div>
                <ol className="setup-steps">
                  {selectedGuide.steps.map((step) => (
                    <li key={step}>{step}</li>
                  ))}
                </ol>
                <div className="setup-links" aria-label={`${selectedGuide.title} 공식 가이드`}>
                  {selectedGuide.links.map((link) => (
                    <a key={link.href} href={link.href} target="_blank" rel="noreferrer">
                      <ExternalLink size={13} /> {link.label}
                    </a>
                  ))}
                </div>
              </div>
            </section>

            <section className="settings-section">
              <h3>고급 AI 설정</h3>
              <p className="sd">
                마법사에서 선택한 연결 방식의 실제 provider, API 키, 모델 값을 조정합니다.
                초보자는 위 순서대로 진행한 뒤 필요한 칸만 채우면 됩니다.
              </p>
              {PROVIDERS.map((meta) => (
                <button
                  key={meta.id}
                  type="button"
                  className={`set-row set-row-btn${meta.legacy ? " is-legacy" : ""}`}
                  onClick={() => !saving && apply({ provider: meta.id })}
                  disabled={saving}
                >
                  <span className="sk-wrap">
                    <span className="sk">{meta.label}</span>
                    <span className="sd">{meta.desc}</span>
                  </span>
                  <span className={`switch${current.provider === meta.id ? " on" : ""}`} />
                </button>
              ))}
              <p className="sd">변경은 다음 AI 호출부터 적용됩니다.</p>

              {(() => {
                const meta = PROVIDERS.find((m) => m.id === current.provider);
                if (!meta) return null;
                return (
                  <div className="provider-config">
                    {meta.credential === "key" && (
                      <div className="modal-field">
                        <label htmlFor="provider-key">API 키</label>
                        <div className="set-field-row">
                          <input
                            id="provider-key"
                            type="password"
                            value={apiKeyDraft}
                            onChange={(e) => setApiKeyDraft(e.target.value)}
                            onBlur={() => {
                              if (apiKeyDraft !== "") {
                                applyProviderConfig(meta.id, { api_key: apiKeyDraft });
                              }
                            }}
                            placeholder={activeConfig.api_key_set ? "저장된 API 키 있음 · 새 키를 붙여넣으면 교체" : "sk-..."}
                            autoComplete="off"
                          />
                          {activeConfig.api_key_set && (
                            <button
                              type="button"
                              className="btn ghost sm"
                              onClick={() => {
                                setApiKeyDraft("");
                                applyProviderConfig(meta.id, { clear_api_key: true });
                              }}
                              disabled={saving}
                            >
                              키 삭제
                            </button>
                          )}
                        </div>
                        <p className="sd">
                          저장된 키는 다시 표시하지 않습니다. 새 키를 입력하면 macOS Keychain의 기존 키를 교체합니다.
                        </p>
                      </div>
                    )}
                    {meta.endpoint && (
                      <div className="modal-field">
                        <label htmlFor="provider-base-url">Base URL (선택)</label>
                        <input
                          id="provider-base-url"
                          type="text"
                          value={baseUrlDraft}
                          onChange={(e) => setBaseUrlDraft(e.target.value)}
                          onBlur={() => {
                            const stored = current.providers?.[meta.id]?.base_url ?? "";
                            if (baseUrlDraft !== stored) {
                              applyProviderConfig(meta.id, { base_url: baseUrlDraft });
                            }
                          }}
                          placeholder="비우면 기본 엔드포인트. 호환 API 예: https://api.minimax.io/v1"
                        />
                      </div>
                    )}
                    {meta.credential === "cli" && (
                      <div className="modal-field">
                        <label htmlFor="provider-cli">CLI 경로 (선택)</label>
                        <div className="set-field-row">
                          <input
                            id="provider-cli"
                            type="text"
                            value={cliPathDraft}
                            onChange={(e) => setCliPathDraft(e.target.value)}
                            onBlur={() => {
                              const stored = current.providers?.[meta.id]?.cli_path ?? "";
                              if (cliPathDraft !== stored) {
                                applyProviderConfig(meta.id, { cli_path: cliPathDraft });
                              }
                            }}
                            placeholder="PATH에서 claude를 못 찾을 때 실행 파일 경로"
                          />
                          <button
                            type="button"
                            className="btn ghost sm"
                            onClick={() => detectCliPath(meta.id)}
                            disabled={saving || cliDetecting}
                          >
                            {cliDetecting ? "찾는 중…" : "자동 찾기"}
                          </button>
                        </div>
                        <p className="sd">
                          PATH·로그인 셸·Homebrew/npm 설치 위치에서 claude를 자동으로 찾습니다.
                        </p>
                        {cliDetectMsg && <p className="sd">{cliDetectMsg}</p>}
                      </div>
                    )}
                    <div className="modal-field">
                      <label htmlFor="provider-model">모델</label>
                      <div className="set-field-row">
                        <input
                          id="provider-model"
                          type="text"
                          list="provider-model-options"
                          value={modelDraft}
                          onChange={(e) => setModelDraft(e.target.value)}
                          onBlur={() => {
                            const stored = current.providers?.[meta.id]?.model ?? "";
                            if (modelDraft !== stored) {
                              applyProviderConfig(meta.id, { model: modelDraft });
                            }
                          }}
                          placeholder="비우면 기본 모델 사용"
                        />
                        <datalist id="provider-model-options">
                          {modelOptions.map((m) => (
                            <option key={m} value={m} />
                          ))}
                        </datalist>
                        <button
                          type="button"
                          className="btn ghost sm"
                          onClick={() => fetchModels(meta.id)}
                          disabled={saving || modelsLoading || meta.id === "claude-code-cli" || meta.credential === "oauth"}
                        >
                          {modelsLoading ? "불러오는 중…" : "모델 새로고침"}
                        </button>
                      </div>
                      {meta.id === "claude-code-cli" ? (
                        <p className="sd">Claude Code CLI는 모델 목록 조회를 지원하지 않습니다. 직접 입력하세요.</p>
                      ) : meta.credential === "oauth" ? (
                        <p className="sd">OpenAI Codex 로그인은 모델 목록 조회를 지원하지 않습니다. 모델 ID를 직접 입력할 수 있고, 비우면 Codex 기본 모델을 사용합니다.</p>
                      ) : (
                        <p className="sd">새로고침은 위 API 키로 제공자의 모델 목록을 가져옵니다. 직접 입력도 가능합니다.</p>
                      )}
                      {modelsError && <p className="error">{modelsError}</p>}
                    </div>
                    <p className="sd">
                      API 키는 macOS Keychain에 저장되고, 모델·Base URL 같은 일반 설정만 로컬 settings.json에 저장됩니다.
                      Claude Code CLI는 기존 사용자 호환용이며 신규 사용자는 Claude API 연결을 권장합니다.
                    </p>
                    <div className="provider-test">
                      <button
                        type="button"
                        className="btn ghost sm"
                        onClick={() => testActiveProvider(meta)}
                        disabled={providerTesting}
                      >
                        {providerTesting ? "테스트 중…" : "연결 테스트"}
                      </button>
                      <p className="sd">
                        짧은 AI 요청을 한 번 보냅니다. API 키 방식은 제공자 정책에 따라 소량의 사용량이 기록될 수 있습니다.
                      </p>
                      {providerTestMsg && (
                        <p className={providerTestMsg.kind === "ok" ? "provider-test-ok" : "provider-test-error"}>
                          {providerTestMsg.text}
                        </p>
                      )}
                    </div>
                  </div>
                );
              })()}
            </section>

            <section className="settings-section">
              <h3>집필</h3>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() => !saving && apply({ typewriter_default: !current.typewriter_default })}
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">타자기 모드 기본값</span>
                  <span className="sd">새 씬을 열 때 타이프라이터 스크롤 켜기</span>
                </span>
                <span className={`switch${current.typewriter_default ? " on" : ""}`} />
              </button>
              <button
                type="button"
                className="set-row set-row-btn"
                onClick={() => !saving && apply({ focus_default: !current.focus_default })}
                disabled={saving}
              >
                <span className="sk-wrap">
                  <span className="sk">포커스 모드 기본값</span>
                  <span className="sd">현재 단락 외 디밍</span>
                </span>
                <span className={`switch${current.focus_default ? " on" : ""}`} />
              </button>
            </section>

            <section className="settings-section">
              <h3>LLM 도구</h3>
              <p className="sd">
                cmd+j 채팅에서 web_search, web_fetch, linetta_apply_ops 도구를 사용할 수 있습니다.
                web_fetch는 키 없이 동작합니다.
              </p>
              <div className="modal-field">
                <label htmlFor="ws-provider">web_search 제공자</label>
                <select
                  id="ws-provider"
                  value={current.web_search_provider}
                  onChange={(e) => apply({ web_search_provider: e.target.value as WebSearchProvider })}
                  disabled={saving}
                >
                  <option value="brave">Brave Search</option>
                  <option value="perplexity">Perplexity Sonar</option>
                </select>
              </div>
              <div className="modal-field">
                <label htmlFor="ws-key">web_search API 키</label>
                <div className="set-field-row">
                  <input
                    id="ws-key"
                    type="password"
                    value={webSearchKeyDraft}
                    onChange={(e) => setWebSearchKeyDraft(e.target.value)}
                    onBlur={() => {
                      if (webSearchKeyDraft !== "") {
                        apply({ web_search_api_key: webSearchKeyDraft });
                      }
                    }}
                    placeholder={webSearchKeyPlaceholder}
                    autoComplete="off"
                  />
                  {current.web_search_api_key_set && (
                    <button
                      type="button"
                      className="btn ghost sm"
                      onClick={() => {
                        setWebSearchKeyDraft("");
                        apply({ web_search_api_key: "" });
                      }}
                      disabled={saving}
                    >
                      키 삭제
                    </button>
                  )}
                </div>
              </div>
              <p className="sd">키는 macOS Keychain에 저장됩니다. settings.json에는 저장 여부만 표시됩니다.</p>
            </section>

            <section className="settings-section">
              <h3>GitHub 동기화</h3>
              <p className="sd">
                하루 한 번 모든 작품을 마크다운으로 내보내 지정한 git 폴더에 커밋·푸시합니다.
                경로를 비워두면 비활성화됩니다. 인증은 시스템 git 설정(SSH 키, 자격 증명 도우미)을 그대로 사용합니다.
              </p>
              <div className="modal-field">
                <label htmlFor="git-dir">git 폴더</label>
                <div className="set-field-row">
                  <input
                    id="git-dir"
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
                    className="btn ghost sm"
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
              </div>
              <div className="modal-field">
                <label htmlFor="git-tmpl">커밋 메시지</label>
                <input
                  id="git-tmpl"
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
              </div>
              <p className="sd">
                <code>{"{date}"}</code> 자리표시자만 지원됩니다 (YYYY-MM-DD HH:MM 으로 치환).
              </p>
              <p className="sd">
                지정한 폴더가 아직 git 저장소가 아닐 때 아래 버튼으로 초기화할 수 있습니다.
                초기화 후 <code>git remote add origin &lt;URL&gt;</code> 로 GitHub remote를 추가하세요.
              </p>
              <button
                type="button"
                className="btn ghost sm"
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
              <p className="sd">하루 한 번 자동 백업이 다음 경로에 저장됩니다 (14일 보관).</p>
              <div className="set-row">
                <span className="sk-wrap"><span className="sk">데이터 폴더</span></span>
                <span className="mono">{current.backup_dir}</span>
              </div>
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
          </>
        )}
      </div>
    </div>
  );
}

function isDegraded(status?: OpsStatus): boolean {
  return Boolean(status?.last_error);
}

function getCredentialState(meta?: ProviderMeta, cfg: ProviderConfig = {}): string {
  if (!meta) return "연결 방식 확인 필요";
  if (meta.credential === "oauth") {
    return "Codex 로그인 필요";
  }
  if (meta.credential === "key") {
    return cfg.api_key_set || cfg.api_key ? "API 키 저장됨" : "API 키 필요";
  }
  if (meta.credential === "cli") {
    return cfg.cli_path ? "CLI 경로 저장됨" : "기존 CLI 설정";
  }
  return "설정 확인 필요";
}

function getWebSearchKeyPlaceholder(current: SettingsRow): string {
  if (current.web_search_api_key_set) {
    return "저장된 검색 API 키 있음 · 새 키를 붙여넣으면 교체";
  }
  if (current.web_search_provider === "perplexity") {
    return "pplx-...";
  }
  return "BSA...";
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
