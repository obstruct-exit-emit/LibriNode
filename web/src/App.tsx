import { useEffect, useState } from "react";
import {
  api,
  ApiError,
  getApiKey,
  setApiKey,
  type Author,
  type SystemStatus,
} from "./api";
import "./App.css";

export default function App() {
  const [key, setKey] = useState(getApiKey());
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [authors, setAuthors] = useState<Author[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!key) return;
    setError("");
    Promise.all([api.systemStatus(), api.listAuthors()])
      .then(([st, au]) => {
        setStatus(st);
        setAuthors(au);
      })
      .catch((err: unknown) => {
        setStatus(null);
        setError(err instanceof ApiError ? err.message : String(err));
      });
  }, [key]);

  return (
    <div className="app">
      <header>
        <h1>🖋️ Quillarr</h1>
        <p className="tagline">written-media automation</p>
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
          <button onClick={() => setKey("")}>Change API key</button>
        </section>
      )}

      {status && (
        <section className="card">
          <h2>System</h2>
          <dl>
            <dt>Version</dt>
            <dd>{status.version}</dd>
            <dt>Platform</dt>
            <dd>
              {status.os}/{status.arch}
            </dd>
            <dt>Uptime</dt>
            <dd>{status.uptime}</dd>
          </dl>
        </section>
      )}

      {status && (
        <section className="card">
          <h2>Authors ({authors.length})</h2>
          {authors.length === 0 ? (
            <p className="muted">
              Library is empty. Add authors via the API — the full browsing UI
              is on its way.
            </p>
          ) : (
            <ul className="authors">
              {authors.map((a) => (
                <li key={a.id}>
                  <span>{a.name}</span>
                  <span className="muted">
                    {a.monitored ? "monitored" : "unmonitored"}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
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
