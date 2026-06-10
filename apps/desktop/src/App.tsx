import { Suspense, lazy, useEffect } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import { Library } from "./routes/Library";
import { ToastProvider } from "./components/ToastProvider";
import { EngineGate } from "./components/EngineGate";
import { I18nProvider, useI18n } from "./lib/i18n";
import { settings as settingsApi } from "./lib/rpc";
import type { Settings } from "./lib/types";

const LibraryAll = lazy(() =>
  import("./routes/LibraryAll").then((m) => ({ default: m.LibraryAll })),
);
const Workspace = lazy(() =>
  import("./routes/Workspace").then((m) => ({ default: m.Workspace })),
);
const ThreadView = lazy(() =>
  import("./routes/ThreadView").then((m) => ({ default: m.ThreadView })),
);
const Settings = lazy(() =>
  import("./routes/Settings").then((m) => ({ default: m.Settings })),
);

export function App() {
  return (
    <ToastProvider>
      <I18nProvider>
        <EngineGate>
          <AppRoutes />
        </EngineGate>
      </I18nProvider>
    </ToastProvider>
  );
}

function AppRoutes() {
  const { t } = useI18n();

  return (
    <div className="app-frame">
      <SettingsVisualBridge />
      <Suspense
        fallback={<main className="shell"><p className="hint">{t("app.loading")}</p></main>}
      >
        <Routes>
          <Route path="/" element={<Library />} />
          <Route path="/library/all" element={<LibraryAll />} />
          <Route path="/workspace/:projectId" element={<Workspace />} />
          <Route path="/workspace/:projectId/threads" element={<ThreadView />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Suspense>
    </div>
  );
}

export function applyVisualSettings(settings: Pick<Settings, "theme" | "editor_font_size" | "editor_line_height">) {
  const root = document.documentElement;
  const theme = settings.theme || "system";
  root.dataset.theme = theme;
  root.style.setProperty("--edit-size", `${settings.editor_font_size || 20}px`);
  root.style.setProperty("--edit-leading", `${settings.editor_line_height || 1.92}`);
}

function SettingsVisualBridge() {
  useEffect(() => {
    let cancelled = false;
    settingsApi.get()
      .then((settings) => {
        if (!cancelled) applyVisualSettings(settings);
      })
      .catch(() => {
        if (!cancelled) applyVisualSettings({ theme: "system", editor_font_size: 20, editor_line_height: 1.92 });
      });
    const onSettings = (event: Event) => {
      const detail = (event as CustomEvent<Settings>).detail;
      if (detail) applyVisualSettings(detail);
    };
    window.addEventListener("linetta:settings-updated", onSettings);
    return () => {
      cancelled = true;
      window.removeEventListener("linetta:settings-updated", onSettings);
    };
  }, []);
  return null;
}
