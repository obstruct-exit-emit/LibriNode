import { useEffect, useState } from "react";
import { api, type MetadataSettings, type ProviderSettings } from "../api";

export default function SettingsView({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [settings, setSettings] = useState<MetadataSettings | null>(null);
  const [active, setActive] = useState("");
  const [providers, setProviders] = useState<Record<string, ProviderSettings>>({});
  const [showToken, setShowToken] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    api
      .getMetadataSettings()
      .then((s) => {
        setSettings(s);
        setActive(s.active);
        setProviders(s.providers);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  if (!settings) return <p className="muted">Loading…</p>;

  const activeSettings = providers[active] ?? { token: "" };

  const setToken = (token: string) => {
    setProviders({ ...providers, [active]: { ...activeSettings, token } });
    setNotice("");
  };

  const test = () => {
    setBusy(true);
    setNotice("");
    api
      .testMetadataProvider(active, activeSettings)
      .then(() => setNotice("✓ Connection OK — token accepted"))
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const save = () => {
    setBusy(true);
    setNotice("");
    api
      .saveMetadataSettings(active, providers)
      .then((s) => {
        setSettings(s);
        setActive(s.active);
        setProviders(s.providers);
        const hasToken = s.active && s.providers[s.active]?.token;
        setNotice(
          hasToken
            ? "✓ Saved — metadata search is live"
            : "Saved — no token set, metadata features stay disabled",
        );
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  return (
    <section className="card">
      <h2>Metadata Provider</h2>
      <p className="muted">
        Book and audiobook metadata (authors, series, editions, covers) comes
        from the provider below. Providers are pluggable — more sources can be
        added without touching the rest of the app.
      </p>

      <div className="settings-form">
        <label>
          Provider
          <select
            value={active}
            onChange={(e) => {
              setActive(e.target.value);
              setNotice("");
            }}
          >
            <option value="">None (disabled)</option>
            {settings.available.map((name) => (
              <option key={name} value={name}>
                {name[0].toUpperCase() + name.slice(1)}
              </option>
            ))}
          </select>
        </label>

        {active && (
          <label>
            API token
            <span className="token-row">
              <input
                type={showToken ? "text" : "password"}
                placeholder={
                  active === "hardcover"
                    ? "Token from hardcover.app/account/api"
                    : "API token"
                }
                value={activeSettings.token}
                onChange={(e) => setToken(e.target.value)}
              />
              <button
                type="button"
                className="toggle"
                onClick={() => setShowToken(!showToken)}
              >
                {showToken ? "hide" : "show"}
              </button>
            </span>
          </label>
        )}

        <div className="settings-actions">
          {active && (
            <button disabled={busy || !activeSettings.token} onClick={test}>
              Test
            </button>
          )}
          <button disabled={busy} onClick={save}>
            Save
          </button>
          {notice && (
            <span className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>
              {notice}
            </span>
          )}
        </div>
      </div>
    </section>
  );
}
