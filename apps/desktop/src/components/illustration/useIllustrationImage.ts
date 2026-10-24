import { useEffect, useState } from "react";

import { illustrations as illustrationsApi } from "../../lib/rpc";

function blobUrl(base64: string, mime: string): string {
  const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
  return URL.createObjectURL(new Blob([bytes], { type: mime }));
}

/** An object URL for one stored illustration, fetched on demand and revoked
 *  when the component lets go of it. null while loading or when it failed. */
export function useIllustrationImage(id: string | null): string | null {
  const [url, setUrl] = useState<string | null>(null);
  useEffect(() => {
    setUrl(null);
    if (!id) return undefined;
    let made: string | null = null;
    let cancelled = false;
    illustrationsApi
      .get(id)
      .then((img) => {
        if (cancelled) return;
        made = blobUrl(img.data_base64, img.mime);
        setUrl(made);
      })
      .catch(() => {
        /* the caller shows its own placeholder */
      });
    return () => {
      cancelled = true;
      if (made) URL.revokeObjectURL(made);
    };
  }, [id]);
  return url;
}
