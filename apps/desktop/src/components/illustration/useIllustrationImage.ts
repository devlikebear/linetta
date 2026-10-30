import { useEffect, useState } from "react";

import { illustrations as illustrationsApi } from "../../lib/rpc";

function blobUrl(base64: string, mime: string): string {
  const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
  return URL.createObjectURL(new Blob([bytes], { type: mime }));
}

/** One stored illustration as an object URL, fetched on demand and revoked
 *  when the component lets go of it. `failed` is set when the picture could
 *  not be loaded — usually because it was deleted from the gallery. */
export function useIllustrationImageState(id: string | null): { url: string | null; failed: boolean } {
  const [state, setState] = useState<{ url: string | null; failed: boolean }>({ url: null, failed: false });
  useEffect(() => {
    setState({ url: null, failed: false });
    if (!id) return undefined;
    let made: string | null = null;
    let cancelled = false;
    illustrationsApi
      .get(id)
      .then((img) => {
        if (cancelled) return;
        made = blobUrl(img.data_base64, img.mime);
        setState({ url: made, failed: false });
      })
      .catch(() => {
        if (!cancelled) setState({ url: null, failed: true });
      });
    return () => {
      cancelled = true;
      if (made) URL.revokeObjectURL(made);
    };
  }, [id]);
  return state;
}

/** The object URL alone; null while loading or when it failed. */
export function useIllustrationImage(id: string | null): string | null {
  return useIllustrationImageState(id).url;
}
