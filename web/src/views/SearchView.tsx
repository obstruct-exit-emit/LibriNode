import { useState } from "react";
import { api, type SearchAuthor, type SearchBook, type SeriesResult } from "../api";

type SearchType = "book" | "author" | "manga" | "comic";

export default function SearchView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [term, setTerm] = useState("");
  const [type, setType] = useState<SearchType>("book");
  const [authors, setAuthors] = useState<SearchAuthor[]>([]);
  const [books, setBooks] = useState<SearchBook[]>([]);
  const [seriesResults, setSeriesResults] = useState<SeriesResult[]>([]);
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
          setSeriesResults([]);
        })
        .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)))
        .finally(done);
    } else if (type === "manga" || type === "comic") {
      api
        .searchSeries(q, type)
        .then((r) => {
          setSeriesResults(r);
          setAuthors([]);
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
          setSeriesResults([]);
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

  const results =
    type === "author"
      ? authors.length
      : type === "manga" || type === "comic"
        ? seriesResults.length
        : books.length;

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
          <option value="manga">Manga</option>
          <option value="comic">Comics</option>
        </select>
        <input
          placeholder={type === "author" ? "Author name…" : "Title…"}
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

      {seriesResults.length > 0 && (
        <ul className="rows results">
          {seriesResults.map((s) => (
            <li key={s.foreignSeriesId}>
              <div className="row">
                <span className="result">
                  {s.coverUrl && <img src={s.coverUrl} alt="" />}
                  <span>
                    {s.title}
                    <span className="muted">
                      {s.authorName && ` — ${s.authorName}`}
                      {s.year ? ` (${s.year})` : ""}
                      {s.issueCount > 0 &&
                        ` · ${s.issueCount} ${type === "manga" ? "volumes" : "issues"}`}
                    </span>
                  </span>
                </span>
                <AddButton
                  added={!!added[`s${s.foreignSeriesId}`]}
                  onAdd={() =>
                    add(`s${s.foreignSeriesId}`, () => api.addSeries(type, s.foreignSeriesId))
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
