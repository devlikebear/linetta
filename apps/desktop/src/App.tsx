import { useState } from "react";

export function App() {
  const [status, setStatus] = useState<string>("idle");
  return (
    <main className="shell">
      <h1>Linetta</h1>
      <p className="hint">Engine bridge smoke test</p>
      <button
        type="button"
        onClick={() => setStatus("not wired yet — see Task 9")}
      >
        Ping engine
      </button>
      <p className="status">{status}</p>
    </main>
  );
}
