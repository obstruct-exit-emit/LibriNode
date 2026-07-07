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
    </>
  );
}
