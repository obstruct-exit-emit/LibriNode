import { useCallback, useEffect, useState } from "react";
import {
  api,
  getApiKey,
  proxiedImage,
  type Book,
  type ReleaseCandidate,
  type RenameMove,
  type Series,
} from "../api";
import { libraryLabels } from "../App";
import RemovePanel from "../components/RemovePanel";

// Full-page series detail, *arr-style: header with cover, description and
// series-level actions, then volumes/issues as rows. Manga volumes and comic
// issues expand to show file locations (manga: owned variants too) and
// per-item monitor/remove controls; a Missing section lists items not in the
// library (neither monitored nor owned), each with a one-click Monitor —
// mirroring the per-author Missing view. Magazines stay flat (issues
// materialize from grabs and scans).
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
  const [renamePlan, setRenamePlan] = useState<RenameMove[] | null>(null);
  const [notice, setNotice] = useState("");
  const [providerOptions, setProviderOptions] = useState<string[]>([]);

  const reload = useCallback(() => {
    api
      .getSeries(id)
      .then(setSeries)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [id, onError]);

  useEffect(reload, [reload]);

  // The provider-override selector lists the providers serving this media type.
  useEffect(() => {
    api
      .getMetadataSettings()
      .then((s) =>
        setProviderOptions(
          mediaType === "manga" ? s.mangaProviders : mediaType === "comic" ? s.comicProviders : [],
        ),
      )
      .catch(() => setProviderOptions([]));
  }, [mediaType]);

  if (!series) return <p className="muted">Loading series…</p>;

  const volumes = series.volumes ?? [];
  const owned = volumes.filter((v) => v.hasFile).length;
  const unitName = mediaType === "manga" ? "volume" : "issue";
  // Manga and comics split into the library (monitored or owned) and Missing
  // (neither), like books/authors, with expandable per-item rows; magazines
  // show every issue in one flat list.
  const expandable = mediaType === "manga" || mediaType === "comic";
  const inLibrary = expandable ? volumes.filter((v) => v.monitored || v.hasFile) : volumes;
  const missing = expandable ? volumes.filter((v) => !v.monitored && !v.hasFile) : [];

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

  // Series-scoped header actions — like the author page, but only this
  // series' volumes are touched.
  const searchWanted = () => {
    setBusy(true);
    setNotice("");
    api
      .searchSeriesWanted(series.id)
      .then((r) =>
        setNotice(
          r.searched === 0
            ? "Nothing to search — every monitored volume is owned (or pending)."
            : `Searched ${r.searched} wanted volume(s), grabbed ${r.grabbed}.`,
        ),
      )
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  const scan = () => {
    setBusy(true);
    setNotice("");
    api
      .scan()
      .then((r) => {
        setNotice(
          r.roots === 0
            ? "No root folders to scan — add one under Settings."
            : `Scanned ${r.scanned} file(s): ${r.matched} matched, ${r.unmatched} unmatched.`,
        );
        reload();
      })
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  const previewRenames = () => {
    setBusy(true);
    setNotice("");
    api
      .renamePreview(undefined, series.id)
      .then((r) => {
        setRenamePlan(r.moves);
        if (r.moves.length === 0) setNotice("This series' files already match the naming templates.");
      })
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  const applyRenames = () => {
    setBusy(true);
    api
      .renameApply(undefined, series.id)
      .then((r) => {
        setNotice(`Moved ${r.moves.length} file(s)${r.skips.length ? `, ${r.skips.length} skipped` : ""}.`);
        setRenamePlan(null);
        reload();
      })
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
          <img className="detail-art" src={proxiedImage(series.coverUrl)} alt="" />
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
            <button
              disabled={busy}
              title={`Search indexers for this series' wanted ${unitName}s`}
              onClick={searchWanted}
            >
              Search wanted
            </button>
            <button disabled={busy} title="Preview naming-template moves for this series' files" onClick={previewRenames}>
              Organize…
            </button>
            <button disabled={busy} title="Scan root folders for new files" onClick={scan}>
              Scan files
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
            {series.metadataSource !== "manual" && providerOptions.length > 0 && (
              <select
                disabled={busy}
                title="Metadata provider override for this series — beats Settings → Metadata (including None) on the next refresh"
                value={series.providerOverride}
                onChange={(e) => run(() => api.setSeriesProvider(series.id, e.target.value))}
              >
                <option value="">Provider: follow settings</option>
                {providerOptions.map((name) => (
                  <option key={name} value={name}>
                    Provider: {name[0].toUpperCase() + name.slice(1)}
                  </option>
                ))}
              </select>
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
          {renamePlan && renamePlan.length > 0 && (
            <div className="rename-plan">
              <p>{renamePlan.length} file(s) would move to match the naming templates:</p>
              <ul className="rows">
                {renamePlan.map((m) => (
                  <li key={m.fileId}>
                    <div className="move">
                      <span className="file-path muted">{m.from}</span>
                      <span className="file-path">→ {m.to}</span>
                    </div>
                  </li>
                ))}
              </ul>
              <div className="settings-actions">
                <button disabled={busy} onClick={applyRenames}>Apply</button>
                <button className="toggle" onClick={() => setRenamePlan(null)}>Cancel</button>
              </div>
            </div>
          )}
        </div>
      </section>

      <section className="card">
        <h2>
          {mediaType === "manga" ? "Volumes" : "Issues"} ({inLibrary.length})
        </h2>
        {notice && <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>}
        {inLibrary.length === 0 ? (
          <p className="muted">
            {expandable
              ? `Nothing in the library — monitor ${unitName}s from Missing below.`
              : "No issues yet — they appear when grabbed or scanned."}
          </p>
        ) : (
          <ul className="rows">
            {inLibrary.map((v) =>
              expandable ? (
                <VolumeRow
                  key={v.id}
                  volume={v}
                  mediaType={mediaType}
                  onChanged={reload}
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
                    </span>
                  </div>
                </li>
              ),
            )}
          </ul>
        )}
      </section>

      {expandable && missing.length > 0 && (
        <section className="card">
          <h2>Missing ({missing.length})</h2>
          <p className="muted">
            {unitName === "volume" ? "Volumes" : "Issues"} in the series you're
            not tracking — neither monitored nor owned. Monitor adds one back
            to the library and searches for it.
          </p>
          <ul className="rows">
            {missing.map((v) => (
              <MissingRow key={v.id} volume={v} onChanged={reload} onError={onError} />
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

const variantLabel = (variant?: string) =>
  variant === "color" ? "colorized" : variant === "mono" ? "monochrome" : "";

// VolumeCover shows the volume's cover, preferring one extracted from the
// owned comic file (first page), then the provider cover, then an initial —
// falling to the next source whenever an image fails to load.
function VolumeCover({ volume }: { volume: Book }) {
  const srcs: string[] = [];
  if (volume.hasFile) {
    srcs.push(`/api/v1/book/${volume.id}/cover?apikey=${encodeURIComponent(getApiKey())}`);
  }
  const provider = proxiedImage(volume.coverUrl);
  if (provider) srcs.push(provider);

  const [idx, setIdx] = useState(0);
  if (idx >= srcs.length) {
    return <div className="missing-thumb fallback">{volume.title.charAt(0)}</div>;
  }
  return (
    <img
      className="missing-thumb"
      src={srcs[idx]}
      alt=""
      loading="lazy"
      onError={() => setIdx(idx + 1)}
    />
  );
}

// coverAbout renders the compact thumbnail + blurb shared by the volume and
// Missing rows (same look as the per-author Missing view).
function coverAbout(volume: Book) {
  return (
    <div className="volume-about">
      <VolumeCover volume={volume} />
      <p className="missing-about">
        {volume.description || "No description from the metadata provider."}
      </p>
    </div>
  );
}

// VolumeRow keeps the list compact for series with hundreds of volumes or
// issues: collapsed it's just the title + an owned/wanted badge. Expanding
// reveals the cover + blurb, where each file lives (and, for manga, which
// variants are owned), plus the same per-item controls an individual book
// gets — monitor, Auto grab, Search releases, and remove-from-library.
function VolumeRow({
  volume,
  mediaType,
  onChanged,
  onError,
}: {
  volume: Book;
  mediaType: string;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [detail, setDetail] = useState<Book | null>(null);
  const [rowBusy, setRowBusy] = useState(false);
  const [searching, setSearching] = useState(false);
  const [candidates, setCandidates] = useState<ReleaseCandidate[] | null>(null);
  const [grabNotice, setGrabNotice] = useState("");
  const [confirmRemove, setConfirmRemove] = useState(false);

  const toggle = () => {
    if (!open && !detail) {
      api
        .getBook(volume.id)
        .then(setDetail)
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
    }
    setOpen(!open);
  };

  const act = (action: () => Promise<unknown>) => {
    setRowBusy(true);
    action()
      .then(onChanged)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setRowBusy(false));
  };

  const autoGrab = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .autoSearchBook(volume.id, mediaType)
      .then((o) => {
        setGrabNotice(o.grabbed ? `✓ Grabbed "${o.release}" via ${o.client}` : `✗ ${o.message ?? "nothing grabbed"}`);
        onChanged();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const interactiveSearch = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .searchReleasesForBook(volume.id, mediaType)
      .then((r) => {
        setCandidates(r.releases);
        if (r.errors.length) setGrabNotice(`Some indexers failed: ${r.errors.join("; ")}`);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const grab = (c: ReleaseCandidate) => {
    api
      .grabRelease(c.title, c.downloadUrl, c.protocol, volume.id, mediaType, c.guid)
      .then((r) => setGrabNotice(`✓ Sent "${c.title}" to ${r.client}`))
      .catch((err: unknown) => setGrabNotice(`✗ ${err instanceof Error ? err.message : String(err)}`));
  };

  const files = (detail?.files ?? []).filter((f) => f.mediaType === mediaType);

  return (
    <li>
      <div className="row">
        <button className="link" onClick={toggle}>
          {open ? "▾" : "▸"} {volume.title}
        </button>
        <span className="row-actions">
          <span className={volume.hasFile ? "owned yes" : "owned no"}>
            {volume.hasFile ? "owned" : "wanted"}
          </span>
        </span>
      </div>
      {open && (
        <div className="book-detail">
          {coverAbout(volume)}
          {volume.hasFile && (
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
          )}
          {volume.hasFile &&
            (detail === null ? (
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
            ))}
          <div className="settings-actions">
            <button
              className={volume.monitored ? "toggle on" : "toggle"}
              disabled={rowBusy}
              title="Whether this volume is searched for automatically"
              onClick={() => act(() => api.monitorBook(volume.id, !volume.monitored))}
            >
              {volume.monitored ? "monitored" : "unmonitored"}
            </button>
            <button disabled={searching} onClick={autoGrab} title="Search indexers and grab the best release">
              {searching ? "Working…" : "Auto grab"}
            </button>
            <button disabled={searching} onClick={interactiveSearch} title="List all release candidates">
              Search releases
            </button>
            <button
              className="danger"
              disabled={rowBusy}
              title="Remove this volume from the library"
              onClick={() => setConfirmRemove(!confirmRemove)}
            >
              Remove from library
            </button>
            {grabNotice && (
              <span className={grabNotice.startsWith("✗") ? "notice bad" : "notice ok"}>{grabNotice}</span>
            )}
          </div>
          {candidates && (
            <ul className="rows nested">
              {candidates.length === 0 && <li className="muted">No releases found.</li>}
              {candidates.map((c) => (
                <li key={c.guid + c.indexer}>
                  <div className="row">
                    <span className="file-path">
                      {c.title}
                      {!c.approved && c.rejections && (
                        <span className="notice bad"> — {c.rejections.join(", ")}</span>
                      )}
                    </span>
                    <span className="row-actions">
                      <span className="muted">
                        {c.indexer} · {c.protocol}
                        {c.seeders >= 0 && ` · ${c.seeders} seeders`}
                        {c.size > 0 && ` · ${(c.size / (1 << 20)).toFixed(1)} MiB`}
                        {` · score ${c.score}`}
                      </span>
                      {c.approved && <button onClick={() => grab(c)}>Grab</button>}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {confirmRemove && (
            <RemovePanel
              message={`Remove "${volume.title}" from the library? It moves to Missing, where you can add it back.`}
              checkboxLabel="Also delete its files from disk (otherwise the next scan re-finds them)"
              busy={rowBusy}
              onConfirm={(deleteFiles) =>
                act(() => api.setBookLibrary(volume.id, mediaType, false, false, deleteFiles))
              }
              onCancel={() => setConfirmRemove(false)}
            />
          )}
        </div>
      )}
    </li>
  );
}

// MissingRow is a series bibliography gap: a volume/issue neither monitored
// nor owned. Compact by default (title + one-click Monitor); expands to the
// cover and blurb, mirroring the per-author Missing view.
function MissingRow({
  volume,
  onChanged,
  onError,
}: {
  volume: Book;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  const monitor = () => {
    setBusy(true);
    api
      .monitorBook(volume.id, true)
      .then(onChanged)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  return (
    <li>
      <div className="row">
        <button className="link" onClick={() => setOpen(!open)}>
          {open ? "▾" : "▸"} {volume.title}
        </button>
        <span className="row-actions">
          <button disabled={busy} title="Add to the library and search for it" onClick={monitor}>
            {busy ? "Adding…" : "+ Monitor"}
          </button>
        </span>
      </div>
      {open && <div className="book-detail">{coverAbout(volume)}</div>}
    </li>
  );
}
