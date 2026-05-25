import { Link } from "react-router-dom";

export function Settings() {
  return (
    <main className="shell">
      <p>
        <Link to="/">← Library</Link>
      </p>
      <h2>Settings</h2>
      <p className="hint">Provider selection lands in Plan 6.</p>
    </main>
  );
}
