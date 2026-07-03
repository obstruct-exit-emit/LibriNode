import { useEffect, useState } from "react";
import { api, type SystemStatus } from "../api";

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
    <section className="card">
      <h2>System</h2>
      <dl>
        <dt>Version</dt>
        <dd>{status.version}</dd>
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
  );
}
