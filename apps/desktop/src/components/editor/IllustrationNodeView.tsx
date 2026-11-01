import { useEffect, useState } from "react";
import { NodeViewWrapper, type ReactNodeViewProps } from "@tiptap/react";

import { useI18n } from "../../lib/i18n";
import { vaultImagesOmitted } from "../../lib/vault";
import { useIllustrationImageState } from "../illustration/useIllustrationImage";

/* A picture in the scene. The document holds only its id and caption; the
 * bytes come from the gallery. A picture the gallery no longer has is drawn
 * as a placeholder and the node stays in the document, so restoring the
 * picture (or the scene) needs nothing else. With pictures left out of sync, a
 * missing one was most likely never carried here, which is not the same as
 * deleted, and turning them back on brings it with the next revision. */
export function IllustrationNodeView({ node, updateAttributes, selected }: ReactNodeViewProps) {
  const { t } = useI18n();
  const id = typeof node.attrs.id === "string" ? node.attrs.id : null;
  const caption = typeof node.attrs.caption === "string" ? node.attrs.caption : "";
  const { url, failed } = useIllustrationImageState(id);
  const [notCarried, setNotCarried] = useState(false);
  useEffect(() => {
    if (!failed) return undefined;
    let cancelled = false;
    void vaultImagesOmitted().then((omitted) => {
      if (!cancelled) setNotCarried(omitted);
    });
    return () => {
      cancelled = true;
    };
  }, [failed]);

  return (
    <NodeViewWrapper
      as="figure"
      className={`illustration-node${selected ? " is-selected" : ""}`}
      data-illustration-id={id ?? ""}
      data-testid="illustration-node"
    >
      <div contentEditable={false} className="illustration-node-frame">
        {url ? (
          <img src={url} alt={caption} draggable={false} />
        ) : (
          <span className={`illustration-node-placeholder${failed ? " is-missing" : ""}`}>
            {failed ? t(notCarried ? "illustration.notOnDevice" : "illustration.missing") : ""}
          </span>
        )}
      </div>
      <input
        className="illustration-node-caption"
        value={caption}
        placeholder={t("illustration.caption")}
        aria-label={t("illustration.caption")}
        onChange={(e) => updateAttributes({ caption: e.target.value })}
        data-testid="illustration-node-caption"
      />
    </NodeViewWrapper>
  );
}
