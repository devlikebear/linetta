import type { Project, LengthTarget } from "../lib/types";
import { Link } from "react-router-dom";

const LENGTH_LABEL: Record<LengthTarget, string> = {
  flash: "플래시",
  short: "단편",
  novella: "중편",
  novel: "장편",
  series: "시리즈",
};

function humanCount(words: number): string {
  if (words === 0) return "초안 시작 전";
  if (words < 1000) return `${words}자`;
  if (words < 10_000) return `${words.toLocaleString("ko-KR")}자`;
  return `${(words / 1000).toFixed(0)}k`;
}

export function ProjectCard({ project }: { project: Project }) {
  const meta = `${LENGTH_LABEL[project.length_target]} · ${humanCount(project.word_count)}`;
  return (
    <Link to={`/workspace/${project.id}`} className="card">
      <p className="card-title">{project.title}</p>
      <p className="card-meta">{meta}</p>
    </Link>
  );
}
