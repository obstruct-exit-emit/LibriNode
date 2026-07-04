import { useCallback, useEffect, useState } from "react";
import { api, type Book, type Series } from "../api";

export default function SeriesView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [series, setSeries] = useState<Series[]>([]);
  const [loading, setLoading] = useState(true);
  const [openID, setOpenID] = useState<number | null>(null);

  const reload = useCallback(() => {
    api
      .listSeries()
      .then(setSeries)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError]);

  useEffect(reload, [reload]);

  if (loading) return <p className="muted">Loading series…</p>;

  return (
    <section className="card">
      <h2>Series ({series.length})</h2>
      {series.length === 0 ? (
        <p className="muted">
          No manga or comic series yet — use <strong>Search</strong> with type
          manga or comic to add one.
        </p>
      ) : (
        <ul className="rows">
          {series.map((s) => (
            <SeriesRow
              key={s.id}
              series={s}
              open={openID === s.id}
              onToggleOpen={() => setOpenID(openID === s.id ? null : s.id)}
              onChanged={reload}
              onError={onError}
            />
          ))}
        </ul>
      )}
    </section>
  );
}

function SeriesRow({
  series,
  open,
  onToggleOpen,
  onChanged,
  onError,
}: {
  series: Series;
  open: boolean;
  onToggleOpen: () => void;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [volumes, setVolumes] = useState<Book[]>([]);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    if (!open) return;
    api
      .getSeries(series.id)
      .then((detail) => setVolumes(detail.volumes ?? []))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [open, series.id, onError]);

  const run = (action: () => Promise<unknown>) => {
    setBusy(true);
    action()
      .then(onChanged)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const owned = volumes.filter((v) => v.hasFile).length;

  return (
    <li>
      <div className="row">
        <button className="link" onClick={onToggleOpen}>
          {open ? "▾" : "▸"} {series.title}
          <span className="muted"> ({series.mediaType})</span>
        </button>
        <span className="row-actions">
          {open && (
            <span className="muted">
              {owned}/{volumes.length} owned
            </span>
          )}
          <button
            className={series.monitored ? "toggle on" : "toggle"}
            disabled={busy}
            onClick={() => run(() => api.monitorSeries(series.id, !series.monitored, !series.monitored))}
          >
            {series.monitored ? "monitored" : "unmonitored"}
          </button>
          <button
            disabled={busy}
            title="Re-fetch from the metadata provider; new volumes follow the monitor-new setting"
            onClick={() => run(() => api.refreshSeries(series.id))}
          >
            refresh
          </button>
          <button
            className="danger"
            disabled={busy}
            onClick={() => {
              if (confirm(`Remove ${series.title} and all its volumes from the library?`)) {
                run(() => api.deleteSeries(series.id));
              }
            }}
          >
            remove
          </button>
        </span>
      </div>
      {open && (
        <>
          {notice && (
            <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>
          )}
          <ul className="rows nested">
            {volumes.length === 0 && <li className="muted">No volumes.</li>}
            {volumes.map((v) => (
              <li key={v.id}>
                <div className="row">
                  <span>{v.title}</span>
                  <span className="row-actions">
                    <span
                      className={v.hasFile ? "owned yes" : "owned no"}
                      title={v.hasFile ? "On disk" : "Not on disk"}
                    >
                      {v.hasFile ? "owned" : "wanted"}
                    </span>
                    {!v.hasFile && (
                      <button
                        disabled={busy}
                        title="Search indexers and grab the best release for this volume"
                        onClick={() => {
                          setBusy(true);
                          setNotice("");
                          api
                            .autoSearchBook(v.id, series.mediaType)
                            .then((o) =>
                              setNotice(
                                o.grabbed
                                  ? `✓ Grabbed "${o.release}" via ${o.client}`
                                  : `✗ ${v.title}: ${o.message ?? "nothing grabbed"}`,
                              ),
                            )
                            .catch((err: unknown) =>
                              setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
                            )
                            .finally(() => setBusy(false));
                        }}
                      >
                        Auto grab
                      </button>
                    )}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
    </li>
  );
}
