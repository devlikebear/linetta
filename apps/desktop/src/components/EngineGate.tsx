import { useEffect, useState, type ReactNode } from "react";
import { engineStatus } from "../lib/rpc";
import type { EngineStatus } from "../lib/types";

interface Props {
  children: ReactNode;
}

export function EngineGate({ children }: Props) {
  const [status, setStatus] = useState<EngineStatus | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = () => {
    setLoading(true);
    engineStatus()
      .then(setStatus)
      .catch((e) => setStatus({ ok: false, error: String(e) }))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    refresh();
  }, []);

  if (loading) {
    return (
      <main className="shell engine-gate">
        <h1>Linetta</h1>
        <p className="hint">엔진 상태를 확인하는 중...</p>
      </main>
    );
  }

  if (!status?.ok) {
    const diagnostics = buildDiagnostics(status);
    return (
      <main className="shell engine-gate">
        <h1>엔진을 시작하지 못했습니다</h1>
        <p className="hint">
          Linetta의 Go sidecar가 응답하지 않습니다. 앱을 다시 시도하거나 아래 진단 정보를 확인하세요.
        </p>
        {status?.error && <p className="error">{status.error}</p>}
        <pre className="engine-diagnostics">{diagnostics}</pre>
        <div className="engine-gate-actions">
          <button type="button" onClick={refresh}>다시 시도</button>
          <button
            type="button"
            onClick={() => navigator.clipboard?.writeText(diagnostics)}
          >
            진단 복사
          </button>
        </div>
      </main>
    );
  }

  return <>{children}</>;
}

function buildDiagnostics(status: EngineStatus | null): string {
  return JSON.stringify({
    ok: status?.ok ?? false,
    error: status?.error ?? null,
    version: status?.version ?? null,
    home: status?.home ?? null,
    db_path: status?.db_path ?? null,
    migration_version: status?.migration_version ?? null,
    migration_count: status?.migration_count ?? null,
  }, null, 2);
}
