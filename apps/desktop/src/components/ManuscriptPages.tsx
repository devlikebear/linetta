import { useEffect, useState } from "react";
import { projects as projectsApi } from "../lib/rpc";
import { useI18n } from "../lib/i18n";

interface Props {
  projectId: string;
  /** Changes when a scene is saved, so the count follows the prose. */
  refreshKey?: number | string | null;
}

/** The work in 200-character manuscript sheets, for writers whose contest
 *  sets length in sheets (episode-length Phase 4). The engine counts; the
 *  line says it is a rough count. Nothing shows for an empty work or a
 *  failed read: a wrong number is worse than none. */
export function ManuscriptPages({ projectId, refreshKey = null }: Readonly<Props>) {
  const { t, language } = useI18n();
  const [pages, setPages] = useState(0);

  useEffect(() => {
    let cancelled = false;
    projectsApi
      .manuscriptPages(projectId)
      .then((r) => {
        if (!cancelled) setPages(r.pages);
      })
      .catch(() => {
        if (!cancelled) setPages(0);
      });
    return () => {
      cancelled = true;
    };
  }, [projectId, refreshKey]);

  if (pages <= 0) return null;
  return (
    <div className="progress-meta manuscript-pages" data-testid="manuscript-pages" title={t("workspace.manuscriptPagesHint")}>
      <span>{t("workspace.manuscriptPages", { count: pages.toLocaleString(language) })}</span>
    </div>
  );
}
