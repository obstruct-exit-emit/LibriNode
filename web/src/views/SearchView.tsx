import { useState } from "react";
import { api, type SearchAuthor, type SearchBook } from "../api";

type SearchType = "book" | "author";

export default function SearchView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [term, setTerm] = useState("");
  const [type, setType] = useState<SearchType>("book");
  const [authors, setAuthors] = useState<SearchAuthor[]>([]);
  const [books, setBooks] = useState<SearchBook[]>([]);
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState(false);
  const [added, setAdded] = useState<Record<string, boolean>>({});

  const search = (e: React.FormEvent) => {
    e.preventDefault();
    const q = term.trim();
    if (!q) return;
    setSearching(true);
    setSearched(false);
    const done = () => {
      setSearching(false);
      setSearched(true);
    };
    if (type === "author") {
      api
        .searchAuthors(q)
        .then((r) => {
          setAuthors(r);
          setBooks([]);
        })
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
        .finally(done);
    } else {
      api
        .searchBooks(q)
        .then((r) => {
          setBooks(r);
          setAuthors([]);
        })
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
        .finally(done);
    }
  };

  const add = (key: string, action: () => Promise<unknown>) => {
    action()
      .then(() => setAdded((m) => ({ ...m, [key]: true })))
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  };

  const results = type === "author" ? authors.length : books.length;

  return (
    <section className="card">
      <h2>Search</h2>
      <form onSubmit={search} className="search-form">
        <select
          value={type}
          onChange={(e) => setType(e.target.value as SearchType)}
        >
          <option value="book">Books</option>
          <option value="author">Authors</option>
        </select>
        <input
          placeholder={type === "book" ? "Title…" : "Author name…"}
          value={term}
          onChange={(e) => setTerm(e.target.value)}
          autoFocus
        />
        <button type="submit" disabled={searching}>
          {searching ? "Searching…" : "Search"}
        </button>
      </form>

      {searched && results === 0 && (
        <p className="muted">No results for “{term.trim()}”.</p>
      )}

      {authors.length > 0 && (
        <ul className="rows results">
          {authors.map((a) => (
            <li key={a.foreignAuthorId}>
              <div className="row">
                <span className="result">
                  {a.imageUrl && <img src={a.imageUrl} alt="" />}
                  <span>
                    {a.name}
                    {a.bookCount ? (
                      <span className="muted"> — {a.bookCount} books</span>
                    ) : null}
                  </span>
                </span>
                <AddButton
                  added={!!added[`a${a.foreignAuthorId}`]}
                  onAdd={() =>
                    add(`a${a.foreignAuthorId}`, () => api.addAuthor(a.foreignAuthorId))
                  }
                />
              </div>
            </li>
          ))}
        </ul>
      )}

      {books.length > 0 && (
        <ul className="rows results">
          {books.map((b) => (
            <li key={b.foreignBookId}>
              <div className="row">
                <span className="result">
                  {b.coverUrl && <img src={b.coverUrl} alt="" />}
                  <span>
                    {b.title}
                    <span className="muted">
                      {b.authorName && ` — ${b.authorName}`}
                      {b.releaseDate && ` (${b.releaseDate.slice(0, 4)})`}
                      {b.rating > 0 && ` · ★ ${b.rating.toFixed(1)}`}
                    </span>
                  </span>
                </span>
                <AddButton
                  added={!!added[`b${b.foreignBookId}`]}
                  onAdd={() =>
                    add(`b${b.foreignBookId}`, () => api.addBook(b.foreignBookId))
                  }
                />
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function AddButton({ added, onAdd }: { added: boolean; onAdd: () => void }) {
  return added ? (
    <span className="added">✓ in library</span>
  ) : (
    <button onClick={onAdd}>Add</button>
  );
}
