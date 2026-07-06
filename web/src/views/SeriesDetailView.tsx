import { useCallback, useEffect, useState } from "react";
import { api, type Book, type Series } from "../api";
import { libraryLabels } from "../App";
import RemovePanel from "../components/RemovePanel";

// Full-page series detail, *arr-style: header with cover, description and
// series-level actions, then volumes/issues as clean rows.
export default function SeriesDetailView({
  id,
  mediaType,
  onError,
  onBack,
}: {
  id: number;
  mediaType: string;
  onError: (message: string) => void;
  onBack: () => void;
}) {
  const label = libraryLabels[mediaType] ?? mediaType;
  const [series, setSeries] = useState<Series | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .getSeries(id)
      .then(setSeries)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [id, onError]);

  useEffect(reload, [reload]);

  if (!series) return <p className="muted">Loading series…</p>;

  const volumes = series.volumes ?? [];
  const owned = volumes.filter((v) => v.hasFile).length;
  const unitName = mediaType === "magazine" ? "issue" : "volume";

  const run = (action: () => Promise<unknown>) => {
    setBusy(true);
    action()
      .then(reload)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const remove = (deleteFiles: boolean) => {
    setBusy(true);
    api
      .deleteSeries(series.id, deleteFiles)
      .then(onBack)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const autoGrab = (v: Book) => {
    setBusy(true);
    setNotice("");
    api
      .autoSearchBook(v.id, mediaType)
      .then((o) =>
        setNotice(
          o.grabbed
            ? `✓ Grabbed "${o.release}" via ${o.client}`
            : `✗ ${v.title}: ${o.message ?? "nothing grabbed"}`,
        ),
      )
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  return (
    <>
      <button className="link back" onClick={onBack}>
        ← {label}
      </button>

      <section className="card detail-head">
        {series.coverUrl ? (
          <img className="detail-art" src={series.coverUrl} alt="" />
        ) : (
          <div className="detail-art fallback">{series.title.charAt(0)}</div>
        )}
        <div className="detail-info">
          <h2>{series.title}</h2>
          <p className="muted">
            {volumes.length} {unitName}
            {volumes.length === 1 ? "" : "s"} · {owned} owned
          </p>
          {series.description && <p className="detail-desc">{series.description}</p>}
          <div className="settings-actions">
            <button
              className={series.monitored ? "toggle on" : "toggle"}
              disabled={busy}
              title="Whether new and missing items are searched for automatically"
              onClick={() =>
                run(() => api.monitorSeries(series.id, !series.monitored, !series.monitored))
              }
            >
              {series.monitored ? "monitored" : "unmonitored"}
            </button>
            {series.metadataSource !== "manual" && (
              <button
                disabled={busy}
                title="Re-fetch from the metadata provider; new volumes follow the monitor-new setting"
                onClick={() => run(() => api.refreshSeries(series.id))}
              >
                Refresh metadata
              </button>
            )}
            <button
              className="danger"
              disabled={busy}
              onClick={() => setConfirmRemove(!confirmRemove)}
            >
              Remove series
            </button>
          </div>
          {confirmRemove && (
            <RemovePanel
              message={`Remove ${series.title} and all its ${unitName}s from the library?`}
              checkboxLabel="Also delete its files from disk (otherwise the next scan re-finds them as unmatched)"
              busy={busy}
              onConfirm={remove}
              onCancel={() => setConfirmRemove(false)}
            />
          )}
        </div>
      </section>

      <section className="card">
        <h2>
          {mediaType === "magazine" ? "Issues" : "Volumes"} ({volumes.length})
        </h2>
        {notice && <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>}
        {volumes.length === 0 ? (
          <p className="muted">
            {mediaType === "magazine"
              ? "No issues yet — they appear when grabbed or scanned."
              : "No volumes."}
          </p>
        ) : (
          <ul className="rows">
            {volumes.map((v) => (
              <li key={v.id}>
                <div className="row">
                  <span>{v.title}</span>
                  <span className="row-actions">
                    <span className={v.hasFile ? "owned yes" : "owned no"}>
                      {v.hasFile ? "owned" : "wanted"}
                    </span>
                    {!v.hasFile && mediaType !== "magazine" && (
                      <button
                        disabled={busy}
                        title="Search indexers and grab the best release for this volume"
                        onClick={() => autoGrab(v)}
                      >
                        Auto grab
                      </button>
                    )}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}
