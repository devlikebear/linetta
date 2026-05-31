import { Suspense, lazy } from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import { Library } from "./routes/Library";
import { ToastProvider } from "./components/ToastProvider";
import { EngineGate } from "./components/EngineGate";

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
      <EngineGate>
        <Suspense
          fallback={<main className="shell"><p className="hint">불러오는 중…</p></main>}
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
      </EngineGate>
    </ToastProvider>
  );
}
