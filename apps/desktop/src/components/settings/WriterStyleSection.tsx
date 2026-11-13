import { useEffect, useState } from "react";

import { useEngineEvent } from "../../hooks/useEngineEvent";
import type { McpChangedPayload } from "../../hooks/useMcpChanges";
import { useI18n } from "../../lib/i18n";
import { style as styleApi } from "../../lib/rpc";
import { rpcErrorMessage } from "../../lib/rpcMessage";
import type { StyleApproveMode, StyleDraftState } from "../../lib/types";

/** The writer's default style (#178): what follows the writer into every work.
 *
 *  The rules (#181) sit under every work's own check rules. The notes (#182)
 *  are prose — tone, rhythm, point-of-view habits — that go into every brief
 *  as the writer's own direction, ahead of the work's notes. Changing either
 *  changes every work at once, and a work can turn both off in its own style
 *  section.
 *
 *  Only the writer sets them. An agent can propose notes (#183), which wait
 *  here until the writer approves, edits or discards them. */
export function WriterStyleSection() {
  const { t } = useI18n();
  const [phrases, setPhrases] = useState<string | null>(null);
  const [maxSentence, setMaxSentence] = useState("");
  const [notes, setNotes] = useState("");
  const [notesLimit, setNotesLimit] = useState(1000);
  const [draft, setDraft] = useState<StyleDraftState | null>(null);
  const [draftBody, setDraftBody] = useState("");
  const [draftRevision, setDraftRevision] = useState(0);
  const [busy, setBusy] = useState<"rules" | "notes" | "draft" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState<"rules" | "notes" | null>(null);

  useEffect(() => {
    let cancelled = false;
    styleApi
      .getWriterRules()
      .then((state) => {
        if (cancelled) return;
        setPhrases(state.avoid_phrases.join("\n"));
        setMaxSentence(state.max_sentence_chars > 0 ? String(state.max_sentence_chars) : "");
        setNotes(state.notes ?? "");
        if (state.notes_limit) setNotesLimit(state.notes_limit);
      })
      .catch((e) => {
        if (!cancelled) setError(e);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // A proposal arrives from an agent's tool call, so it reloads on the
  // engine's change event as well as on mount. Only a writer-wide proposal
  // (no work id) is this section's.
  useEngineEvent<McpChangedPayload>("mcp-changed", (event) => {
    if (event.tool === "linetta_propose_style_profile" && !event.project_id) setDraftRevision((n) => n + 1);
  });
  useEffect(() => {
    let cancelled = false;
    styleApi
      .getWriterDraft()
      .then((state) => {
        if (cancelled) return;
        setDraft(state);
        setDraftBody(state.pending ? state.draft.body : "");
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [draftRevision]);

  const run = async (kind: NonNullable<typeof busy>, work: () => Promise<void>) => {
    setBusy(kind);
    setError(null);
    setSaved(null);
    try {
      await work();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const saveRules = () =>
    run("rules", async () => {
      const limit = maxSentence.trim() === "" ? 0 : Math.trunc(Number(maxSentence));
      const next = await styleApi.setWriterRules({
        avoid_phrases: (phrases ?? "").split("\n").map((line) => line.trim()).filter((line) => line !== ""),
        max_sentence_chars: Number.isFinite(limit) ? limit : 0,
      });
      setPhrases(next.avoid_phrases.join("\n"));
      setMaxSentence(next.max_sentence_chars > 0 ? String(next.max_sentence_chars) : "");
      setSaved("rules");
    });

  const saveNotes = () =>
    run("notes", async () => {
      const next = await styleApi.setWriterNotes(notes);
      setNotes(next.notes);
      setSaved("notes");
    });

  const approve = (mode: StyleApproveMode) =>
    run("draft", async () => {
      const result = await styleApi.approveWriterDraft(draftBody, mode);
      setNotes(result.notes);
      setDraft((prev) => (prev ? { ...prev, pending: false } : prev));
    });

  const discard = () =>
    run("draft", async () => {
      await styleApi.discardWriterDraft();
      setDraft((prev) => (prev ? { ...prev, pending: false } : prev));
    });

  const working = busy != null;
  return (
    <section className="settings-section" data-testid="writer-style-section">
      <h3>{t("settings.writerStyle.title")}</h3>
      <p className="sd">{t("settings.writerStyle.description")}</p>
      {error != null && (
        <p className="sd" role="alert">
          {rpcErrorMessage(error, t)}
        </p>
      )}

      {draft?.pending && (
        <div className="set-row is-stacked" role="group" aria-label={t("style.draftTitle")} data-testid="writer-style-draft">
          <span className="sk">{t("style.draftTitle")}</span>
          <span className="sd">{t("style.draftHint")}</span>
          <textarea
            rows={5}
            aria-label={t("style.draftBody")}
            value={draftBody}
            disabled={working}
            onChange={(e) => setDraftBody(e.target.value)}
          />
          <span className="sd">{t("style.draftCount", { count: [...draftBody].length, limit: draft.limit })}</span>
          <div className="set-row">
            <button type="button" className="btn accent sm" disabled={working || draftBody.trim() === ""} onClick={() => void approve("append")}>
              {t("style.draftAppend")}
            </button>
            <button type="button" className="btn sm" disabled={working || draftBody.trim() === ""} onClick={() => void approve("replace")}>
              {t("style.draftReplace")}
            </button>
            <button type="button" className="btn sm" disabled={working} onClick={() => void discard()}>
              {t("style.draftDiscard")}
            </button>
          </div>
        </div>
      )}

      {phrases != null && (
        <>
          <label className="set-row is-stacked">
            <span className="sk">{t("settings.writerStyle.notes")}</span>
            <span className="sd">{t("settings.writerStyle.notesHint")}</span>
            <textarea
              rows={4}
              value={notes}
              disabled={working}
              placeholder={t("style.notesPlaceholder")}
              onChange={(e) => {
                setNotes(e.target.value);
                setSaved(null);
              }}
            />
            <span className="sd" data-testid="writer-style-notes-count">
              {t("settings.writerStyle.notesCount", { count: [...notes.trim()].length, limit: notesLimit })}
            </span>
          </label>
          <div className="set-row">
            <button type="button" className="btn accent sm" disabled={working} onClick={() => void saveNotes()}>
              {busy === "notes" ? t("common.saving") : t("style.saveNotes")}
            </button>
            {saved === "notes" && (
              <span className="sd" role="status">
                {t("settings.writerStyle.saved")}
              </span>
            )}
          </div>

          <label className="set-row is-stacked">
            <span className="sk">{t("style.avoidPhrases")}</span>
            <textarea
              rows={4}
              value={phrases}
              disabled={working}
              placeholder={t("style.avoidPhrasesPlaceholder")}
              onChange={(e) => {
                setPhrases(e.target.value);
                setSaved(null);
              }}
            />
          </label>
          <label className="set-row is-stacked">
            <span className="sk">{t("style.maxSentence")}</span>
            <input
              type="number"
              min={0}
              max={2000}
              inputMode="numeric"
              value={maxSentence}
              disabled={working}
              placeholder={t("style.maxSentencePlaceholder")}
              onChange={(e) => {
                setMaxSentence(e.target.value);
                setSaved(null);
              }}
            />
          </label>
          <div className="set-row">
            <button type="button" className="btn accent sm" disabled={working} onClick={() => void saveRules()}>
              {busy === "rules" ? t("common.saving") : t("style.save")}
            </button>
            {saved === "rules" && (
              <span className="sd" role="status">
                {t("settings.writerStyle.saved")}
              </span>
            )}
          </div>
        </>
      )}
    </section>
  );
}
