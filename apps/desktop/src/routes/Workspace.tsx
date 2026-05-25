import { useParams, Link } from "react-router-dom";

export function Workspace() {
  const { projectId } = useParams();
  return (
    <main className="shell">
      <p>
        <Link to="/">← Library</Link>
      </p>
      <h2>Workspace placeholder</h2>
      <p className="hint">
        Project <code>{projectId}</code>. The editor lands in Plan 2.
      </p>
    </main>
  );
}
