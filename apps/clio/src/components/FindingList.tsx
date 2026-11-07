import { isRanged } from "../lib/offsets";
import { t } from "../lib/i18n";
import type { Finding, Severity } from "../lib/types";
import "./FindingList.css";

const RANK: Record<Severity, number> = { strong: 0, medium: 1, weak: 2 };

/** Strong first, then medium, weak; within a severity by position. Returns a copy. */
export function sortFindings(findings: Finding[]): Finding[] {
  return [...findings].sort((a, b) => RANK[a.severity] - RANK[b.severity] || a.start - b.start || a.end - b.end);
}

interface Props {
  findings: Finding[];
  /** Called when a ranged finding is clicked. */
  onSelect: (finding: Finding) => void;
}

export function FindingList({ findings, onSelect }: Props) {
  if (findings.length === 0) return <p className="muted">{t("write.noFindings")}</p>;
  return (
    <ul className="findings">
      {sortFindings(findings).map((f, i) => {
        const body = (
          <>
            <span className={`dot dot-${f.severity}`} aria-hidden="true" />
            <span className="finding-text">
              <span className="finding-message">
                <span className="sr-only">{t(`write.severity.${f.severity}`)} </span>
                {f.message}
              </span>
              {f.evidence && <span className="finding-evidence muted">{f.evidence}</span>}
            </span>
          </>
        );
        return (
          <li key={`${f.rule}-${f.start}-${f.end}-${i}`}>
            {isRanged(f) ? (
              <button type="button" className="finding finding-ranged" onClick={() => onSelect(f)}>
                {body}
              </button>
            ) : (
              <div className="finding">{body}</div>
            )}
          </li>
        );
      })}
    </ul>
  );
}
