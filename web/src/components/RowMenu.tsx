import { useEffect, useRef, useState } from "react";

export interface RowMenuItem {
  label: string;
  onClick: () => void;
  // active renders a ✓ and highlights — for toggle items like "Mirror".
  active?: boolean;
  disabled?: boolean;
  danger?: boolean;
}

// RowMenu is a compact "⋯" overflow button that reveals a small dropdown of
// row actions, closing on an outside click or Escape — keeps per-row actions
// tucked away instead of crowding the row.
export default function RowMenu({
  items,
  title = "More",
}: {
  items: RowMenuItem[];
  title?: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);
  return (
    <div className="row-menu" ref={ref}>
      <button
        className="toggle row-menu-btn"
        title={title}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        ⋯
      </button>
      {open && (
        <div className="row-menu-pop" role="menu">
          {items.map((it) => (
            <button
              key={it.label}
              role="menuitem"
              disabled={it.disabled}
              className={[it.active ? "on" : "", it.danger ? "danger" : ""].join(" ").trim() || undefined}
              onClick={() => {
                setOpen(false);
                it.onClick();
              }}
            >
              {it.active ? "✓ " : ""}
              {it.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
