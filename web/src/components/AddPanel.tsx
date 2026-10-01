import { useEffect, useState } from "react";
import { api, addAuthorTo, addBookTo, type SearchAuthor, type SearchBook } from "../api";
import AddResultsGrid, { type AddResult } from "./AddResultsGrid";

// AddPanel searches the metadata provider — authors and books at once — and
// adds a result into the Ebooks library, the Audiobooks library, or both
// (mirrored) with one click. Shared by the library pages and global search, so
// adding works the same everywhere and never dead-ends.
export default function AddPanel({
  initialTerm = "",
  autoFocus = true,
  onAdded,
  onError,
}: {
  initialTerm?: string;
  autoFocus?: boolean;
  onAdded: () => void;
  onError: (message: string) => void;
}) {
  const [term, setTerm] = useState(initialTerm);
  const [authors, setAuthors] = useState<SearchAuthor[]>([]);
  const [books, setBooks] = useState<SearchBook[]>([]);
  const [busy, setBusy] = useState(false);
  const [searched, setSearched] = useState(false);

  const run = (query: string) => {
    if (!query.trim()) return;
    setBusy(true);
    Promise.all([api.searchAuthors(query), api.searchBooks(query)])
      .then(([a, b]) => {
        setAuthors(a);
        setBooks(b);
        setSearched(true);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
      .finally(() => setBusy(false));
  };

  // When opened from a search that already has a term, run it once on mount —
  // not on every keystroke of the term that seeded it.
  useEffect(() => {
    if (initialTerm.trim()) run(initialTerm);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    run(term);
  };

  const results: AddResult[] = [
    ...authors.map((a) => ({
      key: "author:" + a.foreignAuthorId,
      title: a.name,
      subtitle: a.bookCount ? `Author · ${a.bookCount} books` : "Author",
      imageUrl: a.imageUrl || undefined,
      add: (target: Parameters<AddResult["add"]>[0]) => addAuthorTo(a.foreignAuthorId, target),
    })),
    ...books.map((b) => ({
      key: "book:" + b.foreignBookId,
      title: b.title,
      subtitle: b.authorName + (b.releaseDate ? ` · ${b.releaseDate.slice(0, 4)}` : ""),
      imageUrl: b.coverUrl || undefined,
      add: (target: Parameters<AddResult["add"]>[0]) => addBookTo(b.foreignBookId, target),
    })),
  ];

  return (
    <div className="add-panel">
      <form onSubmit={search} className="search-form">
        <input
          placeholder="Search for an author or book to add…"
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          autoFocus={autoFocus}
        />
        <button type="submit" disabled={busy || !term.trim()}>
          {busy ? "Searching…" : "Search"}
        </button>
      </form>
      {!busy && searched && results.length === 0 && (
        <p className="muted">No authors or books matched on the metadata provider.</p>
      )}
      {!busy && !searched && (
        <p className="muted">
          Search the metadata provider — results appear with cover art, each with
          a one-click choice of Ebooks, Audiobooks, or both.
        </p>
      )}
      <AddResultsGrid results={results} onAdded={onAdded} />
    </div>
  );
}
