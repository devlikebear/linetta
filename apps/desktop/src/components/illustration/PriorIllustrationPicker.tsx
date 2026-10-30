import { useEffect, useState } from "react";

import { useI18n } from "../../lib/i18n";
import { illustrations as illustrationsApi } from "../../lib/rpc";
import type { IllustrationMeta } from "../../lib/types";
import { useIllustrationImage } from "./useIllustrationImage";
import "./Illustration.css";

interface Props {
  projectId: string;
  /** The characters in the new picture; the gallery's pictures of them are
   *  offered first. */
  characterIds: string[];
  /** The pictures chosen already (an agent's request carries its own);
   *  shown and selected instead of the suggestion. Undefined takes the
   *  suggestion as it comes. */
  initialIds?: string[];
  /** The pictures that will be sent. */
  value: string[];
  onChange: (ids: string[]) => void;
  disabled?: boolean;
}

function PriorThumb({ meta, selected, disabled, onToggle }: Readonly<{
  meta: IllustrationMeta;
  selected: boolean;
  disabled?: boolean;
  onToggle: () => void;
}>) {
  const url = useIllustrationImage(meta.id);
  return (
    <button
      type="button"
      className={`illustration-prior-thumb${selected ? " is-selected" : ""}`}
      aria-pressed={selected}
      aria-label={meta.caption || meta.id}
      title={meta.caption || undefined}
      disabled={disabled}
      onClick={onToggle}
      data-testid={`illustration-prior-${meta.id}`}
    >
      {url ? <img src={url} alt="" /> : null}
    </button>
  );
}

/* Earlier gallery pictures sent along with a new drawing, so a face drawn
 * once is drawn the same way again. The engine suggests them; every one
 * starts selected and the writer can leave any out. Nothing shows when the
 * gallery has none to offer. */
export function PriorIllustrationPicker({ projectId, characterIds, initialIds, value, onChange, disabled }: Props) {
  const { t } = useI18n();
  const [suggested, setSuggested] = useState<IllustrationMeta[]>([]);
  const key = characterIds.join(",");
  const initialKey = initialIds?.join(",");

  useEffect(() => {
    let cancelled = false;
    const initial = initialKey === undefined ? undefined : initialKey === "" ? [] : initialKey.split(",");
    illustrationsApi
      .suggestPrior(projectId, key === "" ? [] : key.split(","))
      .then(async (rows) => {
        // Chosen pictures the suggestion does not hold are read from the
        // gallery, so the writer sees everything that will be sent.
        const missing = (initial ?? []).filter((id) => !rows.some((r) => r.id === id));
        if (missing.length > 0) {
          const gallery = await illustrationsApi.list(projectId);
          rows = [...gallery.items.filter((m) => missing.includes(m.id)), ...rows];
        }
        if (cancelled) return;
        setSuggested(rows);
        onChange(initial ?? rows.map((r) => r.id));
      })
      .catch(() => {
        if (cancelled) return;
        setSuggested([]);
        onChange([]);
      });
    return () => {
      cancelled = true;
    };
    // onChange is the parent's setter; the suggestion follows the cast only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId, key, initialKey]);

  if (suggested.length === 0) return null;
  const toggle = (id: string) => onChange(value.includes(id) ? value.filter((x) => x !== id) : [...value, id]);
  return (
    <div className="modal-field illustration-prior" data-testid="illustration-prior">
      <label>{t("illustration.prior")}</label>
      <div className="illustration-prior-row">
        {suggested.map((m) => (
          <PriorThumb key={m.id} meta={m} selected={value.includes(m.id)} disabled={disabled} onToggle={() => toggle(m.id)} />
        ))}
      </div>
      <p className="sd">{t("illustration.prior.hint")}</p>
    </div>
  );
}
