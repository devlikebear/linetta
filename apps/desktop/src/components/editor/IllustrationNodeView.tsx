import { NodeViewWrapper, type ReactNodeViewProps } from "@tiptap/react";

import { useI18n } from "../../lib/i18n";
import { useIllustrationImageState } from "../illustration/useIllustrationImage";

/* A picture in the scene. The document holds only its id and caption; the
 * bytes come from the gallery. A picture the gallery no longer has is drawn
 * as a placeholder and the node stays in the document, so restoring the
 * picture (or the scene) needs nothing else. */
export function IllustrationNodeView({ node, updateAttributes, selected }: ReactNodeViewProps) {
  const { t } = useI18n();
  const id = typeof node.attrs.id === "string" ? node.attrs.id : null;
  const caption = typeof node.attrs.caption === "string" ? node.attrs.caption : "";
  const { url, failed } = useIllustrationImageState(id);

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
            {failed ? t("illustration.missing") : ""}
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
