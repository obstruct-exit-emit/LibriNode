import { useEffect, useMemo, useState } from "react";
import { api, type ReleaseCandidate } from "../api";
import { formatBytes } from "../format";

// ReleaseBrowser is the interactive search: every release candidate for a
// book/volume, scored and organized — approved first, sortable, filterable by
// protocol, rejected ones dimmed with their reasons and still force-grabbable.
// Shared by the book page and series volume rows so search behaves (and looks)
// the same in every library.

type SortKey = "score" | "size" | "seeders" | "age";
type ProtoFilter = "all" | "usenet" | "torrent";

const protoIcon = (p: string) => (p === "usenet" ? "📡" : "🧲");

function fmtAge(publishDate?: string): string {
  if (!publishDate) return "";
  const ms = Date.now() - new Date(publishDate).getTime();
  if (isNaN(ms) || ms < 0) return "";
  const days = ms / 86_400_000;
  if (days < 1) return "today";
  if (days < 30) return `${Math.floor(days)}d`;
  if (days < 365) return `${Math.floor(days / 30)}mo`;
  return `${Math.floor(days / 365)}y`;
}

function ageValue(publishDate?: string): number {
  const t = publishDate ? new Date(publishDate).getTime() : 0;
  return isNaN(t) ? 0 : t;
}

// leechers derives from the Torznab convention peers = seeders + leechers;
// indexers that report leechers directly (peers < seeders) pass through.
function leechers(c: ReleaseCandidate): number | null {
  if (c.protocol !== "torrent" || c.peers < 0) return null;
  return c.seeders >= 0 && c.peers >= c.seeders ? c.peers - c.seeders : c.peers;
}

export default function ReleaseBrowser({
  bookId,
  mediaType,
  onGrabbed,
  onClose,
}: {
  bookId: number;
  mediaType: string;
  // Called after a release was sent to a client — refresh queue/badges.
  onGrabbed?: () => void;
  onClose?: () => void;
}) {
  const [releases, setReleases] = useState<ReleaseCandidate[] | null>(null);
  const [errors, setErrors] = useState<string[]>([]);
  const [loadError, setLoadError] = useState("");
  const [showRejected, setShowRejected] = useState(false);
  const [proto, setProto] = useState<ProtoFilter>("all");
  const [sort, setSort] = useState<SortKey>("score");
  // Per-release grab state, keyed by guid+indexer: "sending", "✓ …", "✗ …".
  const [grabState, setGrabState] = useState<Record<string, string>>({});

  useEffect(() => {
    let stopped = false;
    setReleases(null);
    setLoadError("");
    api
      .searchReleasesForBook(bookId, mediaType)
      .then((r) => {
        if (stopped) return;
        setReleases(r.releases);
        setErrors(r.errors);
      })
      .catch((err: unknown) => {
        if (!stopped) setLoadError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      stopped = true;
    };
  }, [bookId, mediaType]);

  const approved = useMemo(() => (releases ?? []).filter((c) => c.approved), [releases]);

  const shown = useMemo(() => {
    let list = showRejected ? (releases ?? []) : approved;
    if (proto !== "all") list = list.filter((c) => c.protocol === proto);
    const sorted = [...list];
    switch (sort) {
      case "size":
        sorted.sort((a, b) => b.size - a.size);
        break;
      case "seeders":
        sorted.sort((a, b) => b.seeders - a.seeders);
        break;
      case "age":
        sorted.sort((a, b) => ageValue(b.publishDate) - ageValue(a.publishDate));
        break;
      default:
        // Best first: approved above rejected, then by score.
        sorted.sort((a, b) =>
          a.approved !== b.approved ? (a.approved ? -1 : 1) : b.score - a.score,
        );
    }
    return sorted;
  }, [releases, approved, showRejected, proto, sort]);

  const grab = (c: ReleaseCandidate) => {
    const key = c.guid + c.indexer;
    setGrabState((s) => ({ ...s, [key]: "sending" }));
    api
      .grabRelease(c.title, c.downloadUrl, c.protocol, bookId, mediaType, c.guid)
      .then((r) => {
        setGrabState((s) => ({ ...s, [key]: `✓ sent to ${r.client}` }));
        onGrabbed?.();
      })
      .catch((err: unknown) =>
        setGrabState((s) => ({
          ...s,
          [key]: `✗ ${err instanceof Error ? err.message : String(err)}`,
        })),
      );
  };

  if (loadError) {
    return (
      <div className="release-browser">
        <p className="notice bad">Search failed: {loadError}</p>
      </div>
    );
  }
  if (releases === null) {
    return (
      <div className="release-browser">
        <p className="muted rb-searching">Searching your indexers…</p>
      </div>
    );
  }

  const torrents = releases.some((c) => c.protocol === "torrent");
  const usenet = releases.some((c) => c.protocol === "usenet");

  return (
    <div className="release-browser">
      <div className="rb-head">
        <span className="rb-summary">
          <strong>{approved.length}</strong> approved · {releases.length} found
        </span>
        <span className="rb-controls">
          <button
            className={!showRejected ? "toggle on" : "toggle"}
            onClick={() => setShowRejected(false)}
            title="Only releases that pass your quality profile"
          >
            approved
          </button>
          <button
            className={showRejected ? "toggle on" : "toggle"}
            onClick={() => setShowRejected(true)}
            title="Everything found, with rejection reasons"
          >
            all
          </button>
          {usenet && torrents && (
            <>
              <span className="rb-sep" />
              {(["all", "usenet", "torrent"] as const).map((p) => (
                <button
                  key={p}
                  className={proto === p ? "toggle on" : "toggle"}
                  onClick={() => setProto(p)}
                >
                  {p === "all" ? "both" : `${protoIcon(p)} ${p}`}
                </button>
              ))}
            </>
          )}
          <select value={sort} onChange={(e) => setSort(e.target.value as SortKey)} title="Sort by">
            <option value="score">Best score</option>
            <option value="size">Largest</option>
            {torrents && <option value="seeders">Most seeders</option>}
            <option value="age">Newest</option>
          </select>
          {onClose && (
            <button className="toggle" onClick={onClose} title="Close the release list">
              ✕
            </button>
          )}
        </span>
      </div>

      {errors.length > 0 && (
        <p className="notice bad rb-errors">Some indexers failed: {errors.join("; ")}</p>
      )}

      {shown.length === 0 ? (
        <p className="muted">
          {releases.length === 0
            ? "No releases found on your indexers."
            : showRejected
              ? "Nothing matches this filter."
              : "Nothing approved — switch to “all” to see what was rejected and why."}
        </p>
      ) : (
        <ul className="rows rb-list">
          {shown.map((c) => {
            const key = c.guid + c.indexer;
            const state = grabState[key];
            return (
              <li key={key} className={c.approved ? undefined : "rb-rejected"}>
                <div className="row">
                  <span className="rb-title" title={c.title}>
                    <span title={c.protocol}>{protoIcon(c.protocol)}</span> {c.title}
                  </span>
                  <span className="row-actions">
                    <span
                      className={`pill rb-score${sort === "score" ? " on" : ""}`}
                      title="Release score — higher grabs first"
                    >
                      {c.score}
                    </span>
                    {state ? (
                      <span className={state.startsWith("✗") ? "notice bad" : "notice ok"}>
                        {state === "sending" ? "Sending…" : state}
                      </span>
                    ) : c.approved ? (
                      <button onClick={() => grab(c)}>Grab</button>
                    ) : (
                      <button
                        className="toggle"
                        onClick={() => grab(c)}
                        title="Rejected by your quality profile — grab it anyway"
                      >
                        grab anyway
                      </button>
                    )}
                  </span>
                </div>
                <div className="rb-meta muted">
                  {c.indexer}
                  <span className={`metric${sort === "size" ? " on" : ""}`} title="Size">
                    📦 {formatBytes(c.size) || "—"}
                  </span>
                  <span
                    className={`metric${sort === "seeders" ? " on" : ""}`}
                    title={c.protocol === "torrent" ? "Seeders" : "Seeders (usenet has none)"}
                  >
                    ↑ {c.protocol === "torrent" && c.seeders >= 0 ? c.seeders : "—"}
                  </span>
                  <span
                    className="metric"
                    title={c.protocol === "torrent" ? "Leechers" : "Leechers (usenet has none)"}
                  >
                    ↓ {leechers(c) ?? "—"}
                  </span>
                  <span className={`metric${sort === "age" ? " on" : ""}`} title="Age (published)">
                    🕓 {fmtAge(c.publishDate) || "—"}
                  </span>
                  {(c.parsed.formats ?? []).map((f) => (
                    <span key={f} className="pill rb-format">
                      {f}
                    </span>
                  ))}
                  {c.parsed.retail && <span className="pill rb-retail">retail</span>}
                  {!c.approved && c.rejections && (
                    <span className="rb-why"> — {c.rejections.join(", ")}</span>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
