import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Author,
  type Book,
  type BookFile,
  type Edition,
  type ReleaseCandidate,
  type RenameMove,
} from "../api";

export default function LibraryView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [authors, setAuthors] = useState<Author[]>([]);
  const [allBooks, setAllBooks] = useState<Book[]>([]);
  const [unmatched, setUnmatched] = useState<BookFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [openAuthor, setOpenAuthor] = useState<number | null>(null);
  const [scanning, setScanning] = useState(false);
  const [scanNotice, setScanNotice] = useState("");
  const [renamePlan, setRenamePlan] = useState<RenameMove[] | null>(null);
  const [organizing, setOrganizing] = useState(false);

  const reload = useCallback(() => {
    Promise.all([api.listAuthors(), api.listBooks(), api.listUnmatchedFiles()])
      .then(([au, bk, un]) => {
        setAuthors(au);
        setAllBooks(bk);
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

  const searchWanted = () => {
    setScanning(true);
    setScanNotice("");
    api
      .searchWanted()
      .then((r) => {
        const details = r.outcomes
          .filter((o) => !o.grabbed && o.message)
          .slice(0, 3)
          .map((o) => `${o.bookTitle}: ${o.message}`)
          .join("; ");
        setScanNotice(
          r.searched === 0
            ? "Nothing to search — every monitored book has a file (or a pending grab)."
            : `Searched ${r.searched} wanted book(s), grabbed ${r.grabbed}.${details ? " " + details : ""}`,
        );
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setScanning(false));
  };

  const previewRenames = () => {
    setOrganizing(true);
    api
      .renamePreview()
      .then((r) => {
        setRenamePlan(r.moves);
        if (r.moves.length === 0) {
          setScanNotice("All files already match the naming templates.");
          setRenamePlan(null);
        }
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setOrganizing(false));
  };

  const applyRenames = () => {
    setOrganizing(true);
    api
      .renameApply()
      .then((r) => {
        const skipped = r.skips.length ? `, ${r.skips.length} skipped` : "";
        setScanNotice(`Moved ${r.moves.length} file(s)${skipped}.`);
        setRenamePlan(null);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setOrganizing(false));
  };

  if (loading) return <p className="muted">Loading library…</p>;

  return (
    <>
      <section className="card">
        <div className="card-head">
          <h2>Authors ({authors.length})</h2>
          <span className="row-actions">
            <button
              disabled={scanning}
              onClick={searchWanted}
              title="Search indexers for every monitored book without a file and grab the best releases"
            >
              Search wanted
            </button>
            <button
              disabled={organizing}
              onClick={previewRenames}
              title="Preview moving files into the naming-template layout"
            >
              Organize…
            </button>
            <button disabled={scanning} onClick={scan} title="Scan root folders for ebook files">
              {scanning ? "Scanning…" : "Scan files"}
            </button>
          </span>
        </div>
        {scanNotice && <p className="muted">{scanNotice}</p>}
        {renamePlan && (
          <div className="rename-plan">
            <p>
              {renamePlan.length} file(s) would move to match the naming
              templates:
            </p>
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
              <button disabled={organizing} onClick={applyRenames}>
                Apply
              </button>
              <button className="toggle" onClick={() => setRenamePlan(null)}>
                Cancel
              </button>
            </div>
          </div>
        )}
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
            Found on disk but not matched to any library book. Pick the right
            book to import (the file is moved into place), or dismiss the
            record (disk is never touched). Books not in the library yet can
            be added from the Search tab first.
          </p>
          <ul className="rows">
            {unmatched.map((f) => (
              <UnmatchedRow
                key={f.id}
                file={f}
                books={allBooks}
                onDone={reload}
                onError={onError}
              />
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

function UnmatchedRow({
  file,
  books,
  onDone,
  onError,
}: {
  file: BookFile;
  books: Book[];
  onDone: () => void;
  onError: (message: string) => void;
}) {
  const [bookID, setBookID] = useState(0);
  const [busy, setBusy] = useState(false);

  const run = (action: () => Promise<unknown>) => {
    setBusy(true);
    action()
      .then(onDone)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  return (
    <li>
      <div className="row">
        <span className="file-path">{file.path}</span>
        <span className="row-actions">
          <select value={bookID} onChange={(e) => setBookID(Number(e.target.value))}>
            <option value={0}>Match to book…</option>
            {books.map((b) => (
              <option key={b.id} value={b.id}>
                {b.title}
              </option>
            ))}
          </select>
          <button
            disabled={busy || bookID === 0}
            onClick={() => run(() => api.matchFile(file.id, bookID))}
          >
            Import
          </button>
          <button
            className="toggle"
            disabled={busy}
            onClick={() => run(() => api.dismissFile(file.id))}
            title="Forget this file (nothing on disk is deleted)"
          >
            dismiss
          </button>
        </span>
      </div>
    </li>
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
  const [candidates, setCandidates] = useState<ReleaseCandidate[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [grabNotice, setGrabNotice] = useState("");

  const interactiveSearch = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .searchReleasesForBook(book.id)
      .then((r) => {
        setCandidates(r.releases);
        if (r.errors.length) setGrabNotice(`Some indexers failed: ${r.errors.join("; ")}`);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const autoGrab = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .autoSearchBook(book.id)
      .then((o) =>
        setGrabNotice(
          o.grabbed
            ? `✓ Grabbed "${o.release}" via ${o.client}`
            : `✗ ${o.message ?? "nothing grabbed"}`,
        ),
      )
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const grab = (c: ReleaseCandidate) => {
    api
      .grabRelease(c.title, c.downloadUrl, c.protocol, book.id)
      .then((r) => setGrabNotice(`✓ Sent "${c.title}" to ${r.client}`))
      .catch((err: unknown) =>
        setGrabNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      );
  };

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
          <div className="settings-actions">
            <button disabled={searching} onClick={autoGrab} title="Search indexers and grab the best release">
              {searching ? "Working…" : "Auto grab"}
            </button>
            <button disabled={searching} onClick={interactiveSearch} title="List all release candidates">
              Search releases
            </button>
            {grabNotice && (
              <span className={grabNotice.startsWith("✗") ? "notice bad" : "notice ok"}>
                {grabNotice}
              </span>
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
                      {c.approved && (
                        <button onClick={() => grab(c)}>Grab</button>
                      )}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
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
