import { useCallback, useEffect, useState } from "react";
import {
  api,
  proxiedImage,
  type Author,
  type Book,
  type RenameMove,
  type SearchAuthor,
  type SearchBook,
  type UnmatchedOption,
} from "../api";
import WantedCard from "../components/WantedCard";

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
  const [unmatched, setUnmatched] = useState<UnmatchedOption[]>([]);
  const [importingAll, setImportingAll] = useState(false);
  const [importNotice, setImportNotice] = useState("");
  const [loading, setLoading] = useState(true);
  const [busyHeader, setBusyHeader] = useState(false);
  const [notice, setNotice] = useState("");
  const [renamePlan, setRenamePlan] = useState<RenameMove[] | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  // Large libraries: filter client-side and render the grid incrementally.
  const [filter, setFilter] = useState("");
  const [visible, setVisible] = useState(60);

  const reload = useCallback(() => {
    Promise.all([api.listAuthors(library), api.listBooks(), api.unmatchedOptions(library)])
      .then(([au, bk, un]) => {
        setAuthors(au);
        setLibraryBooks(bk.filter((b) => (library === "ebook" ? b.inEbookLibrary : b.inAudiobookLibrary)));
        setUnmatched(un);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setLoading(false));
  }, [onError, library]);

  // Existing-file import, the bulk path: adopt every confident match at once.
  const importAllMatched = () => {
    setImportingAll(true);
    setImportNotice("");
    api
      .importMatched(library)
      .then((r) => {
        setImportNotice(
          `✓ Imported ${r.imported} — ${r.needsReview} still need${r.needsReview === 1 ? "s" : ""} review`,
        );
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setImportingAll(false));
  };

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
            <button disabled={busyHeader} onClick={previewRenames} title="Preview naming-template moves">
              Organize…
            </button>
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
          (() => {
            const filtered = authors.filter((a) =>
              a.name.toLowerCase().includes(filter.toLowerCase()),
            );
            return (
              <>
                {authors.length > 10 && (
                  <input
                    className="grid-filter"
                    placeholder="Filter authors…"
                    value={filter}
                    onChange={(e) => {
                      setFilter(e.target.value);
                      setVisible(60);
                    }}
                  />
                )}
                <div className="poster-grid">
                  {filtered.slice(0, visible).map((a) => (
                    <button key={a.id} className="poster-card" onClick={() => onOpenAuthor(a.id)}>
                      {a.imageUrl ? (
                        <img className="poster" src={proxiedImage(a.imageUrl)} alt="" loading="lazy" />
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
                {filtered.length === 0 && <p className="muted">No authors match the filter.</p>}
                {filtered.length > visible && (
                  <button className="toggle show-more" onClick={() => setVisible(visible + 120)}>
                    Show more ({filtered.length - visible} more)
                  </button>
                )}
              </>
            );
          })()
        )}
      </section>

      <WantedCard key={`wanted-${library}`} library={library} onError={onError} />

      {unmatched.length > 0 && (
        <section className="card">
          <div className="card-head">
            <h2>Unmatched files ({unmatched.length})</h2>
            {unmatched.some((o) => o.confident) && (
              <button
                disabled={importingAll}
                onClick={importAllMatched}
                title="Adopt every file with a confident match — the book is imported, added to this library, and monitored"
              >
                {importingAll
                  ? "Importing…"
                  : `Import all matched (${unmatched.filter((o) => o.confident).length})`}
              </button>
            )}
          </div>
          <p className="muted">
            Found on disk but not matched to any book in this library. Rows
            with a confident match import in one click — the book joins the
            library, monitored. For the rest, pick from the author's books.
            Dismiss forgets the record — disk is never touched.
          </p>
          {importNotice && (
            <p className={importNotice.startsWith("✗") ? "notice bad" : "notice ok"}>{importNotice}</p>
          )}
          <ul className="rows">
            {unmatched.map((o) => (
              <UnmatchedRow key={o.file.id} option={o} books={libraryBooks} onDone={reload} onError={onError} />
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
  option,
  books,
  onDone,
  onError,
}: {
  option: UnmatchedOption;
  books: Book[];
  onDone: () => void;
  onError: (message: string) => void;
}) {
  const { file, candidates, duplicate } = option;
  const suggested = candidates.find((c) => c.id === option.suggested);
  const [bookID, setBookID] = useState(option.suggested ?? 0);
  const [busy, setBusy] = useState(false);
  const [findingAuthor, setFindingAuthor] = useState(false);
  const [authorResults, setAuthorResults] = useState<SearchAuthor[] | null>(null);

  const run = (action: () => Promise<unknown>) => {
    setBusy(true);
    action()
      .then(onDone)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  // The file's author isn't in the library: search the metadata provider so
  // one click adds them — after which this file (and any siblings) gets real
  // candidates and, usually, a confident suggestion.
  const findAuthor = () => {
    if (authorResults) {
      setAuthorResults(null); // toggle closed
      return;
    }
    setFindingAuthor(true);
    api
      .searchAuthors(option.authorName ?? "")
      .then((results) => setAuthorResults(results.slice(0, 5)))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setFindingAuthor(false));
  };

  const fmtSize = (bytes: number) =>
    bytes >= 1 << 20 ? `${(bytes / (1 << 20)).toFixed(1)} MiB` : `${(bytes / 1024).toFixed(0)} KiB`;

  // Duplicate of an owned book: show both files and resolve — replace the
  // library's copy with this one, or delete this one from disk.
  if (duplicate) {
    return (
      <li>
        <div className="row">
          <span className="file-path">
            ⚠️ Duplicate of <strong>{duplicate.title}</strong>
            {duplicate.year && ` (${duplicate.year})`}
            <span className="pill match-confidence" title="Match confidence">
              {duplicate.confidence}%
            </span>
          </span>
        </div>
        <ul className="rows nested">
          <li>
            <div className="row">
              <span className="file-path muted">in library: 📄 {duplicate.file.path}</span>
              <span className="muted">
                {duplicate.file.format} · {fmtSize(duplicate.file.size)}
              </span>
            </div>
          </li>
          <li>
            <div className="row">
              <span className="file-path">this file: 📄 {file.path}</span>
              <span className="row-actions">
                <span className="muted">
                  {file.format} · {fmtSize(file.size)}
                </span>
                <button
                  disabled={busy}
                  title="Use this file instead — the library's current copy is deleted from disk"
                  onClick={() => {
                    if (confirm(`Replace the library's copy of "${duplicate.title}" with this file?\n\nThe current file is deleted from disk.`)) {
                      run(() => api.replaceFile(file.id, duplicate.bookId));
                    }
                  }}
                >
                  {busy ? "Working…" : "Replace"}
                </button>
                <button
                  className="danger"
                  disabled={busy}
                  title="Keep the library's copy — delete this file from disk"
                  onClick={() => {
                    if (confirm("Delete this file from disk? The library's copy is kept.")) {
                      run(() => api.dismissFile(file.id, true));
                    }
                  }}
                >
                  Delete
                </button>
                <button
                  className="toggle"
                  disabled={busy}
                  title="Forget this file without touching disk (the next scan re-finds it)"
                  onClick={() => run(() => api.dismissFile(file.id))}
                >
                  dismiss
                </button>
              </span>
            </div>
          </li>
        </ul>
      </li>
    );
  }

  return (
    <li>
      <div className="row">
        <span className="file-path">
          {file.path}
          {option.confident && suggested && (
            <span className="notice ok">
              {" "}→ {suggested.title}
              {suggested.year && ` (${suggested.year})`}
              <span
                className="pill match-confidence"
                title="How sure the match is — an exact title is 100%; longer, more distinctive matches score higher"
              >
                {option.confidence}%
              </span>
            </span>
          )}
        </span>
        <span className="row-actions">
          {option.confident && suggested ? (
            <button
              disabled={busy}
              title={`Import as "${suggested.title}" — the book joins this library, monitored`}
              onClick={() => run(() => api.matchFile(file.id, suggested.id))}
            >
              {busy ? "Importing…" : "Import"}
            </button>
          ) : candidates.length > 0 ? (
            <>
              <select value={bookID} onChange={(e) => setBookID(Number(e.target.value))}>
                <option value={0}>
                  {option.authorName ? `${option.authorName}'s books…` : "Choose book…"}
                </option>
                {candidates.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.title}
                    {c.year ? ` (${c.year})` : ""}
                  </option>
                ))}
              </select>
              <button disabled={busy || bookID === 0} onClick={() => run(() => api.matchFile(file.id, bookID))}>
                Import
              </button>
            </>
          ) : (
            <>
              {option.authorName && !option.authorId && (
                <button
                  className="toggle"
                  disabled={busy || findingAuthor}
                  title={`"${option.authorName}" isn't in the library — search the metadata provider and add them`}
                  onClick={findAuthor}
                >
                  {findingAuthor ? "Searching…" : `+ Add ${option.authorName}`}
                </button>
              )}
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
            </>
          )}
          <button className="toggle" disabled={busy} onClick={() => run(() => api.dismissFile(file.id))}>
            dismiss
          </button>
        </span>
      </div>
      {authorResults && (
        <ul className="rows nested">
          {authorResults.length === 0 && (
            <li className="muted fb-empty">No author named “{option.authorName}” found on the provider.</li>
          )}
          {authorResults.map((a) => (
            <li key={a.foreignAuthorId}>
              <div className="row">
                <span>
                  {a.name}
                  {(a.bookCount ?? 0) > 0 && <span className="muted"> · {a.bookCount} books</span>}
                </span>
                <button
                  disabled={busy}
                  title={`Add ${a.name} to this library — their bibliography becomes matchable`}
                  onClick={() => run(() => api.addAuthor(a.foreignAuthorId, file.mediaType))}
                >
                  {busy ? "Adding…" : "Add author"}
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}
