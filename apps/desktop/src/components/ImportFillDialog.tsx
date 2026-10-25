import type { ExtractionEstimate } from "../lib/types";
import type { MessageKey } from "../lib/i18n";
import { useI18n } from "../lib/i18n";

/** Offered right after a Markdown import finishes (story-assist 3.D.3): the
 *  writer decides whether to let the agent sweep the manuscript for Story
 *  World candidates right away. Nothing is applied by this dialog itself —
 *  [시작] only queues the extraction prompt in the agent panel's composer;
 *  the writer still has to press send, same as a starter chip. */

interface Props {
  /** Null while proposals.extraction_estimate is still loading. */
  estimate: ExtractionEstimate | null;
  /** The connected provider's name key, or undefined if somehow shown with
   *  none — Library only opens this dialog once one is ready. */
  providerNameKey?: MessageKey;
  model?: string;
  onStart: () => void;
  onLater: () => void;
}

export function ImportFillDialog({ estimate, providerNameKey, model, onStart, onLater }: Readonly<Props>) {
  const { t } = useI18n();
  return (
    <div className="backdrop center" onClick={onLater}>
      <div className="modal" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
        <h2>{t("importFill.title")}</h2>
        {estimate ? (
          <p className="modal-sub">
            {t("importFill.body", {
              episodes: estimate.episodes,
              chars: estimate.chars.toLocaleString(),
              tokens: estimate.input_tokens.toLocaleString(),
            })}
          </p>
        ) : (
          <p className="modal-sub">{t("common.loading")}</p>
        )}
        {providerNameKey && (
          <p className="modal-sub">{t("importFill.provider", { provider: t(providerNameKey), model: model ?? "" })}</p>
        )}
        <p className="modal-sub">{t("importFill.disclaimer")}</p>
        <div className="modal-actions">
          <button type="button" className="btn ghost" onClick={onLater}>
            {t("importFill.later")}
          </button>
          <button type="button" className="btn accent" onClick={onStart}>
            {t("importFill.start")}
          </button>
        </div>
      </div>
    </div>
  );
}
