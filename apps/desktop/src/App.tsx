import { Routes, Route, Navigate } from "react-router-dom";
import { Library } from "./routes/Library";
import { LibraryAll } from "./routes/LibraryAll";
import { Workspace } from "./routes/Workspace";
import { ThreadView } from "./routes/ThreadView";
import { Settings } from "./routes/Settings";

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Library />} />
      <Route path="/library/all" element={<LibraryAll />} />
      <Route path="/workspace/:projectId" element={<Workspace />} />
      <Route path="/workspace/:projectId/threads" element={<ThreadView />} />
      <Route path="/settings" element={<Settings />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
