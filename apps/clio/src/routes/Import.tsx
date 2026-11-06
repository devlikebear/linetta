import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { posts } from "../lib/rpc";
import { t, tDynamic } from "../lib/i18n";
import type { ImportParams, ImportPreview, ImportResult, Platform, PostKind } from "../lib/types";
import "./Import.css";

interface LoadedFile {
  name: string;
  content: string;
}

export function Import() {
  const [file, setFile] = useState<LoadedFile | null>(null);
  const [platform, setPlatform] = useState<Platform>("x");
  const [kind, setKind] = useState<PostKind>("post");
  const [userId, setUserId] = useState("");
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [error, setError] = useState("");

  // Rebuild the preview whenever the file or an option changes.
  useEffect(() => {
    if (!file) return;
    let cancelled = false;
    const params: ImportParams = {
      file_name: file.name,
      content: file.content,
      default_kind: kind,
      platform,
      own_user_id: userId.trim(),
    };
    setPreviewing(true);
    setError("");
    setResult(null);
    posts
      .importPreview(params)
      .then((p) => {
        if (!cancelled) setPreview(p);
      })
      .catch((e) => {
        if (cancelled) return;
        setPreview(null);
        setError(t("common.error", { error: e instanceof Error ? e.message : String(e) }));
      })
      .finally(() => {
        if (!cancelled) setPreviewing(false);
      });
    return () => {
      cancelled = true;
    };
  }, [file, platform, kind, userId]);

  const onFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const picked = e.target.files?.[0];
    if (!picked) return;
    try {
      setFile({ name: picked.name, content: await picked.text() });
    } catch (err) {
      setError(t("common.error", { error: err instanceof Error ? err.message : String(err) }));
    }
  };

  const submit = async () => {
    if (!file) return;
    setSubmitting(true);
    setError("");
    try {
      setResult(
        await posts.import({
          file_name: file.name,
          content: file.content,
          default_kind: kind,
          platform,
          own_user_id: userId.trim(),
        }),
      );
    } catch (err) {
      setError(t("common.error", { error: err instanceof Error ? err.message : String(err) }));
    } finally {
      setSubmitting(false);
    }
  };

  const skipped = preview ? Object.entries(preview.skipped).filter(([, n]) => n > 0) : [];

  return (
    <section className="import">
      <h1>{t("import.title")}</h1>
      <p className="muted">{t("import.intro")}</p>

      <div className="field">
        <label htmlFor="import-file">{t("import.file")}</label>
        <input id="import-file" type="file" accept=".json,.js,.txt,.md" onChange={onFile} />
        <span className="note">{t("import.fileHint")}</span>
      </div>

      <div className="field">
        <label htmlFor="import-platform">{t("import.platform")}</label>
        <select id="import-platform" value={platform} onChange={(e) => setPlatform(e.target.value as Platform)}>
          <option value="x">{t("import.platform.x")}</option>
          <option value="threads">{t("import.platform.threads")}</option>
          <option value="blog">{t("import.platform.blog")}</option>
        </select>
      </div>

      <div className="field">
        <label htmlFor="import-kind">{t("import.kind")}</label>
        <select id="import-kind" value={kind} onChange={(e) => setKind(e.target.value as PostKind)}>
          <option value="post">{t("import.kind.post")}</option>
          <option value="reply">{t("import.kind.reply")}</option>
        </select>
        <span className="note">{t("import.kindNote")}</span>
      </div>

      <div className="field">
        <label htmlFor="import-user-id">{t("import.userId")}</label>
        <input
          id="import-user-id"
          type="text"
          inputMode="numeric"
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
        />
        <span className="note">{t("import.userIdNote")}</span>
      </div>

      {error && <p className="error" role="alert">{error}</p>}
      {previewing && <p className="muted">{t("import.previewing")}</p>}

      {preview && !previewing && (
        <div className="preview">
          <h2>{t("import.previewTitle")}</h2>
          {preview.total === 0 ? (
            <p className="muted">{t("import.empty")}</p>
          ) : (
            <p className="total">{t("import.total", { count: preview.total })}</p>
          )}
          <ul className="counts">
            {preview.contexts.map((c) => (
              <li key={`${c.platform}/${c.kind}`}>
                <span>{tDynamic(`import.context.${c.platform}.${c.kind}`, `${c.platform}/${c.kind}`)}</span>
                <span className="num">{c.count}</span>
              </li>
            ))}
          </ul>

          {skipped.length > 0 && (
            <>
              <h3>{t("import.skippedTitle")}</h3>
              <ul className="counts">
                {skipped.map(([reason, n]) => (
                  <li key={reason}>
                    <span>{tDynamic(`import.skip.${reason}`, reason)}</span>
                    <span className="num">{n}</span>
                  </li>
                ))}
              </ul>
            </>
          )}

          {preview.preview.length > 0 && (
            <>
              <h3>{t("import.samples")}</h3>
              <ul className="samples">
                {preview.preview.map((p, i) => (
                  <li key={i} className="clamp2">{p.body}</li>
                ))}
              </ul>
            </>
          )}

          {preview.total > 0 && !result && (
            <button type="button" className="primary" onClick={submit} disabled={submitting}>
              {submitting ? t("import.submitting") : t("import.submit")}
            </button>
          )}
        </div>
      )}

      {result && (
        <p className="result" role="status">
          {t("import.done", { inserted: result.inserted, skipped: result.skipped })}{" "}
          <Link to="/voice">{t("import.goVoice")}</Link>
        </p>
      )}
    </section>
  );
}
