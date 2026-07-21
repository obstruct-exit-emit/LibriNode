import type { Book, HomeItem } from "../api";

// SortSelect is a compact, unobtrusive sort dropdown for a card header: a small
// ⇅ glyph and a borderless select showing the current option. Options are
// [key, label] pairs; the first is the section's natural/default order.
export function SortSelect({
  value,
  onChange,
  options,
}: {
  value: string;
  onChange: (v: string) => void;
  options: [key: string, label: string][];
}) {
  return (
    <label className="sort-select" title="Sort">
      <span className="sort-glyph" aria-hidden>
        ⇅
      </span>
      <select aria-label="Sort by" value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map(([key, label]) => (
          <option key={key} value={key}>
            {label}
          </option>
        ))}
      </select>
    </label>
  );
}

// sortBooks returns a new array sorted by the given key. "default" (or any
// unknown key) preserves the incoming order, so a section's current look is its
// default simply by starting on that key.
export function sortBooks(books: Book[], key: string): Book[] {
  const by = [...books];
  switch (key) {
    case "title":
      return by.sort((a, b) => (a.sortTitle || a.title).localeCompare(b.sortTitle || b.title));
    case "date": // newest first
      return by.sort((a, b) => (b.releaseDate || "").localeCompare(a.releaseDate || ""));
    case "date-asc": // oldest first
      return by.sort((a, b) => (a.releaseDate || "").localeCompare(b.releaseDate || ""));
    case "rating":
      return by.sort((a, b) => b.rating - a.rating);
    default:
      return by;
  }
}

// sortItems is the HomeItem (Wanted) equivalent of sortBooks.
export function sortItems(items: HomeItem[], key: string): HomeItem[] {
  const by = [...items];
  switch (key) {
    case "title":
      return by.sort((a, b) => a.title.localeCompare(b.title));
    case "date":
      return by.sort((a, b) => (b.releaseDate || "").localeCompare(a.releaseDate || ""));
    case "date-asc":
      return by.sort((a, b) => (a.releaseDate || "").localeCompare(b.releaseDate || ""));
    case "rating":
      return by.sort((a, b) => (b.rating || 0) - (a.rating || 0));
    default:
      return by;
  }
}
