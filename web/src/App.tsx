import { useEffect, useState } from "react";
import { api, ApiError, getApiKey, setApiKey } from "./api";
import LibraryView from "./views/LibraryView";
import SearchView from "./views/SearchView";
import SettingsView from "./views/SettingsView";
import SystemView from "./views/SystemView";
import "./App.css";

type Tab = "library" | "search" | "settings" | "system";

export default function App() {
  const [key, setKey] = useState(getApiKey());
  const [connected, setConnected] = useState(false);
  const [tab, setTab] = useState<Tab>("library");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!key) return;
    setError("");
    api
      .systemStatus()
      .then(() => setConnected(true))
      .catch((err: unknown) => {
        setConnected(false);
        setError(err instanceof ApiError ? err.message : String(err));
      });
  }, [key]);

  return (
    <div className="app">
      <header>
        <h1>🖋️ Quillarr</h1>
        {connected && (
          <nav>
            {(["library", "search", "settings", "system"] as const).map((t) => (
              <button
                key={t}
                className={tab === t ? "tab active" : "tab"}
                onClick={() => {
                  setError("");
                  setTab(t);
                }}
              >
                {t[0].toUpperCase() + t.slice(1)}
              </button>
            ))}
          </nav>
        )}
      </header>

      {!key && (
        <section className="card">
          <h2>Connect</h2>
          <p>
            Paste the API key from <code>config.yaml</code> in your Quillarr
            data directory.
          </p>
          <ApiKeyForm onSave={setKey} />
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

      {connected && tab === "library" && <LibraryView onError={setError} />}
      {connected && tab === "search" && <SearchView onError={setError} />}
      {connected && tab === "settings" && <SettingsView onError={setError} />}
      {connected && tab === "system" && <SystemView onError={setError} />}
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
