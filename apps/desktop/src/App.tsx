import { useState } from "react";
import { enginePing } from "./lib/rpc";

export function App() {
  const [status, setStatus] = useState<string>("idle");
  const [pending, setPending] = useState<boolean>(false);

  const onPing = async () => {
    setPending(true);
    try {
      const reply = await enginePing();
      setStatus(`engine says: ${reply}`);
    } catch (err) {
      setStatus(`error: ${String(err)}`);
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="shell">
      <h1>Linetta</h1>
      <p className="hint">Engine bridge smoke test</p>
      <button type="button" onClick={onPing} disabled={pending}>
        {pending ? "pinging…" : "Ping engine"}
      </button>
      <p className="status">{status}</p>
    </main>
  );
}
