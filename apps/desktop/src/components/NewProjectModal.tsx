import { useState, useEffect, type FormEvent } from "react";
import type { NewProjectInput, LengthTarget, DefaultPOV } from "../lib/types";
import { defaultGenres, lengthLabel, povLabel, useI18n } from "../lib/i18n";

const LENGTHS: LengthTarget[] = ["flash", "short", "novella", "novel", "series"];
const POVS: DefaultPOV[] = ["first", "third_limited", "omniscient"];

interface Props {
  open: boolean;
  onClose: () => void;
  onSubmit: (input: NewProjectInput) => Promise<void>;
}

export function NewProjectModal({ open, onClose, onSubmit }: Props) {
  const { language, t } = useI18n();
  const defaultGenreOptions = defaultGenres(language);
  const [title, setTitle] = useState("");
  const [genres, setGenres] = useState<string[]>([]);
  const [customGenre, setCustomGenre] = useState("");
  const [length, setLength] = useState<LengthTarget>("short");
  const [pov, setPov] = useState<DefaultPOV>("first");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setTitle("");
      setGenres([]);
      setCustomGenre("");
      setLength("short");
      setPov("first");
      setError(null);
    }
  }, [open]);

  if (!open) return null;

  const toggleGenre = (g: string) => {
    setGenres((prev) => (prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]));
  };

  const addCustomGenre = () => {
    const g = customGenre.trim();
    if (!g || genres.includes(g)) {
      setCustomGenre("");
      return;
    }
    setGenres((prev) => [...prev, g]);
    setCustomGenre("");
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) {
      setError(t("newProject.titleRequired"));
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit({
        title: title.trim(),
        genres,
        length_target: length,
        default_pov: pov,
      });
    } catch (err) {
      setError(String(err));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="backdrop center" onClick={onClose}>
      <form className="modal" onClick={(e) => e.stopPropagation()} onSubmit={handleSubmit}>
        <h2>{t("newProject.title")}</h2>
        <p className="modal-sub">{t("newProject.subtitle")}</p>

        <div className="modal-field">
          <label>{t("newProject.titleLabel")}</label>
          <input value={title} placeholder={t("newProject.titlePlaceholder")} onChange={(e) => setTitle(e.target.value)} autoFocus />
        </div>

        <div className="modal-field">
          <label>{t("newProject.genres")}</label>
          <div className="chips">
            {[...defaultGenreOptions, ...genres.filter((g) => !defaultGenreOptions.includes(g))].map((g) => (
              <button
                type="button"
                key={g}
                className={`chip${genres.includes(g) ? " on" : ""}`}
                onClick={() => toggleGenre(g)}
              >
                {g}
              </button>
            ))}
            <input
              className="chip-input"
              placeholder="+"
              value={customGenre}
              onChange={(e) => setCustomGenre(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  addCustomGenre();
                }
              }}
            />
          </div>
        </div>

        <div className="modal-field">
          <label>{t("newProject.length")}</label>
          <div className="chips">
            {LENGTHS.map((l) => (
              <button
                type="button"
                key={l}
                className={`chip${length === l ? " on" : ""}`}
                onClick={() => setLength(l)}
              >
                {lengthLabel(language, l)}
              </button>
            ))}
          </div>
        </div>

        <div className="modal-field">
          <label>{t("newProject.pov")}</label>
          <div className="chips">
            {POVS.map((p) => (
              <button
                type="button"
                key={p}
                className={`chip${pov === p ? " on" : ""}`}
                onClick={() => setPov(p)}
              >
                {povLabel(language, p)}
              </button>
            ))}
          </div>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose} disabled={submitting}>{t("common.cancel")}</button>
          <button type="submit" className="btn accent" disabled={submitting}>{submitting ? t("newProject.creating") : t("newProject.create")}</button>
        </div>
      </form>
    </div>
  );
}
