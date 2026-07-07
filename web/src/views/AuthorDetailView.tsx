import { useCallback, useEffect, useState } from "react";
import { api, type Author, type Book } from "../api";
import RemovePanel from "../components/RemovePanel";

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

  const reload = useCallback(() => {
    // Visible = member of this library AND (monitored here or owned here);
    // unmonitored, unowned members stay hidden until the post-1.0 Missing view.
    const visible = (b: Book) =>
      library === "ebook"
        ? b.inEbookLibrary && (b.ebookMonitored || b.hasEbookFile)
        : b.inAudiobookLibrary && (b.audiobookMonitored || b.hasAudiobookFile);
    Promise.all([api.getAuthor(id), api.listBooks(id)])
      .then(([a, all]) => {
        setAuthor(a);
        setBooks(all.filter(visible));
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [id, library, onError]);

  useEffect(reload, [reload]);

  if (!author) return <p className="muted">Loading author…</p>;

  const owned = books.filter((b) =>
    library === "ebook" ? b.hasEbookFile : b.hasAudiobookFile,
  ).length;

  const refresh = () => {
    setBusy(true);
    api
      .refreshAuthor(author.id)
      .then(reload)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  const remove = (deleteFiles: boolean) => {
    setBusy(true);
    api
      .deleteAuthor(author.id, deleteFiles)
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
          <img className="detail-art" src={author.imageUrl} alt="" />
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
              title="Re-fetch metadata from the provider"
              onClick={refresh}
            >
              Refresh metadata
            </button>
            <button className="danger" disabled={busy} onClick={() => setConfirmRemove(!confirmRemove)}>
              Remove author
            </button>
          </div>
          {confirmRemove && (
            <RemovePanel
              message={`Remove ${author.name} and all their books from every library?`}
              checkboxLabel="Also delete their files from disk (otherwise the next scan re-finds them as unmatched)"
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
          <p className="muted">No books in this library.</p>
        ) : (
          <div className="poster-grid">
            {books.map((b) => {
              const bookOwned = library === "ebook" ? b.hasEbookFile : b.hasAudiobookFile;
              const monitored = library === "ebook" ? b.ebookMonitored : b.audiobookMonitored;
              return (
                <button key={b.id} className="poster-card" onClick={() => onOpenBook(b.id)}>
                  {b.coverUrl ? (
                    <img className="poster" src={b.coverUrl} alt="" loading="lazy" />
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
      <h2>Missing ({missing.length})</h2>
      {missing.length === 0 ? (
        <p className="muted">
          No gaps — every book in the bibliography is in this library or owned.
        </p>
      ) : (
        <>
          <p className="muted">
            In the provider's bibliography but not in {label}. Monitor adds the
            book to this library and searches for it automatically.
          </p>
          {groups.map((g, gi) => (
            <div key={g.title || `standalone-${gi}`}>
              {hasSeries && (
                <h3 className="group-heading">{g.title || "Standalone"}</h3>
              )}
              <ul className="rows">
                {g.books.map((b) => (
                  <li key={b.id}>
                    <div className="row">
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
                      <span className="row-actions">
                        {b.rating > 0 && <span className="muted">★ {b.rating.toFixed(1)}</span>}
                        <button
                          disabled={busyID !== null}
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
                          <img className="missing-thumb" src={b.coverUrl} alt="" loading="lazy" />
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
