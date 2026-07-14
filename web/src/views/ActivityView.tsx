import { useCallback, useEffect, useState } from "react";
import { api, type BlockEntry, type GrabRecord, type QueueItem } from "../api";

export default function ActivityView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [items, setItems] = useState<QueueItem[]>([]);
  const [history, setHistory] = useState<GrabRecord[]>([]);
  const [blocked, setBlocked] = useState<BlockEntry[]>([]);
  const [clientErrors, setClientErrors] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [importing, setImporting] = useState(false);
  const [removing, setRemoving] = useState("");
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    Promise.all([api.queue(), api.history(), api.blocklist()])
      .then(([q, h, b]) => {
        setItems(q.items);
        setClientErrors(q.errors);
        setHistory(h);
        setBlocked(b);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError]);

  const removeItem = (it: QueueItem) => {
    if (!confirm(`Remove "${it.title}" from ${it.client}?\n\nIts downloaded data is deleted. The release is NOT blocklisted, so it can be grabbed again.`)) {
      return;
    }
    setRemoving(it.id);
    api
      .removeQueueItem(it.clientConfigId, it.id)
      .then(reload)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setRemoving(""));
  };

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
                  <button
                    className="danger"
                    disabled={removing === it.id}
                    title="Remove this download from the client (its files are deleted; the release is not blocklisted)"
                    onClick={() => removeItem(it)}
                  >
                    remove
                  </button>
                </span>
              </div>
              <div className="progress" title={`${(it.progress * 100).toFixed(0)}%`}>
                <div
                  className={`progress-fill${it.status === "failed" ? " bad" : ""}${it.status === "completed" || it.status === "seeded" ? " done" : ""}`}
                  style={{ width: `${Math.max(2, Math.min(100, it.progress * 100))}%` }}
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>

    {blocked.length > 0 && (
      <section className="card">
        <details className="disclosure">
          <summary>Blocklist ({blocked.length})</summary>
          <div className="disclosure-body">
            <p className="muted" style={{ margin: 0 }}>
              Releases that failed to download — never grabbed again. Remove an
              entry to give a release another chance.
            </p>
            <ul className="rows">
              {blocked.map((b) => (
                <li key={b.id}>
                  <div className="row">
                    <span>
                      {b.title}
                      {b.reason && <span className="file-path muted"> — {b.reason}</span>}
                    </span>
                    <span className="row-actions">
                      <button
                        className="toggle"
                        onClick={() =>
                          api
                            .unblock(b.id)
                            .then(reload)
                            .catch((err: unknown) =>
                              onError(String(err instanceof Error ? err.message : err)),
                            )
                        }
                      >
                        remove
                      </button>
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        </details>
      </section>
    )}

    {history.length > 0 && (
      <section className="card">
        <details className="disclosure">
          <summary>History ({history.length})</summary>
          <div className="disclosure-body">
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
          </div>
        </details>
      </section>
    )}
    </>
  );
}
