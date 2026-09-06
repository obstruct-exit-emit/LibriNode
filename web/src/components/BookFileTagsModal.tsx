import { useEffect, useState } from "react";
import { api, type FileTags } from "../api";

// BookFileTagsModal shows one file's own embedded tags, read live off disk (see
// getBookFileTags) rather than the scan-time snapshot — the point being to
// answer "what does this file actually have on it right now," which the cached
// data can't after a "Write tags" call. Handles both an audiobook file's audio
// tags and an ebook file's metadata. Adapted from CantiNode's TrackFileTagsModal.
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
  const [data, setData] = useState<FileTags | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    api
      .getBookFileTags(fileId, track)
      .then((t) => {
        if (!cancelled) setData(t);
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

  const fields = data ? fieldsFor(data) : [];

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

function fieldsFor(data: FileTags): [string, string][] {
  if (data.kind === "ebook" && data.ebook) {
    const e = data.ebook;
    return [
      ["Title", e.title],
      ["Author", e.author],
      ["Series", e.series],
      ["Series #", e.seriesIndex],
      ["Genre", e.genre],
      ["Language", e.language],
      ["Publisher", e.publisher],
      ["Date", e.date],
      ["ISBN", e.isbn],
      ["ASIN", e.asin],
      ["Description", e.description],
      ["Format", e.format ? e.format.toUpperCase() + (e.writable ? "" : " (read-only)") : ""],
    ];
  }
  const a = data.audiobook;
  if (!a) return [];
  const duration = a.durationSeconds ? formatDuration(a.durationSeconds) : "";
  const audio = [
    a.format?.toUpperCase(),
    a.codec?.toUpperCase(),
    a.bitrate ? `${a.bitrate} kbps` : "",
    a.sampleRate ? `${(a.sampleRate / 1000).toFixed(1)} kHz` : "",
    a.channels ? `${a.channels} ch` : "",
  ]
    .filter(Boolean)
    .join(" · ");
  return [
    ["Title", a.title],
    ["Author", a.author],
    ["Album Artist", a.albumArtist],
    ["Album", a.album],
    ["Narrator", a.narrator],
    ["Series", a.series],
    ["Series Part", a.seriesPart],
    ["Genre", a.genre],
    ["Date", a.date],
    ["Description", a.description],
    ["ISBN", a.isbn],
    ["ASIN", a.asin],
    ["Cover art", a.hasCover ? "embedded" : ""],
    ["Duration", duration],
    ["Audio", audio],
  ];
}

// "3785" -> "1h 3m 5s"
function formatDuration(sec: number): string {
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  return [h ? `${h}h` : "", m ? `${m}m` : "", `${s}s`].filter(Boolean).join(" ");
}
