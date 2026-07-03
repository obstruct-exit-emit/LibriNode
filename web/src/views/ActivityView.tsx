import { useCallback, useEffect, useState } from "react";
import { api, type GrabRecord, type QueueItem } from "../api";

export default function ActivityView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [items, setItems] = useState<QueueItem[]>([]);
  const [history, setHistory] = useState<GrabRecord[]>([]);
  const [clientErrors, setClientErrors] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [importing, setImporting] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    Promise.all([api.queue(), api.history()])
      .then(([q, h]) => {
        setItems(q.items);
        setClientErrors(q.errors);
        setHistory(h);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError]);

  const runImport = () => {
    setImporting(true);
    setNotice("");
    api
      .runImport()
      .then((r) => {
        const extra = r.messages?.length ? ` — ${r.messages.join("; ")}` : "";
        setNotice(`Imported ${r.imported}, failed ${r.failed}, skipped ${r.skipped}${extra}`);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setImporting(false));
  };

  // Poll while the tab is open; downloads move fast.
  useEffect(() => {
    reload();
    const timer = setInterval(reload, 10_000);
    return () => clearInterval(timer);
  }, [reload]);

  if (loading) return <p className="muted">Loading queue…</p>;

  return (
    <>
    <section className="card">
      <div className="card-head">
        <h2>Queue ({items.length})</h2>
        <span className="row-actions">
          <button disabled={importing} onClick={runImport} title="Import finished downloads now">
            {importing ? "Importing…" : "Import now"}
          </button>
          <button onClick={reload}>Refresh</button>
        </span>
      </div>
      {notice && <p className="muted">{notice}</p>}
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

    {history.length > 0 && (
      <section className="card">
        <h2>History ({history.length})</h2>
        <ul className="rows">
          {history.map((g) => (
            <li key={g.id}>
              <div className="row">
                <span>
                  {g.title}
                  {g.message && <span className="file-path muted"> — {g.message}</span>}
                </span>
                <span className="row-actions">
                  <span className="muted">{g.protocol}</span>
                  <span className={`owned ${g.status === "failed" ? "no" : "yes"}`}>
                    {g.status}
                  </span>
                </span>
              </div>
            </li>
          ))}
        </ul>
      </section>
    )}
    </>
  );
}
