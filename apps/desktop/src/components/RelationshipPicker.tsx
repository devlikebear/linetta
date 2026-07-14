import { useEffect, useMemo, useRef, useState } from "react";
import { entities, relationships } from "../lib/rpc";
import type { Entity } from "../lib/types";
import { X, Plus } from "../lib/icons";
import { relationshipLabelPresets, useI18n } from "../lib/i18n";
import "./RelationshipPicker.css";

interface Props {
  projectId: string;
  fromEntityId: string;
  // Entities to hide from the search results (typically [fromEntityId]).
  excludeIds?: string[];
  onClose: () => void;
  onCreated: () => void; // parent refreshes the list
}

export function RelationshipPicker({
  projectId,
  fromEntityId,
  excludeIds = [],
  onClose,
  onCreated,
}: Props) {
  const { language, t } = useI18n();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Entity[]>([]);
  const [target, setTarget] = useState<Entity | null>(null);
  const [label, setLabel] = useState("");
  const [inverseLabel, setInverseLabel] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const debounceRef = useRef<number | null>(null);

  const hide = useMemo(() => new Set([fromEntityId, ...excludeIds]), [excludeIds, fromEntityId]);
  const labelPresets = relationshipLabelPresets(language);

  useEffect(() => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(async () => {
      try {
        const list = await entities.search(projectId, query, 20);
        setResults(list.filter((e) => !hide.has(e.id)));
      } catch (e) {
        setError(String(e));
      }
    }, 200);
    return () => {
      if (debounceRef.current) window.clearTimeout(debounceRef.current);
    };
  }, [hide, projectId, query]);

  const onSave = async () => {
    if (!target || label.trim() === "") return;
    setSaving(true);
    setError(null);
    try {
      if (inverseLabel.trim() === "") {
        await relationships.createOne({
          project_id: projectId,
          from_id: fromEntityId,
          to_id: target.id,
          label: label.trim(),
        });
      } else {
        await relationships.createPair({
          project_id: projectId,
          from_id: fromEntityId,
          to_id: target.id,
          label: label.trim(),
          inverse_label: inverseLabel.trim(),
        });
      }
      onCreated();
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="backdrop center" onMouseDown={onClose}>
      <div className="panel rel-picker" onMouseDown={(e) => e.stopPropagation()}>
        <div className="panel-head">
          <span className="ttl">
            <span className="ic"><Plus size={15} /></span> {t("relationship.add")}
          </span>
          <button type="button" className="panel-close" onClick={onClose} aria-label={t("common.close")}>
            <X size={16} />
          </button>
        </div>

        {error && <p className="rel-picker-error">{error}</p>}

        <div className="panel-scroll">
          <div className="sec">
            <h4>{t("relationship.target")}</h4>
            {!target ? (
              <>
                <input
                  className="rel-picker-input"
                  placeholder={t("relationship.searchPlaceholder")}
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  autoFocus
                />
                <div className="rel-picker-results">
                  {results.length === 0 && <p className="sec-empty">{t("relationship.noResults")}</p>}
                  {results.map((e) => (
                    <button
                      type="button"
                      key={e.id}
                      className="rel-picker-result"
                      onClick={() => setTarget(e)}
                    >
                      <span className="rel-picker-result-name">{e.name}</span>
                      {e.role && <span className="rel-picker-result-role">{e.role}</span>}
                    </button>
                  ))}
                </div>
              </>
            ) : (
              <div className="rel-picker-target">
                <span className="rel-picker-target-name">{target.name}</span>
                <button type="button" className="btn ghost sm" onClick={() => setTarget(null)}>
                  {t("relationship.change")}
                </button>
              </div>
            )}
          </div>

          <div className="sec">
            <h4>{t("relationship.label")}</h4>
            <div className="rel-picker-chips">
              {labelPresets.map((p) => (
                <button
                  key={p}
                  type="button"
                  className={"rel-chip" + (label === p ? " active" : "")}
                  onClick={() => setLabel(p)}
                >
                  {p}
                </button>
              ))}
            </div>
            <input
              className="rel-picker-input"
              placeholder={t("relationship.labelPlaceholder")}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          </div>

          <div className="sec">
            <h4>{t("relationship.inverseLabel")}</h4>
            <p className="rel-picker-hint">
              {t("relationship.inverseHint")}
            </p>
            <div className="rel-picker-chips">
              {labelPresets.map((p) => (
                <button
                  key={p}
                  type="button"
                  className={"rel-chip" + (inverseLabel === p ? " active" : "")}
                  onClick={() => setInverseLabel(p)}
                >
                  {p}
                </button>
              ))}
            </div>
            <input
              className="rel-picker-input"
              placeholder={t("relationship.inversePlaceholder")}
              value={inverseLabel}
              onChange={(e) => setInverseLabel(e.target.value)}
            />
          </div>
        </div>

        <div className="panel-foot">
          <span className="spacer" />
          <button type="button" className="btn ghost sm" onClick={onClose} disabled={saving}>{t("common.cancel")}</button>
          <button
            type="button"
            className="btn accent sm"
            disabled={saving || !target || label.trim() === ""}
            onClick={onSave}
          >
            {saving ? t("common.saving") : t("common.save")}
          </button>
        </div>
      </div>
    </div>
  );
}
