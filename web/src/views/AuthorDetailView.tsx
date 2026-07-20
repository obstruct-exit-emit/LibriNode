import { useCallback, useEffect, useState } from "react";
import { api, proxiedImage, type Author, type Book, type RenameMove } from "../api";
import RemovePanel from "../components/RemovePanel";
import { DetailSkeleton } from "../components/Skeleton";

// Full-page author detail, *arr-style: header with portrait, description and
// author-level actions, then this library's books as a cover grid — clicking
// a card opens the book's own page.
export default function AuthorDetailView({
  id,
  library,
  onError,
  onBack,
  onOpenBook,
}: {
  id: number;
  library: "ebook" | "audiobook";
  onError: (message: string) => void;
  onBack: () => void;
  onOpenBook: (bookId: number) => void;
}) {
  const label = library === "ebook" ? "Ebooks" : "Audiobooks";
  const [author, setAuthor] = useState<Author | null>(null);
  const [books, setBooks] = useState<Book[]>([]);
  const [busy, setBusy] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [notice, setNotice] = useState("");
  const [renamePlan, setRenamePlan] = useState<RenameMove[] | null>(null);
  const [providerOptions, setProviderOptions] = useState<string[]>([]);

  // The provider-override selector lists the registered book providers.
  useEffect(() => {
    api
      .getMetadataSettings()
      .then((s) => setProviderOptions(s.available))
      .catch(() => setProviderOptions([]));
  }, []);

  const reload = useCallback(() => {
    // Visible = enrolled in this library, monitored or not. Monitoring only
    // controls auto-grab/upgrade; it never hides a book. Everything NOT
    // enrolled lives in the Missing section below.
    const visible = (b: Book) =>
      library === "ebook" ? b.inEbookLibrary : b.inAudiobookLibrary;
    Promise.all([api.getAuthor(id), api.listBooks(id)])
      .then(([a, all]) => {
        setAuthor(a);
        setBooks(all.filter(visible));
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [id, library, onError]);

  useEffect(reload, [reload]);

  if (!author) return <DetailSkeleton />;

  const owned = books.filter((b) =>
    library === "ebook" ? b.hasEbookFile : b.hasAudiobookFile,
  ).length;

  // headerAction runs one of the author-scoped buttons and reports back.
  const headerAction = (action: () => Promise<string>) => {
    setBusy(true);
    setNotice("");
    action()
      .then((msg) => {
        setNotice(msg);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const refresh = () =>
    headerAction(async () => {
      await api.refreshAuthor(author.id);
      return "✓ Metadata refreshed";
    });

  const searchWanted = () =>
    headerAction(async () => {
      const r = await api.searchAuthorWanted(author.id, library);
      const details = r.outcomes
        .filter((o) => !o.grabbed && o.message)
        .slice(0, 3)
        .map((o) => `${o.bookTitle}: ${o.message}`)
        .join("; ");
      return r.searched === 0
        ? "Nothing to search — every monitored book here is owned (or pending)."
        : `Searched ${r.searched} wanted book(s), grabbed ${r.grabbed}.${details ? " " + details : ""}`;
    });

  const scan = () =>
    headerAction(async () => {
      const r = await api.scan(library);
      return r.roots === 0
        ? `No ${label} root folders to scan — add one under Settings.`
        : `Scanned ${r.scanned} file(s): ${r.matched} matched, ${r.unmatched} unmatched.`;
    });

  const previewRenames = async () => {
    setBusy(true);
    setNotice("");
    try {
      // Scan this library first (scoped — never every library) so the plan
      // reflects what's actually on disk.
      await api.scan(library);
      const r = await api.renamePreview(author.id);
      setRenamePlan(r.moves);
      if (r.moves.length === 0) setNotice("This author's files already match the naming templates.");
    } catch (err) {
      onError(String(err instanceof Error ? err.message : err));
    } finally {
      setBusy(false);
    }
  };

  const applyRenames = () => {
    setBusy(true);
    api
      .renameApply(author.id)
      .then((r) => {
        setNotice(`Moved ${r.moves.length} file(s)${r.skips.length ? `, ${r.skips.length} skipped` : ""}.`);
        setRenamePlan(null);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const remove = (deleteFiles: boolean) => {
    setBusy(true);
    api
      .removeAuthorFromLibrary(author.id, library, deleteFiles)
      .then(onBack)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  return (
    <>
      <button className="link back" onClick={onBack}>
        ← {label}
      </button>

      <section className="card detail-head">
        {author.imageUrl ? (
          <img className="detail-art" src={proxiedImage(author.imageUrl)} alt={`Portrait of ${author.name}`} />
        ) : (
          <div className="detail-art fallback">{author.name.charAt(0)}</div>
        )}
        <div className="detail-info">
          <h2>{author.name}</h2>
          <p className="muted">
            {books.length} book{books.length === 1 ? "" : "s"} in {label} · {owned} owned
          </p>
          {author.description && <p className="detail-desc">{author.description}</p>}
          <div className="settings-actions">
            <button
              disabled={busy}
              title={`Search indexers for this author's wanted ${library}s only`}
              onClick={searchWanted}
            >
              Search wanted
            </button>
            <button
              disabled={busy}
              title="Preview naming-template moves for this author's files only"
              onClick={previewRenames}
            >
              Organize…
            </button>
            <button disabled={busy} title="Scan root folders for new files" onClick={scan}>
              Scan files
            </button>
            <button
              disabled={busy}
              title="Re-fetch metadata from the provider (never changes what's monitored)"
              onClick={refresh}
            >
              Refresh metadata
            </button>
            {providerOptions.length > 0 && (
              <select
                disabled={busy}
                title="Metadata provider override for this author — beats Settings → Metadata (including None) on the next refresh"
                value={author.providerOverride}
                onChange={(e) =>
                  headerAction(async () => {
                    await api.setAuthorProvider(author.id, e.target.value);
                    return e.target.value
                      ? `✓ Provider pinned to ${e.target.value}`
                      : "✓ Provider follows Settings → Metadata";
                  })
                }
              >
                <option value="">Provider: follow settings</option>
                {providerOptions.map((name) => (
                  <option key={name} value={name}>
                    Provider: {name[0].toUpperCase() + name.slice(1)}
                  </option>
                ))}
              </select>
            )}
            <button className="danger" disabled={busy} onClick={() => setConfirmRemove(!confirmRemove)}>
              Remove from {label}
            </button>
          </div>
          {notice && <p className="muted">{notice}</p>}
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
          {confirmRemove && (
            <RemovePanel
              message={`Remove ${author.name} from ${label}? The other library is untouched; if this was their last library, the author is deleted entirely.`}
              checkboxLabel={`Also delete their ${library} files from disk`}
              busy={busy}
              onConfirm={remove}
              onCancel={() => setConfirmRemove(false)}
            />
          )}
        </div>
      </section>

      <section className="card">
        <h2>Books ({books.length})</h2>
        {books.length === 0 ? (
          <p className="muted">
            Nothing here yet — pick books to monitor from{" "}
            <strong>Missing</strong> below, or scan a root folder with their
            files.
          </p>
        ) : (
          <div className="poster-grid">
            {books.map((b) => {
              const bookOwned = library === "ebook" ? b.hasEbookFile : b.hasAudiobookFile;
              const monitored = library === "ebook" ? b.ebookMonitored : b.audiobookMonitored;
              return (
                <button key={b.id} className="poster-card" onClick={() => onOpenBook(b.id)}>
                  {b.coverUrl ? (
                    <img className="poster" src={proxiedImage(b.coverUrl)} alt="" loading="lazy" />
                  ) : (
                    <div className="poster fallback">{b.title.charAt(0)}</div>
                  )}
                  <span className="poster-title">{b.title}</span>
                  <span className="poster-sub">
                    {b.releaseDate ? b.releaseDate.slice(0, 4) + " · " : ""}
                    {bookOwned ? "owned" : "wanted"}
                    {!monitored && " · unmonitored"}
                  </span>
                </button>
              );
            })}
          </div>
        )}
      </section>

      <MissingCard authorId={id} library={library} refresh={books} onMonitored={reload} onError={onError} />
    </>
  );
}

// MissingCard lists the author's bibliography gaps — books from the
// metadata provider that are neither monitored nor owned in this library —
// grouped by series (then standalones by year). Rows expand to a compact
// thumbnail + blurb; the Monitor button works without expanding.
function MissingCard({
  authorId,
  library,
  refresh,
  onMonitored,
  onError,
}: {
  authorId: number;
  library: "ebook" | "audiobook";
  refresh: unknown; // changes whenever the parent reloaded its book list
  onMonitored: () => void;
  onError: (message: string) => void;
}) {
  const label = library === "ebook" ? "Ebooks" : "Audiobooks";
  const [missing, setMissing] = useState<Book[] | null>(null);
  const [open, setOpen] = useState<number | null>(null);
  const [busyID, setBusyID] = useState<number | null>(null);
  const [busyAll, setBusyAll] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());

  useEffect(() => {
    api
      .authorMissing(authorId, library)
      .then(setMissing)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [authorId, library, refresh, onError]);

  if (!missing) return null;

  const monitor = (b: Book) => {
    setBusyID(b.id);
    api
      .setBookLibrary(b.id, library, true, true)
      .then(onMonitored) // the book moves up into the Books grid
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusyID(null));
  };

  // Bulk monitor: a whole series group, or the checked rows.
  const monitorMany = (books: Book[]) => {
    setBusyAll(true);
    Promise.allSettled(books.map((b) => api.setBookLibrary(b.id, library, true, true)))
      .then((results) => {
        const failed = results.filter((r) => r.status === "rejected").length;
        if (failed > 0) onError(`${failed} of ${books.length} could not be monitored`);
        setSelected(new Set());
        onMonitored();
      })
      .finally(() => setBusyAll(false));
  };

  const toggleSelect = (id: number) => {
    const next = new Set(selected);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    setSelected(next);
  };

  // Grouping: the backend orders series (alphabetical, by position), then
  // standalones by release date — preserve that order while grouping.
  const groups: { title: string; books: Book[] }[] = [];
  for (const b of missing) {
    const title = b.series?.[0]?.title ?? "";
    const last = groups[groups.length - 1];
    if (last && last.title === title) {
      last.books.push(b);
    } else {
      groups.push({ title, books: [b] });
    }
  }
  const hasSeries = groups.some((g) => g.title !== "");

  return (
    <section className="card">
      <div className="card-head">
        <h2>Missing ({missing.length})</h2>
        {selected.size > 0 && (
          <button
            disabled={busyAll}
            title={`Monitor the ${selected.size} checked book(s)`}
            onClick={() => monitorMany(missing.filter((b) => selected.has(b.id)))}
          >
            {busyAll ? "Monitoring…" : `+ Monitor selected (${selected.size})`}
          </button>
        )}
      </div>
      {missing.length === 0 ? (
        <p className="muted">
          No gaps — every book in the bibliography is in this library or owned.
        </p>
      ) : (
        <>
          <p className="muted">
            In the provider's bibliography but not in {label}. Monitor adds the
            book to this library and searches for it automatically — check
            several rows to monitor them in one go.
          </p>
          {groups.map((g, gi) => (
            <div key={g.title || `standalone-${gi}`}>
              {hasSeries && (
                <h3 className="group-heading">
                  {g.title || "Standalone"}
                  {g.books.length > 1 && (
                    <button
                      className="toggle group-monitor"
                      disabled={busyAll}
                      title={
                        g.title
                          ? `Monitor all ${g.books.length} missing ${g.title} books`
                          : `Monitor all ${g.books.length} standalones`
                      }
                      onClick={() => monitorMany(g.books)}
                    >
                      + Monitor all ({g.books.length})
                    </button>
                  )}
                </h3>
              )}
              <ul className="rows">
                {g.books.map((b) => (
                  <li key={b.id}>
                    <div className="row">
                      <span className="row-select">
                        <input
                          type="checkbox"
                          aria-label={`Select ${b.title}`}
                          checked={selected.has(b.id)}
                          onChange={() => toggleSelect(b.id)}
                        />
                        <button
                          className="link"
                          onClick={() => setOpen(open === b.id ? null : b.id)}
                        >
                          {open === b.id ? "▾" : "▸"} {b.title}
                          <span className="muted">
                            {b.series?.[0] ? ` #${b.series[0].position}` : ""}
                            {b.releaseDate ? ` (${b.releaseDate.slice(0, 4)})` : ""}
                          </span>
                        </button>
                      </span>
                      <span className="row-actions">
                        {b.rating > 0 && <span className="muted">★ {b.rating.toFixed(1)}</span>}
                        <button
                          disabled={busyID !== null || busyAll}
                          title={`Add to ${label} and search for it automatically`}
                          onClick={() => monitor(b)}
                        >
                          {busyID === b.id ? "Adding…" : "+ Monitor"}
                        </button>
                      </span>
                    </div>
                    {open === b.id && (
                      <div className="missing-detail">
                        {b.coverUrl ? (
                          <img className="missing-thumb" src={proxiedImage(b.coverUrl)} alt="" loading="lazy" />
                        ) : (
                          <div className="missing-thumb fallback">{b.title.charAt(0)}</div>
                        )}
                        <p className="missing-about">
                          {b.description || "No description from the metadata provider."}
                        </p>
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </>
      )}
    </section>
  );
}
