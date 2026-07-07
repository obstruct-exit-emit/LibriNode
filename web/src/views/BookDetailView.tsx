import { useCallback, useEffect, useState } from "react";
import {
  api,
  type Author,
  type Book,
  type Edition,
  type ReleaseCandidate,
} from "../api";
import RemovePanel from "../components/RemovePanel";

// Full-page book detail, mirroring the author page: header with cover art,
// about text, and per-format status/actions, then releases, files, and
// edition info as their own cards. The back button returns to the author.
export default function BookDetailView({
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
  const [book, setBook] = useState<Book | null>(null);
  const [author, setAuthor] = useState<Author | null>(null);
  const [candidates, setCandidates] = useState<ReleaseCandidate[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [grabNotice, setGrabNotice] = useState("");

  const reload = useCallback(() => {
    api
      .getBook(id)
      .then((b) => {
        setBook(b);
        return api.getAuthor(b.authorId).then(setAuthor);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [id, onError]);

  useEffect(reload, [reload]);

  if (!book) return <p className="muted">Loading book…</p>;

  const owned = library === "ebook" ? book.hasEbookFile : book.hasAudiobookFile;
  const monitored = library === "ebook" ? book.ebookMonitored : book.audiobookMonitored;
  const otherLibrary = library === "ebook" ? "audiobook" : "ebook";
  const inOther = library === "ebook" ? book.inAudiobookLibrary : book.inEbookLibrary;
  const ownedOther = library === "ebook" ? book.hasAudiobookFile : book.hasEbookFile;
  const files = (book.files ?? []).filter((f) => f.mediaType === library);

  const setMembership = (lib: string, member: boolean, mon: boolean, deleteFiles = false) => {
    api
      .setBookLibrary(book.id, lib, member, mon, deleteFiles)
      .then(() => {
        // Leaving this library means the book no longer belongs on this
        // page — return to the author.
        if (lib === library && !member) {
          onBack();
        } else {
          reload();
        }
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  };

  const autoGrab = () => {
    setSearching(true);
    setGrabNotice("");
    api
      .autoSearchBook(book.id, library)
      .then((o) => {
        setGrabNotice(o.grabbed ? `✓ Grabbed "${o.release}" via ${o.client}` : `✗ ${o.message ?? "nothing grabbed"}`);
        reload();
      })
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
  const subtitle = [
    author?.name,
    book.series && book.series.length > 0
      ? book.series.map((s) => `${s.title} #${s.position}`).join(", ")
      : "",
    book.rating > 0 ? `★ ${book.rating.toFixed(1)}` : "",
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <>
      <button className="link back" onClick={onBack}>
        ← {author?.name ?? "Author"}
      </button>

      <section className="card detail-head">
        {book.coverUrl ? (
          <img className="detail-art" src={book.coverUrl} alt="" />
        ) : (
          <div className="detail-art fallback">{book.title.charAt(0)}</div>
        )}
        <div className="detail-info">
          <h2>
            {book.title}
            <span className="muted">{year}</span>
          </h2>
          <p className="muted">
            {subtitle}
            {subtitle && " · "}
            <span className={owned ? "owned yes" : "owned no"}>
              {owned ? "owned" : "wanted"}
            </span>
          </p>
          {book.description && <p className="detail-desc">{book.description}</p>}
          <div className="settings-actions">
            <button
              className={monitored ? "toggle on" : "toggle"}
              title="Whether this format is searched for automatically"
              onClick={() => setMembership(library, true, !monitored)}
            >
              {monitored ? "monitored" : "unmonitored"}
            </button>
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
              Remove from library
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
        </div>
      </section>

      {candidates && (
        <section className="card">
          <h2>Releases ({candidates.length})</h2>
          {candidates.length === 0 ? (
            <p className="muted">No releases found.</p>
          ) : (
            <ul className="rows">
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
        </section>
      )}

      {files.length > 0 && (
        <section className="card">
          <h2>Files ({files.length})</h2>
          <ul className="rows">
            {files.map((f) => (
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
        </section>
      )}

      <EditionsCard editions={book.editions ?? []} library={library} />
    </>
  );
}

// EditionsCard shows only this library's format as compact metadata —
// editions are reference info, not controls (library membership, not
// edition monitoring, decides what gets acquired).
function EditionsCard({
  editions,
  library,
}: {
  editions: Edition[];
  library: "ebook" | "audiobook";
}) {
  const relevant = editions.filter(
    (e) => e.format === library && (e.isbn13 || e.asin || e.publisher),
  );
  if (relevant.length === 0) return null;

  const others = editions.length - relevant.length;
  const shown = relevant.slice(0, 5);
  return (
    <section className="card">
      <h2>
        Editions ({relevant.length}
        {others > 0 ? ` ${library}, +${others} other formats` : ""})
      </h2>
      <ul className="rows">
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
    </section>
  );
}
