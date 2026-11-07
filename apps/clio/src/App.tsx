import { useEffect, useState } from "react";
import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { EngineGate } from "./components/EngineGate";
import { posts } from "./lib/rpc";
import { t } from "./lib/i18n";
import { Import } from "./routes/Import";
import { Voice } from "./routes/Voice";
import { Write } from "./routes/Write";

/** `/` goes to the voice profile once there are posts, otherwise to the import screen. */
function Home() {
  const [target, setTarget] = useState<string | null>(null);
  useEffect(() => {
    posts
      .counts()
      .then((counts) => setTarget(counts.reduce((sum, c) => sum + c.count, 0) > 0 ? "/voice" : "/import"))
      .catch(() => setTarget("/import"));
  }, []);
  return target ? <Navigate to={target} replace /> : null;
}

export function App() {
  return (
    <EngineGate>
      <div className="layout">
        <nav className="sidebar" aria-label="main">
          <div className="brand">{t("app.name")}</div>
          <NavLink to="/import">{t("nav.import")}</NavLink>
          <NavLink to="/voice">{t("nav.voice")}</NavLink>
          <NavLink to="/write">{t("nav.write")}</NavLink>
        </nav>
        <main className="content">
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/import" element={<Import />} />
            <Route path="/voice" element={<Voice />} />
            <Route path="/write" element={<Write />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
    </EngineGate>
  );
}
