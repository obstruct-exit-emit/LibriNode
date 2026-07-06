import { useCallback, useEffect, useState } from "react";
import { api, type Series, type SeriesResult } from "../api";
import { libraryLabels } from "../App";
import WantedCard from "../components/WantedCard";

// A series-first library area (Manga, Comics, or Magazines) — a *arr-style
// poster grid of series; clicking one opens its full detail page. Adding is
// scoped to this library.
export default function SeriesLibraryView({
  mediaType,
  onError,
  onOpenSeries,
}: {
  mediaType: string;
  onError: (message: string) => void;
  onOpenSeries: (id: number) => void;
}) {
  const [series, setSeries] = useState<Series[]>([]);
  const [loading, setLoading] = useState(true);
  const [showAdd, setShowAdd] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .listSeries()
      .then((all) => setSeries(all.filter((s) => s.mediaType === mediaType)))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError, mediaType]);

  useEffect(reload, [reload]);

  const searchWanted = () => {
    setBusy(true);
    setNotice("");
    api
      .searchWanted()
      .then((r) =>
        setNotice(
          r.searched === 0
            ? "Nothing to search — everything monitored is owned (or pending)."
            : `Searched ${r.searched} wanted item(s), grabbed ${r.grabbed}.`,
        ),
      )
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const scan = () => {
    setBusy(true);
    setNotice("");
    api
      .scan()
      .then((r) => {
        setNotice(`Scanned ${r.scanned} file(s): ${r.matched} matched, ${r.unmatched} unmatched.`);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  if (loading) return <p className="muted">Loading…</p>;

  return (
    <>
    <section className="card">
      <div className="card-head">
        <h2>
          {libraryLabels[mediaType] ?? mediaType} ({series.length})
        </h2>
        <span className="row-actions">
          <button onClick={() => setShowAdd(!showAdd)}>{showAdd ? "Close" : "+ Add"}</button>
          <button disabled={busy} onClick={searchWanted}>Search wanted</button>
          <button disabled={busy} onClick={scan}>Scan files</button>
        </span>
      </div>
      {notice && <p className="muted">{notice}</p>}

      {showAdd &&
        (mediaType === "magazine" ? (
          <AddMagazinePanel onAdded={reload} />
        ) : (
          <AddSeriesPanel mediaType={mediaType} onAdded={reload} />
        ))}

      {series.length === 0 ? (
        <p className="muted">
          This library is empty — use <strong>+ Add</strong>
          {mediaType === "magazine"
            ? " to add a magazine by name."
            : ` to search for ${mediaType} series.`}
        </p>
      ) : (
        <div className="poster-grid">
          {series.map((s) => (
            <button key={s.id} className="poster-card" onClick={() => onOpenSeries(s.id)}>
              {s.coverUrl ? (
                <img className="poster" src={s.coverUrl} alt="" loading="lazy" />
              ) : (
                <div className="poster fallback">{s.title.charAt(0)}</div>
              )}
              <span className="poster-title">{s.title}</span>
              <span className="poster-sub">
                {s.ownedCount}/{s.itemCount} owned
                {!s.monitored && " · unmonitored"}
              </span>
            </button>
          ))}
        </div>
      )}
    </section>

    <WantedCard key={`wanted-${mediaType}`} library={mediaType} onError={onError} />
    </>
  );
}

function AddSeriesPanel({
  mediaType,
  onAdded,
}: {
  mediaType: string;
  onAdded: () => void;
}) {
  const [term, setTerm] = useState("");
  const [results, setResults] = useState<SeriesResult[]>([]);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    if (!term.trim()) return;
    setBusy(true);
    setNotice("");
    api
      .searchSeries(term, mediaType)
      .then(setResults)
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  const add = (r: SeriesResult) => {
    setBusy(true);
    setNotice("");
    api
      .addSeries(mediaType, r.foreignSeriesId)
      .then(() => {
        setNotice(`✓ Added "${r.title}"`);
        onAdded();
      })
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  return (
    <div className="add-panel">
      <form onSubmit={search} className="search-form">
        <input
          placeholder={`Search ${mediaType} series…`}
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          autoFocus
        />
        <button type="submit" disabled={busy || !term.trim()}>Search</button>
      </form>
      {notice && <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>}
      <ul className="rows">
        {results.map((r) => (
          <li key={r.foreignSeriesId}>
            <div className="row">
              <span>
                {r.title}
                <span className="muted">
                  {r.year ? ` · ${r.year}` : ""}
                  {r.authorName ? ` · ${r.authorName}` : ""}
                  {r.issueCount > 0 ? ` · ${r.issueCount} volumes` : " · volume count TBD"}
                </span>
              </span>
              <button disabled={busy} onClick={() => add(r)}>Add series</button>
            </div>
          </li>
        ))}
      </ul>
    </div>
  );
}

function AddMagazinePanel({ onAdded }: { onAdded: () => void }) {
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const add = (e: React.FormEvent) => {
    e.preventDefault();
    const t = title.trim();
    if (!t) return;
    setBusy(true);
    setNotice("");
    api
      .addMagazine(t)
      .then(() => {
        setTitle("");
        setNotice(`✓ Added "${t}" — new issues are grabbed as they appear on your indexers`);
        onAdded();
      })
      .catch((err: unknown) => setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`))
      .finally(() => setBusy(false));
  };

  return (
    <div className="add-panel">
      <form onSubmit={add} className="search-form">
        <input
          placeholder="Magazine name (e.g. The Economist) — issues match by date"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          autoFocus
        />
        <button type="submit" disabled={busy || !title.trim()}>Add magazine</button>
      </form>
      {notice && <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>}
    </div>
  );
}
