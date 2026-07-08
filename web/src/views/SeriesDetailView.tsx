import { useCallback, useEffect, useState } from "react";
import { api, type Book, type Series } from "../api";
import { libraryLabels } from "../App";
import RemovePanel from "../components/RemovePanel";

// Full-page series detail, *arr-style: header with cover, description and
// series-level actions, then volumes/issues as clean rows. Manga volumes
// expand to show their owned variants and file locations (see
// MangaVolumeRow); other types stay flat.
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
            {volumes.map((v) =>
              mediaType === "manga" ? (
                <MangaVolumeRow
                  key={v.id}
                  volume={v}
                  busy={busy}
                  onAutoGrab={autoGrab}
                  onError={onError}
                />
              ) : (
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
              ),
            )}
          </ul>
        )}
      </section>
    </>
  );
}

const variantLabel = (variant?: string) =>
  variant === "color" ? "colorized" : variant === "mono" ? "monochrome" : "";

// MangaVolumeRow keeps the list compact for series with hundreds of volumes:
// collapsed it's just the title + an owned/wanted badge (Auto grab when
// wanted). Owned volumes expand to reveal which variants are owned and where
// each file lives on disk — details that would otherwise crowd the row.
function MangaVolumeRow({
  volume,
  busy,
  onAutoGrab,
  onError,
}: {
  volume: Book;
  busy: boolean;
  onAutoGrab: (v: Book) => void;
  onError: (message: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [detail, setDetail] = useState<Book | null>(null);

  const toggle = () => {
    if (!open && !detail) {
      api
        .getBook(volume.id)
        .then(setDetail)
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
    }
    setOpen(!open);
  };

  const files = (detail?.files ?? []).filter((f) => f.mediaType === "manga");

  return (
    <li>
      <div className="row">
        {volume.hasFile ? (
          <button className="link" onClick={toggle}>
            {open ? "▾" : "▸"} {volume.title}
          </button>
        ) : (
          <span>{volume.title}</span>
        )}
        <span className="row-actions">
          <span className={volume.hasFile ? "owned yes" : "owned no"}>
            {volume.hasFile ? "owned" : "wanted"}
          </span>
          {!volume.hasFile && (
            <button
              disabled={busy}
              title="Search indexers and grab the best release for this volume"
              onClick={() => onAutoGrab(volume)}
            >
              Auto grab
            </button>
          )}
        </span>
      </div>
      {open && volume.hasFile && (
        <div className="book-detail">
          <div className="settings-actions">
            {volume.hasColorFile && (
              <span className="owned yes" title="Colorized copy owned">
                🎨 colorized
              </span>
            )}
            {volume.hasMonoFile && (
              <span className="owned yes" title="Monochrome copy owned">
                ◻️ monochrome
              </span>
            )}
          </div>
          {detail === null ? (
            <p className="muted">Loading files…</p>
          ) : files.length === 0 ? (
            <p className="muted">No files recorded.</p>
          ) : (
            <ul className="rows nested">
              {files.map((f) => (
                <li key={f.id}>
                  <div className="row">
                    <span className="file-path">
                      📄 {variantLabel(f.variant) && `${variantLabel(f.variant)} · `}
                      {f.path}
                    </span>
                    <span className="muted">
                      {f.format} · {(f.size / 1024).toFixed(0)} KiB
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </li>
  );
}
