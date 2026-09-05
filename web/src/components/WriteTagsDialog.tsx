import { useState } from "react";

// WriteTagsDialog: one button opens this options window — merge (the default)
// vs clear-first — so clear-first doesn't need its own entry in the action row.
// Mirrors CantiNode's WriteTagsDialog.
export default function WriteTagsDialog({
  onConfirm,
  onClose,
}: {
  onConfirm: (clear: boolean) => void;
  onClose: () => void;
}) {
  const [clear, setClear] = useState(false);

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div
        className="modal write-tags-modal"
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
      >
        <h3>Write tags</h3>
        <p className="muted">
          Embed LibriNode's metadata into this book's audiobook file(s), so
          Audiobookshelf, Plex, or a phone app read it straight from the file.
        </p>
        <div className="write-tags-options">
          <label className={`write-tags-option${clear ? "" : " selected"}`}>
            <input
              type="radio"
              name="write-tags-mode"
              checked={!clear}
              onChange={() => setClear(false)}
            />
            <div>
              <div className="write-tags-option-title">Merge (recommended)</div>
              <p className="muted">
                Only touches the fields LibriNode manages — title, author, album,
                narrator, date, and cover. Everything else already on the file
                (comments, ratings, custom fields from other taggers) is left
                untouched.
              </p>
            </div>
          </label>
          <label className={`write-tags-option${clear ? " selected" : ""}`}>
            <input
              type="radio"
              name="write-tags-mode"
              checked={clear}
              onChange={() => setClear(true)}
            />
            <div>
              <div className="write-tags-option-title">Clear first</div>
              <p className="muted">
                Wipes every tag LibriNode doesn't manage before writing —
                existing cover art (a fresh one is re-embedded if enabled),
                comments, ratings, custom fields. <strong>This cannot be undone.</strong>
              </p>
            </div>
          </label>
        </div>
        <div className="settings-actions">
          <button className={clear ? "danger" : undefined} autoFocus onClick={() => onConfirm(clear)}>
            {clear ? "Clear and write" : "Write tags"}
          </button>
          <button className="toggle" onClick={onClose}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
