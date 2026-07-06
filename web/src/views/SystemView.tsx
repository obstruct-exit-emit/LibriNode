import { useCallback, useEffect, useState } from "react";
import { api, type HealthResult, type SystemStatus } from "../api";

export default function SystemView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [status, setStatus] = useState<SystemStatus | null>(null);

  useEffect(() => {
    api
      .systemStatus()
      .then(setStatus)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  if (!status) return <p className="muted">Loading…</p>;

  return (
    <>
      <HealthCard onError={onError} />
      <section className="card">
        <h2>System</h2>
        <dl>
          <dt>Version</dt>
          <dd>{status.appVersion ?? status.version}</dd>
          <dt>Platform</dt>
          <dd>
            {status.os}/{status.arch}
          </dd>
          <dt>Uptime</dt>
          <dd>{status.uptime}</dd>
          <dt>Started</dt>
          <dd>{status.startTime}</dd>
          <dt>Data directory</dt>
          <dd>
            <code>{status.dataDir}</code>
          </dd>
        </dl>
      </section>
    </>
  );
}

// HealthCard shows the latest background check results and can re-run them
// on demand (checks cover root folders, indexers, download clients, and the
// metadata provider token).
function HealthCard({ onError }: { onError: (message: string) => void }) {
  const [result, setResult] = useState<HealthResult | null>(null);
  const [busy, setBusy] = useState(false);

  const reload = useCallback(() => {
    api
      .health()
      .then(setResult)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const runNow = () => {
    setBusy(true);
    api
      .checkHealth()
      .then(setResult)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const hasRun = result && !result.checkedAt.startsWith("0001-");

  return (
    <section className="card">
      <div className="card-head">
        <h2>Health</h2>
        <button disabled={busy} onClick={runNow} title="Re-run every check now">
          {busy ? "Checking…" : "Run checks now"}
        </button>
      </div>
      <p className="muted">
        Root folders, indexers, download clients, and the metadata provider
        are checked in the background every 15 minutes.
        {hasRun && ` Last run: ${new Date(result.checkedAt).toLocaleString()}.`}
      </p>
      {!hasRun ? (
        <p className="muted">No check has completed yet.</p>
      ) : result.issues.length === 0 ? (
        <p className="notice ok">✓ All checks passed</p>
      ) : (
        <ul className="rows">
          {result.issues.map((issue, i) => (
            <li key={i}>
              <p className={issue.level === "error" ? "health-issue error" : "health-issue"}>
                {issue.level === "error" ? "⛔" : "⚠️"} {issue.message}
              </p>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
