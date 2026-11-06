import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { posts, voice } from "../lib/rpc";
import { t, tDynamic } from "../lib/i18n";
import { FEATURE_NAMES, type ContextProfile, type Platform, type Post, type PostKind } from "../lib/types";
import "./Voice.css";

interface Tab {
  id: string;
  label: "voice.tab.xPost" | "voice.tab.xReply" | "voice.tab.all";
  platform: Platform | "*";
  kind: PostKind | "*";
}

const TABS: Tab[] = [
  { id: "x/post", label: "voice.tab.xPost", platform: "x", kind: "post" },
  { id: "x/reply", label: "voice.tab.xReply", platform: "x", kind: "reply" },
  { id: "*/*", label: "voice.tab.all", platform: "*", kind: "*" },
];

const pct = (v: number) => `${Math.round(v * 100)}%`;
const num = (v: number) => (Number.isInteger(v) ? String(v) : v.toFixed(2));

/** Features that are shares (0..1) rather than counts. */
const SHARE_FEATURES = new Set(["punctuated", "polite", "emoji", "laugh", "cry", "hashtag", "ellipsis", "exclaim", "ai_tells"]);

function fmt(name: string, v: number): string {
  return SHARE_FEATURES.has(name) && v <= 1 ? pct(v) : num(v);
}

export function Voice() {
  const [tab, setTab] = useState<Tab>(TABS[0]);
  const [profile, setProfile] = useState<ContextProfile | null>(null);
  const [bodies, setBodies] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    (async () => {
      const p = await voice.profile({ platform: tab.platform, kind: tab.kind });
      // Examples are ids. The profile carries no bodies, so resolve them from
      // the stored posts of the context the profile actually used.
      let found: Record<string, string> = {};
      if (p.examples.length > 0) {
        const [fb, fk] = p.fallback.split("/");
        const list: Post[] = await posts.list({
          platform: fb === "*" ? "" : (fb as Platform),
          kind: fk === "*" || fk === undefined ? "" : (fk as PostKind),
          limit: 500,
        });
        found = Object.fromEntries(list.map((x) => [x.id, x.body]));
      }
      if (!cancelled) {
        setProfile(p);
        setBodies(found);
      }
    })()
      .catch((e) => {
        if (!cancelled) setError(t("common.error", { error: e instanceof Error ? e.message : String(e) }));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tab]);

  const tabs = (
    <div className="tabs" role="tablist">
      {TABS.map((x) => (
        <button
          key={x.id}
          type="button"
          role="tab"
          aria-selected={x.id === tab.id}
          className={x.id === tab.id ? "tab active" : "tab"}
          onClick={() => setTab(x)}
        >
          {t(x.label)}
        </button>
      ))}
    </div>
  );

  const empty = profile !== null && profile.n === 0;

  return (
    <section className="voice">
      <h1>{t("voice.title")}</h1>
      {tabs}
      {error && <p className="error" role="alert">{error}</p>}
      {loading && <p className="muted">{t("common.loading")}</p>}

      {!loading && empty && (
        <div className="empty">
          <p className="muted">{t("voice.empty")}</p>
          <Link to="/import">{t("voice.emptyAction")}</Link>
        </div>
      )}

      {!loading && profile && !empty && (
        <>
          <p className="muted">
            {t("voice.sample", { n: profile.n })}
            {profile.fallback !== profile.key && (
              <span className="fallback" role="note">
                {" · "}
                {t("voice.fallback", {
                  context: tDynamic(`voice.context.${profile.fallback}`, profile.fallback),
                })}
              </span>
            )}
          </p>

          <div className="shares">
            <div>
              <div className="big">{pct(profile.polite_share)}</div>
              <div className="muted">{t("voice.politeShare")}</div>
            </div>
            <div>
              <div className="big">{pct(profile.mixed_share)}</div>
              <div className="muted">{t("voice.mixedShare")}</div>
            </div>
          </div>

          <h2>{t("voice.featuresTitle")}</h2>
          <table className="features">
            <thead>
              <tr>
                <th>{t("voice.col.feature")}</th>
                <th>{t("voice.col.p10")}</th>
                <th>{t("voice.col.p50")}</th>
                <th>{t("voice.col.p90")}</th>
              </tr>
            </thead>
            <tbody>
              {FEATURE_NAMES.map((name) => {
                const s = profile.features[name];
                if (!s) return null;
                return (
                  <tr key={name}>
                    <th scope="row">{tDynamic(`voice.feature.${name}`, name)}</th>
                    <td>{fmt(name, s.p10)}</td>
                    <td>{fmt(name, s.p50)}</td>
                    <td>{fmt(name, s.p90)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>

          <h2>{t("voice.endingsTitle")}</h2>
          <ul className="endings">
            {profile.top.slice(0, 8).map((e) => (
              <li key={e.ending}>
                <span className="ending">{e.ending}</span>
                <span className="bar" aria-hidden="true">
                  <span style={{ width: `${Math.max(2, Math.round(e.share * 100))}%` }} />
                </span>
                <span className="num">{pct(e.share)}</span>
              </li>
            ))}
          </ul>

          {profile.examples.length > 0 && (
            <>
              <h2>{t("voice.samplesTitle")}</h2>
              <ul className="samples">
                {profile.examples.map((id) => (
                  <li key={id} className="clamp3">
                    {bodies[id] ?? t("voice.sampleMissing", { id })}
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </section>
  );
}
