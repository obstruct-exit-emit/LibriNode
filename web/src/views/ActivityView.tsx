import { useCallback, useEffect, useState } from "react";
import { api, type QueueItem } from "../api";

export default function ActivityView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [items, setItems] = useState<QueueItem[]>([]);
  const [clientErrors, setClientErrors] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(() => {
    api
      .queue()
      .then((q) => {
        setItems(q.items);
        setClientErrors(q.errors);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError]);

  // Poll while the tab is open; downloads move fast.
  useEffect(() => {
    reload();
    const timer = setInterval(reload, 10_000);
    return () => clearInterval(timer);
  }, [reload]);

  if (loading) return <p className="muted">Loading queue…</p>;

  return (
    <section className="card">
      <div className="card-head">
        <h2>Queue ({items.length})</h2>
        <span className="row-actions">
          <button onClick={reload}>Refresh</button>
        </span>
      </div>
      {clientErrors.map((e) => (
        <p key={e} className="notice bad">
          {e}
        </p>
      ))}
      {items.length === 0 ? (
        <p className="muted">
          Nothing downloading. Grab releases from a book's search results, or
          check that a download client is configured under Settings.
        </p>
      ) : (
        <ul className="rows">
          {items.map((it) => (
            <li key={it.client + it.id + it.title}>
              <div className="row">
                <span>
                  {it.title}
                  {it.path && <span className="file-path muted"> → {it.path}</span>}
                </span>
                <span className="row-actions">
                  <span className="muted">{it.client}</span>
                  <span className={`owned ${it.status === "failed" ? "no" : "yes"}`}>
                    {it.status}
                    {it.status === "downloading" &&
                      ` ${(it.progress * 100).toFixed(0)}%`}
                  </span>
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
