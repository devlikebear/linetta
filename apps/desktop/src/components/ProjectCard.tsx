import type { Project } from "../lib/types";
import { Link } from "react-router-dom";
import { formatWordCount, lengthLabel, useI18n } from "../lib/i18n";

export function ProjectCard({ project }: { project: Project }) {
  const { language } = useI18n();
  const meta = `${lengthLabel(language, project.length_target)} · ${formatWordCount(language, project.word_count)}`;
  return (
    <Link to={`/workspace/${project.id}`} className="card">
      <p className="card-title">{project.title}</p>
      <p className="card-meta">{meta}</p>
    </Link>
  );
}
