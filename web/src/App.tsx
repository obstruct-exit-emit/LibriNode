import { useCallback, useEffect, useState } from "react";
import {
  api,
  ApiError,
  getApiKey,
  setApiKey,
  type AuthStatus,
  type HealthIssue,
  type LibraryStatus,
} from "./api";
import ActivityView from "./views/ActivityView";
import AuthorDetailView from "./views/AuthorDetailView";
import BookDetailView from "./views/BookDetailView";
import BooksLibraryView from "./views/BooksLibraryView";
import CalendarView from "./views/CalendarView";
import HomeView from "./views/HomeView";
import SeriesDetailView from "./views/SeriesDetailView";
import SeriesLibraryView from "./views/SeriesLibraryView";
import SettingsView from "./views/SettingsView";
import SystemView from "./views/SystemView";
import "./App.css";

// Plex-style navigation: Home, then one entry per *active* library (a media
// type appears only once its library is set up), then the app-level pages.
type Page =
  | { name: "home" }
  | { name: "library"; mediaType: string }
  | { name: "author"; id: number; library: "ebook" | "audiobook" }
  | { name: "book"; id: number; library: "ebook" | "audiobook"; authorId: number }
  | { name: "series-detail"; id: number; mediaType: string }
  | { name: "calendar" }
  | { name: "activity" }
  | { name: "settings" }
  | { name: "system" };

export const libraryLabels: Record<string, string> = {
  ebook: "Ebooks",
  audiobook: "Audiobooks",
  manga: "Manga",
  comic: "Comics",
  magazine: "Magazines",
};

const libraryIcons: Record<string, string> = {
  ebook: "📖",
  audiobook: "🎧",
  manga: "🀄",
  comic: "💥",
  magazine: "📰",
};

export default function App() {
  const [key, setKey] = useState(getApiKey());
  const [auth, setAuth] = useState<AuthStatus | null>(null);
  const [connected, setConnected] = useState(false);
  const [libraries, setLibraries] = useState<LibraryStatus[]>([]);
  const [health, setHealth] = useState<HealthIssue[]>([]);
  const [page, setPage] = useState<Page>({ name: "home" });
  const [error, setError] = useState("");

  // Login sessions replace the API-key prompt once an account is set up
  // (Settings → General → Security); without one, the key prompt remains.
  useEffect(() => {
    api
      .authStatus()
      .then(setAuth)
      .catch(() => setAuth({ authEnabled: false, authenticated: false }));
  }, []);

  const reloadLibraries = useCallback(() => {
    api
      .libraries()
      .then(setLibraries)
      .catch(() => setLibraries([]));
  }, []);

  const reloadHealth = useCallback(() => {
    api
      .health()
      .then((h) => setHealth(h.issues))
      .catch(() => {}); // the banner is best-effort; never blocks the UI
  }, []);

  // Checks run server-side every 15 min; poll the cached result so the
  // banner appears/clears without a reload.
  useEffect(() => {
    if (!connected) return;
    reloadHealth();
    const timer = setInterval(reloadHealth, 60_000);
    return () => clearInterval(timer);
  }, [connected, reloadHealth]);

  useEffect(() => {
    if (!auth) return; // auth status still loading
    const ready = auth.authEnabled ? auth.authenticated : !!key;
    if (!ready) return;
    setError("");
    api
      .systemStatus()
      .then(() => {
        setConnected(true);
        reloadLibraries();
      })
      .catch((err: unknown) => {
        setConnected(false);
        setError(err instanceof ApiError ? err.message : String(err));
      });
  }, [auth, key, reloadLibraries]);

  const active = libraries.filter((l) => l.active);

  const go = (p: Page) => {
    setError("");
    setPage(p);
    reloadLibraries(); // library activity can change after adds/scans
  };

  const navButton = (p: Page, label: string, icon: string) => {
    const current =
      page.name === p.name &&
      (p.name !== "library" ||
        (page.name === "library" && page.mediaType === (p as { mediaType?: string }).mediaType));
    return (
      <button
        key={label}
        className={current ? "nav-item active" : "nav-item"}
        onClick={() => go(p)}
      >
        <span className="nav-icon">{icon}</span> {label}
      </button>
    );
  };

  return (
    <div className={connected ? "app with-sidebar" : "app"}>
      {connected && (
        <aside className="sidebar">
          <h1 className="brand">🖋️ LibriNode</h1>
          <nav>
            {navButton({ name: "home" }, "Home", "🏠")}
            {active.length > 0 && <div className="nav-group">Libraries</div>}
            {active.map((l) =>
              navButton(
                { name: "library", mediaType: l.mediaType },
                libraryLabels[l.mediaType] ?? l.mediaType,
                libraryIcons[l.mediaType] ?? "📚",
              ),
            )}
            <div className="nav-group">App</div>
            {navButton({ name: "calendar" }, "Calendar", "📅")}
            {navButton({ name: "activity" }, "Activity", "⬇️")}
            {navButton({ name: "settings" }, "Settings", "⚙️")}
            {navButton({ name: "system" }, "System", "🖥️")}
            {auth?.authEnabled && auth.authenticated && (
              <button
                className="nav-item"
                onClick={() => {
                  api
                    .logout()
                    .catch(() => {})
                    .finally(() => location.reload());
                }}
              >
                <span className="nav-icon">🚪</span> Log out
              </button>
            )}
          </nav>
        </aside>
      )}

      <main className="content">
        {!connected && <h1 className="brand">🖋️ LibriNode</h1>}

        {auth?.authEnabled && !auth.authenticated && (
          <section className="card">
            <h2>Sign in</h2>
            <LoginForm
              onLoggedIn={() => setAuth({ authEnabled: true, authenticated: true })}
            />
          </section>
        )}

        {auth && !auth.authEnabled && !key && (
          <section className="card">
            <h2>Connect</h2>
            <p>
              Paste the API key from <code>config.yaml</code> in your LibriNode
              data directory. (You can set up a login account later under
              Settings → General.)
            </p>
            <ApiKeyForm onSave={setKey} />
          </section>
        )}

        {connected && health.length > 0 && (
          <section className="card health-banner">
            {health.map((issue, i) => (
              <p key={i} className={issue.level === "error" ? "health-issue error" : "health-issue"}>
                {issue.level === "error" ? "⛔" : "⚠️"} {issue.message}
              </p>
            ))}
          </section>
        )}

        {error && (
          <section className="card error">
            <p>{error}</p>
            {!connected && key && (
              <button
                onClick={() => {
                  setApiKey("");
                  setKey("");
                  setError("");
                }}
              >
                Change API key
              </button>
            )}
          </section>
        )}

        {connected && page.name === "home" && (
          <HomeView
            onError={setError}
            onOpenLibrary={(mediaType) => go({ name: "library", mediaType })}
          />
        )}
        {connected && page.name === "library" &&
          (page.mediaType === "ebook" || page.mediaType === "audiobook" ? (
            <BooksLibraryView
              key={page.mediaType}
              library={page.mediaType as "ebook" | "audiobook"}
              onError={setError}
              onOpenAuthor={(id) =>
                go({ name: "author", id, library: page.mediaType as "ebook" | "audiobook" })
              }
            />
          ) : (
            <SeriesLibraryView
              key={page.mediaType}
              mediaType={page.mediaType}
              onError={setError}
              onOpenSeries={(id) => go({ name: "series-detail", id, mediaType: page.mediaType })}
            />
          ))}
        {connected && page.name === "author" && (
          <AuthorDetailView
            id={page.id}
            library={page.library}
            onError={setError}
            onBack={() => go({ name: "library", mediaType: page.library })}
            onOpenBook={(bookId) =>
              go({ name: "book", id: bookId, library: page.library, authorId: page.id })
            }
          />
        )}
        {connected && page.name === "book" && (
          <BookDetailView
            id={page.id}
            library={page.library}
            onError={setError}
            onBack={() => go({ name: "author", id: page.authorId, library: page.library })}
          />
        )}
        {connected && page.name === "series-detail" && (
          <SeriesDetailView
            id={page.id}
            mediaType={page.mediaType}
            onError={setError}
            onBack={() => go({ name: "library", mediaType: page.mediaType })}
          />
        )}
        {connected && page.name === "calendar" && <CalendarView onError={setError} />}
        {connected && page.name === "activity" && <ActivityView onError={setError} />}
        {connected && page.name === "settings" && (
          <SettingsView onError={setError} onLibrariesChanged={reloadLibraries} />
        )}
        {connected && page.name === "system" && <SystemView onError={setError} />}
      </main>
    </div>
  );
}

function LoginForm({ onLoggedIn }: { onLoggedIn: () => void }) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (!username.trim() || !password) return;
        setBusy(true);
        setNotice("");
        api
          .login(username.trim(), password)
          .then(onLoggedIn)
          .catch((err: unknown) =>
            setNotice(err instanceof Error ? err.message : String(err)),
          )
          .finally(() => setBusy(false));
      }}
    >
      <input
        placeholder="Username"
        value={username}
        onChange={(e) => setUsername(e.target.value)}
        autoFocus
      />
      <input
        type="password"
        placeholder="Password"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
      />
      <button type="submit" disabled={busy || !username.trim() || !password}>
        {busy ? "Signing in…" : "Sign in"}
      </button>
      {notice && <span className="notice bad">{notice}</span>}
    </form>
  );
}

function ApiKeyForm({ onSave }: { onSave: (key: string) => void }) {
  const [value, setValue] = useState("");
  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        const trimmed = value.trim();
        if (!trimmed) return;
        setApiKey(trimmed);
        onSave(trimmed);
      }}
    >
      <input
        type="password"
        placeholder="API key"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        autoFocus
      />
      <button type="submit">Save</button>
    </form>
  );
}
