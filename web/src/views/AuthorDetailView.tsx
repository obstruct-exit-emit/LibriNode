import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Author,
  type Book,
  type Edition,
  type ReleaseCandidate,
} from "../api";
import RemovePanel from "../components/RemovePanel";

// Full-page author detail, *arr-style: header with portrait, description and
// author-level actions, then this library's books as clean rows. All
// ownership/monitor/search controls stay scoped to the current format.
export default function AuthorDetailView({
  id,
  library,
  onError,
  onBack,
}: {
  id: number;
  library: "ebook" | "audiobook";
  onError: (message: string) => void;
  onBack: () => void;
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
          <ul className="rows">
            {books.map((b) => (
              <BookRow key={b.id} book={b} library={library} onChanged={reload} onError={onError} />
            ))}
          </ul>
        )}
      </section>
    </>
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
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [grabNotice, setGrabNotice] = useState("");

  const owned = library === "ebook" ? book.hasEbookFile : book.hasAudiobookFile;
  const monitored = library === "ebook" ? book.ebookMonitored : book.audiobookMonitored;
  const otherLibrary = library === "ebook" ? "audiobook" : "ebook";
  const inOther = library === "ebook" ? book.inAudiobookLibrary : book.inEbookLibrary;
  const ownedOther = library === "ebook" ? book.hasAudiobookFile : book.hasEbookFile;

  const loadDetail = () => {
    if (!open) {
      api
        .getBook(book.id)
        .then(setDetail)
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
    }
    setOpen(!open);
  };

  const setMembership = (lib: string, member: boolean, mon: boolean, deleteFiles = false) => {
    api
      .setBookLibrary(book.id, lib, member, mon, deleteFiles)
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
            <button
              className="danger"
              title={`Remove from the ${library} library (the other library is untouched)`}
              onClick={() => setConfirmRemove(!confirmRemove)}
            >
              remove from library
            </button>
            {grabNotice && (
              <span className={grabNotice.startsWith("✗") ? "notice bad" : "notice ok"}>{grabNotice}</span>
            )}
            {inOther ? (
              <span
                className={`cross-format ${ownedOther ? "owned yes" : "owned no"}`}
                title={
                  ownedOther
                    ? `You own the ${otherLibrary} of this book`
                    : `Also in the ${otherLibrary === "ebook" ? "Ebooks" : "Audiobooks"} library, not owned yet`
                }
              >
                {otherLibrary === "audiobook" ? "🎧" : "📖"}{" "}
                {otherLibrary} {ownedOther ? "owned" : "in library"}
              </span>
            ) : (
              <button
                className="toggle cross-format"
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
          </div>
          {confirmRemove && (
            <RemovePanel
              message={`Remove "${book.title}" from the ${library === "ebook" ? "Ebooks" : "Audiobooks"} library? The other library is untouched.`}
              checkboxLabel={`Also delete its ${library} file(s) from disk`}
              busy={searching}
              onConfirm={(deleteFiles) => setMembership(library, false, false, deleteFiles)}
              onCancel={() => setConfirmRemove(false)}
            />
          )}
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
