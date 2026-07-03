import { useCallback, useEffect, useState } from "react";
import { api, type Author, type Book, type BookFile, type Edition } from "../api";

export default function LibraryView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [authors, setAuthors] = useState<Author[]>([]);
  const [unmatched, setUnmatched] = useState<BookFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [openAuthor, setOpenAuthor] = useState<number | null>(null);
  const [scanning, setScanning] = useState(false);
  const [scanNotice, setScanNotice] = useState("");

  const reload = useCallback(() => {
    Promise.all([api.listAuthors(), api.listUnmatchedFiles()])
      .then(([au, un]) => {
        setAuthors(au);
        setUnmatched(un);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError]);

  useEffect(reload, [reload]);

  const scan = () => {
    setScanning(true);
    setScanNotice("");
    api
      .scan()
      .then((r) => {
        const errors = r.errors?.length ? `, ${r.errors.length} root(s) failed` : "";
        setScanNotice(
          r.roots === 0
            ? "No ebook root folders to scan — add one under Settings."
            : `Scanned ${r.scanned} file(s): ${r.matched} matched, ${r.unmatched} unmatched, ${r.removed} removed${errors}`,
        );
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setScanning(false));
  };

  if (loading) return <p className="muted">Loading library…</p>;

  return (
    <>
      <section className="card">
        <div className="card-head">
          <h2>Authors ({authors.length})</h2>
          <span className="row-actions">
            <button disabled={scanning} onClick={scan} title="Scan root folders for ebook files">
              {scanning ? "Scanning…" : "Scan files"}
            </button>
          </span>
        </div>
        {scanNotice && <p className="muted">{scanNotice}</p>}
        {authors.length === 0 ? (
          <p className="muted">
            Nothing here yet — use <strong>Search</strong> to find an author or
            book and add it to the library.
          </p>
        ) : (
          <ul className="rows">
            {authors.map((a) => (
              <AuthorRow
                key={a.id}
                author={a}
                open={openAuthor === a.id}
                onToggleOpen={() => setOpenAuthor(openAuthor === a.id ? null : a.id)}
                onChanged={reload}
                onError={onError}
              />
            ))}
          </ul>
        )}
      </section>

      {unmatched.length > 0 && (
        <section className="card">
          <h2>Unmatched files ({unmatched.length})</h2>
          <p className="muted">
            Found on disk but not matched to any library book. Add the books
            (Search tab) and re-scan, or wait for manual import to land.
          </p>
          <ul className="rows">
            {unmatched.map((f) => (
              <li key={f.id}>
                <div className="row">
                  <span className="file-path">{f.path}</span>
                  <span className="muted">{f.format}</span>
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

function AuthorRow({
  author,
  open,
  onToggleOpen,
  onChanged,
  onError,
}: {
  author: Author;
  open: boolean;
  onToggleOpen: () => void;
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [books, setBooks] = useState<Book[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!open) return;
    api
      .listBooks(author.id)
      .then(setBooks)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [open, author.id, onError]);

  const run = (action: () => Promise<unknown>) => {
    setBusy(true);
    action()
      .then(onChanged)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  return (
    <li>
      <div className="row">
        <button className="link" onClick={onToggleOpen}>
          {open ? "▾" : "▸"} {author.name}
        </button>
        <span className="row-actions">
          <button
            className={author.monitored ? "toggle on" : "toggle"}
            disabled={busy}
            title={author.monitored ? "Monitored — click to unmonitor" : "Unmonitored — click to monitor"}
            onClick={() => run(() => api.monitorAuthor(author.id, !author.monitored))}
          >
            {author.monitored ? "monitored" : "unmonitored"}
          </button>
          <button
            disabled={busy}
            title="Re-fetch metadata from the provider"
            onClick={() => run(() => api.refreshAuthor(author.id))}
          >
            refresh
          </button>
          <button
            className="danger"
            disabled={busy}
            onClick={() => {
              if (confirm(`Remove ${author.name} and all their books from the library?`)) {
                run(() => api.deleteAuthor(author.id));
              }
            }}
          >
            remove
          </button>
        </span>
      </div>
      {open && (
        <ul className="rows nested">
          {books.length === 0 && <li className="muted">No books.</li>}
          {books.map((b) => (
            <BookRow key={b.id} book={b} onError={onError} />
          ))}
        </ul>
      )}
    </li>
  );
}

function BookRow({
  book,
  onError,
}: {
  book: Book;
  onError: (message: string) => void;
}) {
  const [detail, setDetail] = useState<Book | null>(null);
  const [open, setOpen] = useState(false);
  const [monitored, setMonitored] = useState(book.monitored);

  const loadDetail = () => {
    if (!open) {
      api
        .getBook(book.id)
        .then(setDetail)
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
    }
    setOpen(!open);
  };

  const toggleMonitor = () => {
    api
      .monitorBook(book.id, !monitored)
      .then(() => setMonitored(!monitored))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  };

  const year = book.releaseDate ? ` (${book.releaseDate.slice(0, 4)})` : "";

  return (
    <li>
      <div className="row">
        <button className="link" onClick={loadDetail}>
          {open ? "▾" : "▸"} {book.title}
          <span className="muted">{year}</span>
        </button>
        <span className="row-actions">
          <span
            className={book.hasFile ? "owned yes" : "owned no"}
            title={book.hasFile ? "File on disk" : "No file yet"}
          >
            {book.hasFile ? "owned" : "wanted"}
          </span>
          {book.rating > 0 && <span className="muted">★ {book.rating.toFixed(1)}</span>}
          <button className={monitored ? "toggle on" : "toggle"} onClick={toggleMonitor}>
            {monitored ? "monitored" : "unmonitored"}
          </button>
        </span>
      </div>
      {open && detail && (
        <div className="book-detail">
          {detail.series && detail.series.length > 0 && (
            <p className="muted">
              {detail.series
                .map((s) => `${s.title} #${s.position}`)
                .join(", ")}
            </p>
          )}
          {detail.files && detail.files.length > 0 && (
            <ul className="rows nested">
              {detail.files.map((f) => (
                <li key={f.id}>
                  <div className="row">
                    <span className="file-path">📄 {f.path}</span>
                    <span className="muted">
                      {f.format} · {(f.size / 1024).toFixed(0)} KiB
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {detail.editions && detail.editions.length > 0 ? (
            <ul className="rows nested">
              {detail.editions.map((e) => (
                <EditionRow key={e.id} edition={e} onError={onError} />
              ))}
            </ul>
          ) : (
            <p className="muted">
              No editions cached — refresh the book to fetch them.
            </p>
          )}
        </div>
      )}
    </li>
  );
}

function EditionRow({
  edition,
  onError,
}: {
  edition: Edition;
  onError: (message: string) => void;
}) {
  const [monitored, setMonitored] = useState(edition.monitored);

  const label = [
    edition.format,
    edition.isbn13 && `ISBN ${edition.isbn13}`,
    edition.asin && `ASIN ${edition.asin}`,
    edition.publisher,
    edition.language,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <li>
      <div className="row">
        <span>{label || "edition"}</span>
        <button
          className={monitored ? "toggle on" : "toggle"}
          onClick={() =>
            api
              .monitorEdition(edition.id, !monitored)
              .then(() => setMonitored(!monitored))
              .catch((err: unknown) =>
                onError(String(err instanceof Error ? err.message : err)),
              )
          }
        >
          {monitored ? "monitored" : "unmonitored"}
        </button>
      </div>
    </li>
  );
}
