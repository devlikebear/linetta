import type { MessageKey } from "./i18n";
import type { PublishEpisodeState, PublishEpisodeView, PublishWorkStatus } from "./types";

/** The label each publishing state reads as, in the outline and the dialog. */
export const PUBLISH_STATE_KEYS: Record<PublishEpisodeState, MessageKey> = {
  unpublished: "publish.state.unpublished",
  public: "publish.state.public",
  modified: "publish.state.modified",
  pending: "publish.state.pending",
  rejected: "publish.state.rejected",
  needs_review: "publish.state.needsReview",
  private: "publish.state.private",
  site_only: "publish.state.siteOnly",
};

/** States the outline marks on an episode row. Unpublished is the default
 *  for every episode of every work and would only be noise. */
export const OUTLINE_BADGE_STATES: ReadonlySet<PublishEpisodeState> = new Set([
  "public",
  "modified",
  "pending",
  "rejected",
  "needs_review",
  "private",
]);

/** The episode a node belongs to, by the engine's own answer: the episode
 *  itself or one of its scenes. The engine's rules decide, not a copy here. */
export function findPublishEpisode(
  status: PublishWorkStatus | null | undefined,
  nodeId: string,
): PublishEpisodeView | null {
  if (!status) return null;
  return (
    status.episodes.find((ep) => ep.node_id === nodeId || ep.leaf_ids?.includes(nodeId)) ?? null
  );
}
