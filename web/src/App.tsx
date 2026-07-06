import { useCallback, useEffect, useState } from "react";
import {
  api,
  ApiError,
  getApiKey,
  setApiKey,
  type HealthIssue,
  type LibraryStatus,
} from "./api";
import ActivityView from "./views/ActivityView";
import AuthorDetailView from "./views/AuthorDetailView";
import BooksLibraryView from "./views/BooksLibraryView";
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
  | { name: "series-detail"; id: number; mediaType: string }
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
  const [connected, setConnected] = useState(false);
  const [libraries, setLibraries] = useState<LibraryStatus[]>([]);
  const [health, setHealth] = useState<HealthIssue[]>([]);
  const [page, setPage] = useState<Page>({ name: "home" });
  const [error, setError] = useState("");

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
    if (!key) return;
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
  }, [key, reloadLibraries]);

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
            {navButton({ name: "activity" }, "Activity", "⬇️")}
            {navButton({ name: "settings" }, "Settings", "⚙️")}
            {navButton({ name: "system" }, "System", "🖥️")}
          </nav>
        </aside>
      )}

      <main className="content">
        {!connected && <h1 className="brand">🖋️ LibriNode</h1>}

        {!key && (
          <section className="card">
            <h2>Connect</h2>
            <p>
              Paste the API key from <code>config.yaml</code> in your LibriNode
              data directory.
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
        {connected && page.name === "activity" && <ActivityView onError={setError} />}
        {connected && page.name === "settings" && (
          <SettingsView onError={setError} onLibrariesChanged={reloadLibraries} />
        )}
        {connected && page.name === "system" && <SystemView onError={setError} />}
      </main>
    </div>
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
