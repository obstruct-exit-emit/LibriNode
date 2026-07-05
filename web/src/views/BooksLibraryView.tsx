import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Author,
  type Book,
  type BookFile,
  type Edition,
  type ReleaseCandidate,
  type RenameMove,
  type SearchAuthor,
  type SearchBook,
} from "../api";

// One format library's area (Ebooks or Audiobooks) — Plex-style: only books
// that are members of THIS library appear; ownership, monitoring, and search
// are all scoped to this format. The other format surfaces only inside a
// book's detail, as a cross-add.
export default function BooksLibraryView({
  library,
  onError,
}: {
  library: "ebook" | "audiobook";
  onError: (message: string) => void;
}) {
  const label = library === "ebook" ? "Ebooks" : "Audiobooks";
  const [authors, setAuthors] = useState<Author[]>([]);
  const [libraryBooks, setLibraryBooks] = useState<Book[]>([]);
  const [unmatched, setUnmatched] = useState<BookFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [openAuthor, setOpenAuthor] = useState<number | null>(null);
  const [busyHeader, setBusyHeader] = useState(false);
  const [notice, setNotice] = useState("");
  const [renamePlan, setRenamePlan] = useState<RenameMove[] | null>(null);
  const [showAdd, setShowAdd] = useState(false);

  const inLibrary = useCallback(
    (b: Book) => (library === "ebook" ? b.inEbookLibrary : b.inAudiobookLibrary),
    [library],
  );

  const reload = useCallback(() => {
    Promise.all([api.listAuthors(library), api.listBooks(), api.listUnmatchedFiles()])
      .then(([au, bk, un]) => {
        setAuthors(au);
        setLibraryBooks(bk.filter((b) => (library === "ebook" ? b.inEbookLibrary : b.inAudiobookLibrary)));
        setUnmatched(un.filter((f) => f.mediaType === library));
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError, library]);

  useEffect(reload, [reload]);

  const headerAction = (action: () => Promise<string>) => {
    setBusyHeader(true);
    setNotice("");
    action()
      .then((msg) => {
        setNotice(msg);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusyHeader(false));
  };

  const scan = () =>
    headerAction(async () => {
      const r = await api.scan();
      const errors = r.errors?.length ? `, ${r.errors.length} root(s) failed` : "";
      return r.roots === 0
        ? "No root folders to scan — add one under Settings."
        : `Scanned ${r.scanned} file(s): ${r.matched} matched, ${r.unmatched} unmatched, ${r.removed} removed${errors}`;
    });

  const searchWanted = () =>
    headerAction(async () => {
      const r = await api.searchWanted();
      const details = r.outcomes
        .filter((o) => !o.grabbed && o.message)
        .slice(0, 3)
        .map((o) => `${o.bookTitle}: ${o.message}`)
        .join("; ");
      return r.searched === 0
        ? "Nothing to search — every monitored item has its file (or a pending grab)."
        : `Searched ${r.searched} wanted item(s), grabbed ${r.grabbed}.${details ? " " + details : ""}`;
    });

  const previewRenames = () => {
    setBusyHeader(true);
    api
      .renamePreview()
      .then((r) => {
        setRenamePlan(r.moves);
        if (r.moves.length === 0) setNotice("All files already match the naming templates.");
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusyHeader(false));
  };

  const applyRenames = () => {
    setBusyHeader(true);
    api
      .renameApply()
      .then((r) => {
        setNotice(`Moved ${r.moves.length} file(s)${r.skips.length ? `, ${r.skips.length} skipped` : ""}.`);
        setRenamePlan(null);
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusyHeader(false));
  };

  if (loading) return <p className="muted">Loading {label.toLowerCase()}…</p>;

  return (
    <>
      <section className="card">
        <div className="card-head">
          <h2>{label} — Authors ({authors.length})</h2>
          <span className="row-actions">
            <button onClick={() => setShowAdd(!showAdd)}>{showAdd ? "Close" : "+ Add"}</button>
            <button disabled={busyHeader} onClick={searchWanted} title="Search indexers for everything wanted">
              Search wanted
            </button>
            {library === "ebook" && (
              <button disabled={busyHeader} onClick={previewRenames} title="Preview naming-template moves">
                Organize…
              </button>
            )}
            <button disabled={busyHeader} onClick={scan} title="Scan root folders">
              Scan files
            </button>
          </span>
        </div>
        {notice && <p className="muted">{notice}</p>}

        {showAdd && (
          <AddPanel library={library} onAdded={() => { reload(); }} onError={onError} />
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
              <button disabled={busyHeader} onClick={applyRenames}>Apply</button>
              <button className="toggle" onClick={() => setRenamePlan(null)}>Cancel</button>
            </div>
          </div>
        )}

        {authors.length === 0 ? (
          <p className="muted">
            This library is empty — use <strong>+ Add</strong> to search for an
            author or book, or scan a root folder with existing files.
          </p>
        ) : (
          <ul className="rows">
            {authors.map((a) => (
              <AuthorRow
                key={a.id}
                author={a}
                library={library}
                inLibrary={inLibrary}
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
            Found on disk but not matched to any book in this library. Files
            attach automatically when you add their book; or match by hand
            below. Dismiss forgets the record — disk is never touched.
          </p>
          <ul className="rows">
            {unmatched.map((f) => (
              <UnmatchedRow key={f.id} file={f} books={libraryBooks} onDone={reload} onError={onError} />
            ))}
          </ul>
        </section>
      )}
    </>
  );
}

// AddPanel searches the metadata provider and adds into THIS library.
function AddPanel({
  library,
  onAdded,
  onError,
}: {
  library: "ebook" | "audiobook";
  onAdded: () => void;
  onError: (message: string) => void;
}) {
  const [term, setTerm] = useState("");
  const [kind, setKind] = useState<"author" | "book">("author");
  const [authors, setAuthors] = useState<SearchAuthor[]>([]);
  const [books, setBooks] = useState<SearchBook[]>([]);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    if (!term.trim()) return;
    setBusy(true);
    setNotice("");
    const done = () => setBusy(false);
    if (kind === "author") {
      api.searchAuthors(term).then((r) => { setAuthors(r); setBooks([]); }, (err: unknown) =>
        onError(String(err instanceof Error ? err.message : err))).finally(done);
    } else {
      api.searchBooks(term).then((r) => { setBooks(r); setAuthors([]); }, (err: unknown) =>
        onError(String(err instanceof Error ? err.message : err))).finally(done);
    }
  };

  const add = (action: () => Promise<unknown>, title: string) => {
    setBusy(true);
    setNotice("");
    action()
      .then(() => {
        setNotice(`✓ Added "${title}" to this library`);
        onAdded();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  return (
    <div className="add-panel">
      <form onSubmit={search} className="search-form">
        <select value={kind} onChange={(e) => setKind(e.target.value as "author" | "book")}>
          <option value="author">Author</option>
          <option value="book">Book</option>
        </select>
        <input
          placeholder="Search the metadata provider…"
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          autoFocus
        />
        <button type="submit" disabled={busy || !term.trim()}>Search</button>
      </form>
      {notice && (
        <p className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</p>
      )}
      <ul className="rows">
        {authors.map((a) => (
          <li key={a.foreignAuthorId}>
            <div className="row">
              <span>
                {a.name}
                {a.bookCount ? <span className="muted"> · {a.bookCount} books</span> : null}
              </span>
              <button disabled={busy} onClick={() => add(() => api.addAuthor(a.foreignAuthorId, library), a.name)}>
                Add author
              </button>
            </div>
          </li>
        ))}
        {books.map((b) => (
          <li key={b.foreignBookId}>
            <div className="row">
              <span>
                {b.title}
                <span className="muted"> · {b.authorName}{b.releaseDate ? ` · ${b.releaseDate.slice(0, 4)}` : ""}</span>
              </span>
              <button disabled={busy} onClick={() => add(() => api.addBook(b.foreignBookId, library), b.title)}>
                Add book
              </button>
            </div>
          </li>
        ))}
      </ul>
    </div>
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
          {books.length > 0 && (
            <>
              <select value={bookID} onChange={(e) => setBookID(Number(e.target.value))}>
                <option value={0}>Match to book…</option>
                {books.map((b) => (
                  <option key={b.id} value={b.id}>{b.title}</option>
                ))}
              </select>
              <button disabled={busy || bookID === 0} onClick={() => run(() => api.matchFile(file.id, bookID))}>
                Import
              </button>
            </>
          )}
          <button className="toggle" disabled={busy} onClick={() => run(() => api.dismissFile(file.id))}>
            dismiss
          </button>
        </span>
      </div>
    </li>
  );
}

function AuthorRow({
  author,
  library,
  inLibrary,
  open,
  onToggleOpen,
  onChanged,
  onError,
}: {
  author: Author;
  library: "ebook" | "audiobook";
  inLibrary: (b: Book) => boolean;
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
      .then((all) => setBooks(all.filter(inLibrary)))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [open, author.id, onError, inLibrary]);

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
              if (confirm(`Remove ${author.name} and all their books from every library?`)) {
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
          {books.length === 0 && <li className="muted">No books in this library.</li>}
          {books.map((b) => (
            <BookRow key={b.id} book={b} library={library} onChanged={onChanged} onError={onError} />
          ))}
        </ul>
      )}
    </li>
  );
}

function BookRow({
  book,
  library,
  onChanged,
  onError,
}: {
  book: Book;
  library: "ebook" | "audiobook";
  onChanged: () => void;
  onError: (message: string) => void;
}) {
  const [detail, setDetail] = useState<Book | null>(null);
  const [open, setOpen] = useState(false);
  const [candidates, setCandidates] = useState<ReleaseCandidate[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [grabNotice, setGrabNotice] = useState("");

  const owned = library === "ebook" ? book.hasEbookFile : book.hasAudiobookFile;
  const monitored = library === "ebook" ? book.ebookMonitored : book.audiobookMonitored;
  const otherLibrary = library === "ebook" ? "audiobook" : "ebook";
  const inOther = library === "ebook" ? book.inAudiobookLibrary : book.inEbookLibrary;

  const loadDetail = () => {
    if (!open) {
      api
        .getBook(book.id)
        .then(setDetail)
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
    }
    setOpen(!open);
  };

  const setMembership = (lib: string, member: boolean, mon: boolean) => {
    api
      .setBookLibrary(book.id, lib, member, mon)
      .then(onChanged)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  };

  const autoGrab = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .autoSearchBook(book.id, library)
      .then((o) =>
        setGrabNotice(o.grabbed ? `✓ Grabbed "${o.release}" via ${o.client}` : `✗ ${o.message ?? "nothing grabbed"}`),
      )
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const interactiveSearch = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .searchReleasesForBook(book.id, library)
      .then((r) => {
        setCandidates(r.releases);
        if (r.errors.length) setGrabNotice(`Some indexers failed: ${r.errors.join("; ")}`);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const grab = (c: ReleaseCandidate) => {
    api
      .grabRelease(c.title, c.downloadUrl, c.protocol, book.id, library, c.guid)
      .then((r) => setGrabNotice(`✓ Sent "${c.title}" to ${r.client}`))
      .catch((err: unknown) => setGrabNotice(`✗ ${err instanceof Error ? err.message : String(err)}`));
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
          <span className={owned ? "owned yes" : "owned no"}>
            {owned ? "owned" : "wanted"}
          </span>
          {book.rating > 0 && <span className="muted">★ {book.rating.toFixed(1)}</span>}
          <button
            className={monitored ? "toggle on" : "toggle"}
            title="Whether this format is searched for automatically"
            onClick={() => setMembership(library, true, !monitored)}
          >
            {monitored ? "monitored" : "unmonitored"}
          </button>
        </span>
      </div>
      {open && detail && (
        <div className="book-detail">
          {detail.series && detail.series.length > 0 && (
            <p className="muted">{detail.series.map((s) => `${s.title} #${s.position}`).join(", ")}</p>
          )}
          <div className="settings-actions">
            <button disabled={searching} onClick={autoGrab} title="Search indexers and grab the best release">
              {searching ? "Working…" : "Auto grab"}
            </button>
            <button disabled={searching} onClick={interactiveSearch} title="List all release candidates">
              Search releases
            </button>
            {!inOther && (
              <button
                className="toggle"
                title={`This book isn't in the ${otherLibrary} library yet`}
                onClick={() => {
                  const mon = confirm(
                    `Add "${book.title}" to the ${otherLibrary} library.\n\nOK = monitor (search for it automatically) · Cancel = just add`,
                  );
                  setMembership(otherLibrary, true, mon);
                }}
              >
                + Add to {otherLibrary === "ebook" ? "Ebooks" : "Audiobooks"}
              </button>
            )}
            <button
              className="danger"
              title={`Remove from the ${library} library (files and the other library are untouched)`}
              onClick={() => {
                if (confirm(`Remove "${book.title}" from this library?`)) {
                  setMembership(library, false, false);
                }
              }}
            >
              remove from library
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
          {detail.files && detail.files.length > 0 && (
            <ul className="rows nested">
              {detail.files
                .filter((f) => f.mediaType === library)
                .map((f) => (
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
          <EditionsSummary editions={detail.editions ?? []} library={library} />
        </div>
      )}
    </li>
  );
}

// EditionsSummary shows only this library's format as compact metadata —
// editions are reference info, not controls (library membership, not
// edition monitoring, decides what gets acquired).
function EditionsSummary({
  editions,
  library,
}: {
  editions: Edition[];
  library: "ebook" | "audiobook";
}) {
  const relevant = editions.filter(
    (e) => e.format === library && (e.isbn13 || e.asin || e.publisher),
  );
  const others = editions.length - relevant.length;
  if (relevant.length === 0) {
    return editions.length > 0 ? (
      <p className="muted">No {library} editions with identifiers ({editions.length} other editions known).</p>
    ) : null;
  }

  const shown = relevant.slice(0, 5);
  return (
    <div className="editions-summary">
      <p className="muted">
        {relevant.length} {library} edition{relevant.length === 1 ? "" : "s"}
        {others > 0 && ` (+${others} other formats)`}:
      </p>
      <ul className="rows nested">
        {shown.map((e) => (
          <li key={e.id} className="muted">
            {[
              e.isbn13 && `ISBN ${e.isbn13}`,
              e.asin && `ASIN ${e.asin}`,
              e.publisher,
              e.language,
            ]
              .filter(Boolean)
              .join(" · ")}
          </li>
        ))}
        {relevant.length > shown.length && (
          <li className="muted">…and {relevant.length - shown.length} more</li>
        )}
      </ul>
    </div>
  );
}
