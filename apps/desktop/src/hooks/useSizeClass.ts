import { useEffect, useState } from "react";

export type SizeClass = "compact" | "ipad" | "desktop";

export const DESKTOP_QUERY = "(min-width: 1181px), (pointer: fine)";
export const IPAD_QUERY =
  "(min-width: 701px) and (min-height: 600px) and (pointer: coarse)";

export function resolveSizeClass(matches: {
  desktop: boolean;
  ipad: boolean;
}): SizeClass {
  if (matches.desktop) return "desktop";
  if (matches.ipad) return "ipad";
  return "compact";
}

function readSizeClass(): SizeClass {
  if (typeof window === "undefined" || !window.matchMedia) return "desktop";
  return resolveSizeClass({
    desktop: window.matchMedia(DESKTOP_QUERY).matches,
    ipad: window.matchMedia(IPAD_QUERY).matches,
  });
}

export function useSizeClass(): SizeClass {
  const [cls, setCls] = useState<SizeClass>(readSizeClass);
  useEffect(() => {
    if (typeof window === "undefined" || !window.matchMedia) return;
    const desktop = window.matchMedia(DESKTOP_QUERY);
    const ipad = window.matchMedia(IPAD_QUERY);
    const update = () =>
      setCls(resolveSizeClass({ desktop: desktop.matches, ipad: ipad.matches }));
    desktop.addEventListener("change", update);
    ipad.addEventListener("change", update);
    update();
    return () => {
      desktop.removeEventListener("change", update);
      ipad.removeEventListener("change", update);
    };
  }, []);
  return cls;
}
