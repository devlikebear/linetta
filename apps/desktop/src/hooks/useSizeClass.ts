import { useEffect, useState } from "react";

export type SizeClass = "compact" | "ipad" | "desktop";

// Touch capability is detected with `any-pointer: coarse` (true when ANY
// available input is touch), NOT `pointer: coarse` (the PRIMARY pointer). A
// Magic Keyboard trackpad — and the iOS Simulator — make the primary pointer
// `fine`, so `pointer: coarse` wrongly drops the iPad to the desktop layout.
// `not (any-pointer: coarse)` therefore identifies a real mouse-only desktop.
// Touch-capable iPads stay in the iPad tier through the largest regular iPad
// landscape width (1366 CSS px); wider external displays can use desktop.
export const DESKTOP_QUERY = "(min-width: 1367px), (not (any-pointer: coarse))";
export const IPAD_QUERY =
  "(min-width: 701px) and (max-width: 1366px) and (min-height: 600px) and (any-pointer: coarse)";

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
