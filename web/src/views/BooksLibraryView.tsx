import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Author,
  type Book,
  type BookFile,
  type RenameMove,
  type SearchAuthor,
  type SearchBook,
} from "../api";

// One format library's area (Ebooks or Audiobooks) — a *arr-style poster
// grid of authors; clicking one opens their full detail page. Only books
// that are members of THIS library count here.
export default function BooksLibraryView({
  library,
  onError,
  onOpenAuthor,
}: {
  library: "ebook" | "audiobook";
  onError: (message: string) => void;
  onOpenAuthor: (id: number) => void;
}) {
  const label = library === "ebook" ? "Ebooks" : "Audiobooks";
  const [authors, setAuthors] = useState<Author[]>([]);
  const [libraryBooks, setLibraryBooks] = useState<Book[]>([]);
  const [unmatched, setUnmatched] = useState<BookFile[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyHeader, setBusyHeader] = useState(false);
  const [notice, setNotice] = useState("");
  const [renamePlan, setRenamePlan] = useState<RenameMove[] | null>(null);
  const [showAdd, setShowAdd] = useState(false);

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
          <div className="poster-grid">
            {authors.map((a) => (
              <button key={a.id} className="poster-card" onClick={() => onOpenAuthor(a.id)}>
                {a.imageUrl ? (
                  <img className="poster" src={a.imageUrl} alt="" loading="lazy" />
                ) : (
                  <div className="poster fallback">{a.name.charAt(0)}</div>
                )}
                <span className="poster-title">{a.name}</span>
                <span className="poster-sub">
                  {a.ownedCount}/{a.bookCount ?? 0} owned
                </span>
              </button>
            ))}
          </div>
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
