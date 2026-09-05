import { useCallback, useEffect, useRef, useState } from "react";
import { api, proxiedImage, type Author, type Book } from "../api";
import RemovePanel from "../components/RemovePanel";
import ReleaseBrowser from "../components/ReleaseBrowser";
import WriteTagsDialog from "../components/WriteTagsDialog";
import { DetailSkeleton } from "../components/Skeleton";
import { downloadPct, useQueue } from "../useQueue";
import { formatBytes } from "../format";
import { useUi } from "../ui";

// "970" -> "16h 10m"
function formatRuntime(min: number): string {
  const h = Math.floor(min / 60);
  const m = min % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

// Wikipedia's "Go" search: redirects straight to the article when the title
// matches (e.g. a well-known narrator), otherwise lands on search results — so
// the link is always useful and never a dead page.
function wikiUrl(name: string): string {
  return `https://en.wikipedia.org/wiki/Special:Search?search=${encodeURIComponent(
    name.trim(),
  )}&go=Go`;
}

function splitNarrators(narrator: string): string[] {
  return narrator
    .split(",")
    .map((n) => n.trim())
    .filter(Boolean);
}

// NarratorChip shows a single narrator as a Wikipedia link chip, or — for a
// full cast — one "N narrators" button that opens the modal, so a dozen names
// never crowd the page.
function NarratorChip({ names, onShowAll }: { names: string[]; onShowAll: () => void }) {
  return (
    <div className="narrator-chips">
      {names.length === 1 ? (
        <a
          className="toggle"
          href={wikiUrl(names[0])}
          target="_blank"
          rel="noreferrer"
          title={`Look up ${names[0]} on Wikipedia`}
        >
          {names[0]} ↗
        </a>
      ) : (
        <button className="toggle" onClick={onShowAll} title="Show all narrators">
          {names.length} narrators
        </button>
      )}
    </div>
  );
}

// NarratorsModal lists every narrator, each with a Wikipedia link — mirrors
// CantiNode's TrackCreditsModal (the "Featuring" popup): the app's modal shell
// and a bordered credits-list row per name.
function NarratorsModal({ names, onClose }: { names: string[]; onClose: () => void }) {
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" role="dialog" aria-modal="true" onClick={(e) => e.stopPropagation()}>
        <h3>Narrators</h3>
        <ul className="credits-list">
          {names.map((name, i) => (
            <li key={i}>
              <span>{name}</span>
              <a
                className="toggle"
                href={wikiUrl(name)}
                target="_blank"
                rel="noreferrer"
                title={`Search Wikipedia for ${name}`}
              >
                Wikipedia ↗
              </a>
            </li>
          ))}
        </ul>
        <div className="settings-actions">
          <button onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}

// Full-page book detail, mirroring the author page: header with cover art,
// about text, and per-format status/actions, then releases and files as their
// own cards. The back button returns to the author. (A file row shows the
// matched narrator/runtime for an owned audiobook.)
export default function BookDetailView({
  id,
  library,
  onError,
  onBack,
  onSwitchLibrary,
}: {
  id: number;
  library: "ebook" | "audiobook";
  onError: (message: string) => void;
  onBack: () => void;
  // Navigate to this same book in the other format library (the cross-format
  // badge links there when the book is already in it).
  onSwitchLibrary?: (library: "ebook" | "audiobook") => void;
}) {
  const { confirmDlg } = useUi();
  const [book, setBook] = useState<Book | null>(null);
  const [author, setAuthor] = useState<Author | null>(null);
  const [showReleases, setShowReleases] = useState(false);
  const [searching, setSearching] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const [addingOther, setAddingOther] = useState(false);
  const [grabNotice, setGrabNotice] = useState("");
  const [fileBusy, setFileBusy] = useState(false);
  const [fileNotice, setFileNotice] = useState("");
  const [showNarrators, setShowNarrators] = useState(false);
  const [showWriteTags, setShowWriteTags] = useState(false);

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

  // Live download status for this book+format (shared, server-cached queue
  // poll). When an active download disappears — imported, failed, removed —
  // refresh the book so the badge flips to owned or back to wanted.
  const { refresh, activeFor } = useQueue();
  const dl = activeFor(id, library);
  const hadDl = useRef(false);
  useEffect(() => {
    if (hadDl.current && !dl) reload();
    hadDl.current = dl !== null;
  }, [dl, reload]);

  if (!book) return <DetailSkeleton />;

  const owned = library === "ebook" ? book.hasEbookFile : book.hasAudiobookFile;
  const monitored = library === "ebook" ? book.ebookMonitored : book.audiobookMonitored;
  const otherLibrary = library === "ebook" ? "audiobook" : "ebook";
  const inOther = library === "ebook" ? book.inAudiobookLibrary : book.inEbookLibrary;
  const ownedOther = library === "ebook" ? book.hasAudiobookFile : book.hasEbookFile;
  const files = (book.files ?? []).filter((f) => f.mediaType === library);

  // Header facts, drawn from the owned file(s) of this format.
  const totalSize = files.reduce((sum, f) => sum + f.size, 0);
  const formatList = [...new Set(files.map((f) => f.format).filter(Boolean))]
    .map((s) => s.toUpperCase())
    .join(", ");
  const narrator = files.find((f) => f.narrator)?.narrator ?? "";
  const narratorNames = splitNarrators(narrator);
  const runtimeMinutes = files.find((f) => f.runtimeMinutes)?.runtimeMinutes ?? 0;
  const trackCount = files.find((f) => f.tracks?.length)?.tracks?.length ?? 0;
  const basePath = files[0]?.path ?? "";

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
        if (o.grabbed) {
          setGrabNotice(`✓ Grabbed "${o.release}" → ${o.client}`);
          refresh(); // show the downloading badge right away
        } else {
          setGrabNotice(`✗ ${o.message ?? "nothing grabbed"} — Search releases shows why`);
        }
        reload();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setSearching(false));
  };

  const writeTags = (clear: boolean) => {
    setShowWriteTags(false);
    setGrabNotice("");
    api
      .writeBookTags(book.id, clear)
      .then((r) => {
        setGrabNotice(
          r.errors.length > 0
            ? `✗ Wrote ${r.written} file(s); ${r.errors.length} failed: ${r.errors[0]}`
            : `✓ Tags written to ${r.written} file(s)`,
        );
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
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
          <img className="detail-art" src={proxiedImage(book.coverUrl)} alt={`Cover of ${book.title}`} />
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
            {!owned && dl ? (
              <span className="owned dl" title={`${dl.status} on ${dl.client}`}>
                ⬇ downloading {downloadPct(dl)}
              </span>
            ) : (
              <span className={owned ? "owned yes" : "owned no"}>
                {owned ? "owned" : "wanted"}
              </span>
            )}
          </p>
          {files.length > 0 && (
            <>
              <div className="detail-stats">
                {narratorNames.length > 0 && (
                  <div className="detail-stat wide">
                    <span className="detail-stat-label">Narrator</span>
                    <NarratorChip names={narratorNames} onShowAll={() => setShowNarrators(true)} />
                  </div>
                )}
                {runtimeMinutes > 0 && (
                  <div className="detail-stat">
                    <span className="detail-stat-label">Runtime</span>
                    <span className="detail-stat-value">{formatRuntime(runtimeMinutes)}</span>
                  </div>
                )}
                {formatList && (
                  <div className="detail-stat">
                    <span className="detail-stat-label">Format</span>
                    <span className="detail-stat-value">{formatList}</span>
                  </div>
                )}
                {trackCount > 1 && (
                  <div className="detail-stat">
                    <span className="detail-stat-label">Tracks</span>
                    <span className="detail-stat-value">{trackCount}</span>
                  </div>
                )}
                {totalSize > 0 && (
                  <div className="detail-stat">
                    <span className="detail-stat-label">Size</span>
                    <span className="detail-stat-value">{formatBytes(totalSize)}</span>
                  </div>
                )}
              </div>
              {basePath && (
                <div className="detail-stats">
                  <div className="detail-stat wide">
                    <span className="detail-stat-label">Path</span>
                    <span className="detail-stat-value" title={basePath}>{basePath}</span>
                  </div>
                </div>
              )}
            </>
          )}
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
              {searching ? "Searching…" : "Auto grab"}
            </button>
            <button
              className={showReleases ? "toggle on" : ""}
              onClick={() => setShowReleases(!showReleases)}
              title="Browse every release candidate — sort, filter, pick one yourself"
            >
              {showReleases ? "Hide releases" : "Search releases"}
            </button>
            {library === "audiobook" && files.length > 0 && (
              <button
                onClick={() => setShowWriteTags(true)}
                title="Embed LibriNode's metadata into the audiobook file(s) so other players read it"
              >
                Write tags…
              </button>
            )}
            {grabNotice && (
              <span className={grabNotice.startsWith("✗") ? "notice bad" : "notice ok"}>{grabNotice}</span>
            )}
            {inOther ? (
              <button
                className={`cross-format owned-link ${ownedOther ? "owned yes" : "owned no"}`}
                title={`Open this book in the ${otherLibrary === "ebook" ? "Ebooks" : "Audiobooks"} library${ownedOther ? " (owned there)" : " (not owned there yet)"}`}
                onClick={() => onSwitchLibrary?.(otherLibrary)}
              >
                {otherLibrary === "audiobook" ? "🎧" : "📖"}{" "}
                {otherLibrary} {ownedOther ? "owned" : "in library"} →
              </button>
            ) : addingOther ? (
              // A real three-way choice — monitor, just add, or back out —
              // instead of a yes/no dialog pretending to be one.
              <span className="row-actions cross-format">
                <button
                  onClick={() => {
                    setAddingOther(false);
                    setMembership(otherLibrary, true, true);
                  }}
                  title="Add and search for it automatically"
                >
                  Add + monitor
                </button>
                <button
                  className="toggle"
                  onClick={() => {
                    setAddingOther(false);
                    setMembership(otherLibrary, true, false);
                  }}
                  title="Add without monitoring — track it, don't search for it"
                >
                  Just add
                </button>
                <button className="toggle" onClick={() => setAddingOther(false)} title="Cancel">
                  ✕
                </button>
              </span>
            ) : (
              <button
                className="toggle cross-format"
                title={`This book isn't in the ${otherLibrary} library yet`}
                onClick={() => setAddingOther(true)}
              >
                + Add to {otherLibrary === "ebook" ? "Ebooks" : "Audiobooks"}
              </button>
            )}
          </div>
          <details className="disclosure">
            <summary>Advanced</summary>
            <div className="disclosure-body">
              <div className="settings-actions">
                <button
                  className="danger"
                  title={`Remove from the ${library} library (the other library is untouched)`}
                  onClick={() => setConfirmRemove(!confirmRemove)}
                >
                  Remove from library
                </button>
              </div>
            </div>
          </details>
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

      {showReleases && (
        <section className="card">
          <h2>Releases</h2>
          <ReleaseBrowser
            bookId={book.id}
            mediaType={library}
            onGrabbed={refresh}
            onClose={() => setShowReleases(false)}
          />
        </section>
      )}

      {files.length > 0 && (
        <section className="card">
          <h2>Files ({files.length})</h2>
          {fileNotice && <p className="notice ok">{fileNotice}</p>}
          <ul className="rows">
            {files.map((f) => (
              <li key={f.id}>
                <div className="row">
                  <span className="file-path">
                    {f.tracks?.length ? "📁" : "📄"} {f.path}
                  </span>
                  <span className="row-actions">
                    <span className="muted">
                      {f.format} · {formatBytes(f.size)}
                    </span>
                    <button
                      className="toggle"
                      disabled={fileBusy}
                      title="Move this book's files to match the naming templates"
                      onClick={async () => {
                        setFileBusy(true);
                        setFileNotice("");
                        try {
                          const preview = await api.renamePreview(undefined, undefined, undefined, book.id);
                          if (preview.moves.length === 0) {
                            setFileNotice("✓ Already organized — files match the naming templates.");
                            return;
                          }
                          const ok = await confirmDlg({
                            title: "Organize files",
                            message:
                              `Move ${preview.moves.length} file(s) to match the naming templates?\n\n` +
                              preview.moves.map((m) => `${m.from}\n  → ${m.to}`).join("\n"),
                            confirmLabel: "Organize",
                          });
                          if (!ok) return;
                          const applied = await api.renameApply(undefined, undefined, undefined, book.id);
                          setFileNotice(`✓ Moved ${applied.moves.length} file(s).`);
                          reload();
                        } catch (err) {
                          onError(String(err instanceof Error ? err.message : err));
                        } finally {
                          setFileBusy(false);
                        }
                      }}
                    >
                      organize
                    </button>
                    <button
                      className="danger"
                      disabled={fileBusy}
                      title="Delete this file from disk and forget it"
                      onClick={async () => {
                        const ok = await confirmDlg({
                          title: "Delete file",
                          message: `Delete this file from disk?\n\n${f.path}\n\nThe book loses this copy; without it the book counts as wanted again.`,
                          confirmLabel: "Delete file",
                          danger: true,
                        });
                        if (!ok) return;
                        setFileBusy(true);
                        setFileNotice("");
                        try {
                          await api.dismissFile(f.id, true);
                          setFileNotice("✓ File deleted.");
                          reload();
                        } catch (err) {
                          onError(String(err instanceof Error ? err.message : err));
                        } finally {
                          setFileBusy(false);
                        }
                      }}
                    >
                      delete
                    </button>
                  </span>
                </div>
                {(f.tracks?.length ?? 0) > 0 && (
                  <details className="track-list">
                    <summary className="muted">
                      {f.tracks!.length} file{f.tracks!.length === 1 ? "" : "s"}
                    </summary>
                    <ul className="rows nested">
                      {f.tracks!.map((t) => (
                        <li key={t.name}>
                          <div className="row">
                            <span className="file-path">🎵 {t.name}</span>
                            <span className="muted">{formatBytes(t.size)}</span>
                          </div>
                        </li>
                      ))}
                    </ul>
                  </details>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
      {showNarrators && (
        <NarratorsModal names={narratorNames} onClose={() => setShowNarrators(false)} />
      )}
      {showWriteTags && (
        <WriteTagsDialog onConfirm={writeTags} onClose={() => setShowWriteTags(false)} />
      )}
    </>
  );
}
