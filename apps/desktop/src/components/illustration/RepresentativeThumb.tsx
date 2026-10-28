import { useI18n } from "../../lib/i18n";
import { useIllustrationImage } from "./useIllustrationImage";

/* The picture that stands for a scene or an episode, small: beside its row
 * in the outline and above the open scene (illustration Phase 4). Nothing
 * while it loads or when it cannot be read. */
export function RepresentativeThumb({ illustrationId, className = "" }: { illustrationId: string; className?: string }) {
  const { t } = useI18n();
  const url = useIllustrationImage(illustrationId);
  if (!url) return null;
  return (
    <img
      className={`representative-thumb ${className}`.trim()}
      src={url}
      alt=""
      title={t("illustration.representative")}
      data-testid="representative-thumb"
    />
  );
}
