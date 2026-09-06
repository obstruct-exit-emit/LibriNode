import { useEffect, useState } from "react";
import { api, type BookFileTags } from "../api";

// BookFileTagsModal shows one audiobook file's own embedded tags, read live off
// disk (see getBookFileTags) rather than the scan-time snapshot — the point
// being to answer "what does this file actually have on it right now," which
// the cached data can't after a "Write tags" call. Adapted from CantiNode's
// TrackFileTagsModal with an audiobook-appropriate field set.
export default function BookFileTagsModal({
  fileId,
  fileName,
  track,
  onClose,
}: {
  fileId: number;
  fileName: string;
  // A specific file within a multi-file audiobook (folder-relative name); when
  // absent, the unit's first file stands in.
  track?: string;
  onClose: () => void;
}) {
  const [tags, setTags] = useState<BookFileTags | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    api
      .getBookFileTags(fileId, track)
      .then((t) => {
        if (!cancelled) setTags(t);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [fileId, track]);

  const duration = tags?.durationSeconds ? formatDuration(tags.durationSeconds) : "";
  const audio = tags
    ? [tags.format?.toUpperCase(), tags.codec?.toUpperCase(), tags.bitrate ? `${tags.bitrate} kbps` : "",
       tags.sampleRate ? `${(tags.sampleRate / 1000).toFixed(1)} kHz` : "",
       tags.channels ? `${tags.channels} ch` : ""]
        .filter(Boolean)
        .join(" · ")
    : "";

  const fields: [string, string][] = tags
    ? [
        ["Title", tags.title],
        ["Author", tags.author],
        ["Album Artist", tags.albumArtist],
        ["Album", tags.album],
        ["Narrator", tags.narrator],
        ["Series", tags.series],
        ["Series Part", tags.seriesPart],
        ["Genre", tags.genre],
        ["Date", tags.date],
        ["Description", tags.description],
        ["ISBN", tags.isbn],
        ["ASIN", tags.asin],
        ["Cover art", tags.hasCover ? "embedded" : ""],
        ["Duration", duration],
        ["Audio", audio],
      ]
    : [];

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal tag-modal"
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        <h3>Embedded tags</h3>
        <p className="muted tag-modal-filename" title={fileName}>
          {fileName}
        </p>
        {loading && <p className="muted">Reading tags…</p>}
        {error && <p className="notice bad">{error}</p>}
        {!loading && !error && (
          <dl className="tag-fields">
            {fields.map(([label, value]) => (
              <div className="tag-field" key={label}>
                <dt>{label}</dt>
                <dd>{value || <span className="muted">—</span>}</dd>
              </div>
            ))}
          </dl>
        )}
        <div className="settings-actions">
          <button onClick={onClose}>Close</button>
        </div>
      </div>
    </div>
  );
}

// "3785" -> "1h 3m 5s"
function formatDuration(sec: number): string {
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  return [h ? `${h}h` : "", m ? `${m}m` : "", `${s}s`].filter(Boolean).join(" ");
}
