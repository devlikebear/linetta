import { useState, useEffect, type FormEvent } from "react";
import type { NewProjectInput, LengthTarget, DefaultPOV } from "../lib/types";

const DEFAULT_GENRES = ["SF", "판타지", "추리", "문학"];
const LENGTHS: { value: LengthTarget; label: string }[] = [
  { value: "flash", label: "플래시" },
  { value: "short", label: "단편" },
  { value: "novella", label: "중편" },
  { value: "novel", label: "장편" },
  { value: "series", label: "시리즈" },
];
const POVS: { value: DefaultPOV; label: string }[] = [
  { value: "first", label: "1인칭" },
  { value: "third_limited", label: "3인칭 제한" },
  { value: "omniscient", label: "전지적" },
];

interface Props {
  open: boolean;
  onClose: () => void;
  onSubmit: (input: NewProjectInput) => Promise<void>;
}

export function NewProjectModal({ open, onClose, onSubmit }: Props) {
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
      setError("제목을 입력하세요");
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
        <h2>새 작품</h2>
        <p className="modal-sub">제목은 나중에 바꿀 수 있어요. 모든 데이터는 이 컴퓨터에만 저장됩니다.</p>

        <div className="modal-field">
          <label>제목</label>
          <input value={title} placeholder="예: 물의 기억" onChange={(e) => setTitle(e.target.value)} autoFocus />
        </div>

        <div className="modal-field">
          <label>장르 (다중 선택)</label>
          <div className="chips">
            {[...DEFAULT_GENRES, ...genres.filter((g) => !DEFAULT_GENRES.includes(g))].map((g) => (
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
          <label>예상 분량</label>
          <div className="chips">
            {LENGTHS.map((l) => (
              <button
                type="button"
                key={l.value}
                className={`chip${length === l.value ? " on" : ""}`}
                onClick={() => setLength(l.value)}
              >
                {l.label}
              </button>
            ))}
          </div>
        </div>

        <div className="modal-field">
          <label>기본 시점</label>
          <div className="chips">
            {POVS.map((p) => (
              <button
                type="button"
                key={p.value}
                className={`chip${pov === p.value ? " on" : ""}`}
                onClick={() => setPov(p.value)}
              >
                {p.label}
              </button>
            ))}
          </div>
        </div>

        {error && <p className="error">{error}</p>}

        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onClose} disabled={submitting}>취소</button>
          <button type="submit" className="btn accent" disabled={submitting}>{submitting ? "생성 중…" : "만들기"}</button>
        </div>
      </form>
    </div>
  );
}
