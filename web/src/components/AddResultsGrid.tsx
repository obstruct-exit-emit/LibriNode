import { useState } from "react";
import { proxiedImage, addTargetLabel, type AddTarget } from "../api";
import { useUi } from "../ui";

// AddResultsGrid renders provider search results as a poster grid — cover art,
// title, subtitle, optional blurb — with either a one-click format choice
// (Ebooks, Audiobooks, or Both, mirrored) or, when addLabel is set, a single
// plain "add" button for results with no format split (e.g. series). Shared
// by every add flow.
export interface AddResult {
  key: string;
  title: string;
  subtitle?: string;
  blurb?: string;
  imageUrl?: string;
  /** When set, render one button with this label instead of the format-target picker. */
  addLabel?: string;
  add: (target: AddTarget) => Promise<unknown>;
}

const targets: { target: AddTarget; label: string; title: string }[] = [
  { target: "ebook", label: "📖 Ebooks", title: "Add to the Ebooks library" },
  { target: "audiobook", label: "🎧 Audiobooks", title: "Add to the Audiobooks library" },
  { target: "both", label: "⇄ Both", title: "Add to both and mirror ebook ↔ audiobook" },
];

export default function AddResultsGrid({
  results,
  onAdded,
}: {
  results: AddResult[];
  onAdded: () => void;
}) {
  const { toast } = useUi();
  // Per-card state: "busy" (which target is adding) or the target it was added to.
  const [state, setState] = useState<Record<string, { busy?: AddTarget; added?: AddTarget }>>({});

  if (results.length === 0) return null;

  const add = (r: AddResult, target: AddTarget) => {
    setState((s) => ({ ...s, [r.key]: { busy: target } }));
    r.add(target)
      .then(() => {
        setState((s) => ({ ...s, [r.key]: { added: target } }));
        toast(
          r.addLabel ? `Added "${r.title}"` : `Added "${r.title}" to ${addTargetLabel[target]}`,
          "ok",
        );
        onAdded();
      })
      .catch((err: unknown) => {
        setState((s) => {
          const next = { ...s };
          delete next[r.key];
          return next;
        });
        toast(String(err instanceof Error ? err.message : err), "bad");
      });
  };

  return (
    <div className="add-grid">
      {results.map((r) => {
        const st = state[r.key];
        return (
          <div key={r.key} className="add-card">
            {r.imageUrl ? (
              <img className="poster" src={proxiedImage(r.imageUrl)} alt="" loading="lazy" />
            ) : (
              <div className="poster fallback">{r.title.charAt(0)}</div>
            )}
            <div className="add-card-body">
              <span className="poster-title" title={r.title}>
                {r.title}
              </span>
              {r.subtitle && (
                <span className="poster-sub" title={r.subtitle}>
                  {r.subtitle}
                </span>
              )}
              {r.blurb && <p className="add-blurb">{r.blurb}</p>}
            </div>
            {st?.added ? (
              <span className="add-done">
                {r.addLabel ? "✓ Added" : `✓ Added to ${addTargetLabel[st.added]}`}
              </span>
            ) : r.addLabel ? (
              <div className="add-targets">
                <button
                  className="toggle"
                  disabled={!!st?.busy}
                  onClick={() => add(r, "both")}
                >
                  {st?.busy ? "Adding…" : r.addLabel}
                </button>
              </div>
            ) : (
              <div className="add-targets">
                {targets.map((t) => (
                  <button
                    key={t.target}
                    className="toggle"
                    title={t.title}
                    disabled={!!st?.busy}
                    onClick={() => add(r, t.target)}
                  >
                    {st?.busy === t.target ? "Adding…" : t.label}
                  </button>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
