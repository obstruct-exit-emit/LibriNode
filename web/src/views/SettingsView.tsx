import { useCallback, useEffect, useState } from "react";
import {
  api,
  getApiKey,
  setApiKey,
  type AuthStatus,
  type DownloadClient,
  type Indexer,
  type MetadataSettings,
  type NamingSettings,
  type ProviderSettings,
  type QualityProfile,
  type RootFolder,
  type SystemStatus,
} from "../api";

// Settings groups, *arr-style: pages organized by concern instead of one
// long scroll. Order matches the README spec.
const settingsGroups = [
  "Media Management",
  "Libraries",
  "Metadata",
  "Indexers",
  "Download Clients",
  "General",
] as const;
type SettingsGroup = (typeof settingsGroups)[number];

export default function SettingsView({
  onError,
  onLibrariesChanged,
}: {
  onError: (message: string) => void;
  onLibrariesChanged?: () => void;
}) {
  const [group, setGroup] = useState<SettingsGroup>("Media Management");

  // Plex-style gating: type-specific settings render only for libraries
  // that are set up. Root Folders always offers every type — that's how a
  // library gets created in the first place.
  const [activeTypes, setActiveTypes] = useState<string[]>([]);
  const reloadLibraries = useCallback(() => {
    api
      .libraries()
      .then((ls) => setActiveTypes(ls.filter((l) => l.active).map((l) => l.mediaType)))
      .catch(() => setActiveTypes([]));
  }, []);
  useEffect(reloadLibraries, [reloadLibraries]);

  const librariesChanged = () => {
    reloadLibraries();
    onLibrariesChanged?.();
  };

  return (
    <>
      <nav className="subnav">
        {settingsGroups.map((g) => (
          <button
            key={g}
            className={g === group ? "tab active" : "tab"}
            onClick={() => setGroup(g)}
          >
            {g}
          </button>
        ))}
      </nav>

      {group === "Media Management" && (
        <>
          <RootFoldersCard onError={onError} onChanged={librariesChanged} />
          <NamingCard onError={onError} activeTypes={activeTypes} />
        </>
      )}
      {group === "Libraries" && (
        <QualityProfilesCard onError={onError} activeTypes={activeTypes} />
      )}
      {group === "Metadata" && <MetadataCard onError={onError} />}
      {group === "Indexers" && <IndexersCard onError={onError} />}
      {group === "Download Clients" && <DownloadClientsCard onError={onError} />}
      {group === "General" && <GeneralCard onError={onError} />}
    </>
  );
}

// General: instance facts, the login account, and the API key. Server-side
// options (host/port, SSL, proxy) live in config.yaml — see the README's
// reverse-proxy section for HTTPS guidance.
function GeneralCard({ onError }: { onError: (message: string) => void }) {
  const [status, setStatus] = useState<SystemStatus | null>(null);
  const [key, setKey] = useState(getApiKey());
  const [showKey, setShowKey] = useState(false);
  const [keyNotice, setKeyNotice] = useState("");

  useEffect(() => {
    api
      .systemStatus()
      .then(setStatus)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  const regenerate = () => {
    if (
      !confirm(
        "Regenerate the API key?\n\nProwlarr and any scripts using the current key stop working until you update them.",
      )
    ) {
      return;
    }
    api
      .regenerateApiKey()
      .then((r) => {
        setApiKey(r.apiKey); // keep this browser working
        setKey(r.apiKey);
        setKeyNotice("✓ New key generated — update Prowlarr and any scripts using the old one");
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  };

  return (
    <>
      <SecurityCard onError={onError} />

      <section className="card">
        <h2>API Key</h2>
        <p className="muted">
          Used by scripts and Prowlarr (and by this browser when no login
          account is set). Regenerating invalidates the old key everywhere.
        </p>
        <div className="settings-form">
          <label>
            API key
            <span className="token-row">
              <input
                type={showKey ? "text" : "password"}
                value={key}
                onChange={(e) => setKey(e.target.value)}
              />
              <button type="button" className="toggle" onClick={() => setShowKey(!showKey)}>
                {showKey ? "hide" : "show"}
              </button>
            </span>
          </label>
          <div className="settings-actions">
            <button
              disabled={!key.trim() || key.trim() === getApiKey()}
              onClick={() => {
                setApiKey(key.trim());
                location.reload();
              }}
            >
              Save & reconnect
            </button>
            <button className="danger" onClick={regenerate}>
              Regenerate
            </button>
            {keyNotice && <span className="notice ok">{keyNotice}</span>}
          </div>
        </div>
      </section>

      <section className="card">
        <h2>Instance</h2>
        {status ? (
          <dl>
            <dt>Version</dt>
            <dd>{status.appVersion ?? status.version}</dd>
            <dt>Platform</dt>
            <dd>
              {status.os}/{status.arch}
            </dd>
            <dt>Data directory</dt>
            <dd>{status.dataDir}</dd>
            <dt>Uptime</dt>
            <dd>{status.uptime}</dd>
          </dl>
        ) : (
          <p className="muted">Loading…</p>
        )}
        <p className="muted" style={{ marginBottom: 0 }}>
          Host, port, and data directory are set in <code>config.yaml</code>{" "}
          (or <code>LIBRINODE_*</code> environment variables) and need a
          restart. For HTTPS, run LibriNode behind a reverse proxy — see the
          README. Health checks, logs, and backups live on the System page.
        </p>
      </section>
    </>
  );
}

// SecurityCard manages the optional login account. When one is set, the UI
// requires signing in (sessions) instead of pasting the API key; the key
// keeps working for scripts and Prowlarr.
function SecurityCard({ onError }: { onError: (message: string) => void }) {
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .authStatus()
      .then(setStatus)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const save = () => {
    if (password !== confirmPw) {
      setNotice("✗ Passwords don't match");
      return;
    }
    setBusy(true);
    setNotice("");
    api
      .setCredentials(username.trim(), password)
      .then(() => {
        setNotice("✓ Login required from now on — this browser is already signed in");
        setPassword("");
        setConfirmPw("");
        reload();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const disable = () => {
    if (!confirm("Disable the login requirement? The UI goes back to the API-key prompt.")) return;
    setBusy(true);
    setNotice("");
    api
      .setCredentials("", "")
      .then(() => {
        setNotice("✓ Login disabled");
        reload();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  return (
    <section className="card">
      <h2>Security</h2>
      <p className="muted">
        {status?.authEnabled
          ? "A login account is set — the UI requires signing in. Set new credentials below to change them."
          : "No login account yet — the UI asks for the raw API key. Set a username and password to switch to a login page (sessions last 30 days; a server restart signs everyone out)."}
      </p>
      <div className="settings-form">
        <label>
          Username
          <input value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
        <label>
          Password (min. 8 characters)
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </label>
        <label>
          Confirm password
          <input
            type="password"
            value={confirmPw}
            onChange={(e) => setConfirmPw(e.target.value)}
          />
        </label>
        <div className="settings-actions">
          <button
            disabled={busy || !username.trim() || password.length < 8}
            onClick={save}
          >
            {status?.authEnabled ? "Update credentials" : "Enable login"}
          </button>
          {status?.authEnabled && (
            <button className="danger" disabled={busy} onClick={disable}>
              Disable login
            </button>
          )}
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

const emptyDownloadClient: Omit<DownloadClient, "id"> = {
  name: "",
  type: "qbittorrent",
  host: "",
  username: "",
  password: "",
  apiKey: "",
  category: "librinode",
  enabled: true,
  priority: 1,
};

function DownloadClientsCard({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [clients, setClients] = useState<DownloadClient[]>([]);
  const [draft, setDraft] = useState(emptyDownloadClient);
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .listDownloadClients()
      .then(setClients)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const set = (patch: Partial<typeof emptyDownloadClient>) =>
    setDraft((d) => ({ ...d, ...patch }));

  const act = (action: () => Promise<unknown>, done?: string) => {
    setBusy(true);
    setNotice("");
    action()
      .then(() => {
        if (done) setNotice(done);
        reload();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const draftValid =
    draft.name.trim() !== "" &&
    /^https?:\/\//.test(draft.host.trim()) &&
    (draft.type !== "sabnzbd" || draft.apiKey.trim() !== "");

  return (
    <section className="card">
      <h2>Download Clients</h2>
      <p className="muted">
        Where grabbed releases go: <strong>qBittorrent</strong> for torrents,{" "}
        <strong>SABnzbd</strong> for usenet. Downloads are tagged with the
        category so LibriNode only tracks its own.
      </p>

      {clients.length > 0 && (
        <ul className="rows">
          {clients.map((c) => (
            <li key={c.id}>
              <div className="row">
                <span>
                  {c.name} <span className="muted">({c.type})</span>
                </span>
                <span className="row-actions">
                  <span className="muted file-path">{c.host}</span>
                  <button
                    className="toggle"
                    disabled={busy}
                    title="Check the saved connection still works"
                    onClick={() => act(async () => {
                      await api.testDownloadClient(c);
                    }, `✓ ${c.name}: connection OK`)}
                  >
                    test
                  </button>
                  <button
                    className={c.enabled ? "toggle on" : "toggle"}
                    disabled={busy}
                    onClick={() => act(() => api.updateDownloadClient({ ...c, enabled: !c.enabled }))}
                  >
                    {c.enabled ? "enabled" : "disabled"}
                  </button>
                  <button
                    className="danger"
                    disabled={busy}
                    onClick={() => {
                      if (confirm(`Remove download client ${c.name}?`)) {
                        act(() => api.deleteDownloadClient(c.id));
                      }
                    }}
                  >
                    remove
                  </button>
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div className="settings-form" style={{ marginTop: "0.75rem" }}>
        <label>
          Name
          <input value={draft.name} onChange={(e) => set({ name: e.target.value })} />
        </label>
        <label>
          Type
          <select
            value={draft.type}
            onChange={(e) => set({ type: e.target.value as DownloadClient["type"] })}
          >
            <option value="qbittorrent">qBittorrent (torrents)</option>
            <option value="sabnzbd">SABnzbd (usenet)</option>
          </select>
        </label>
        <label>
          Host
          <input
            placeholder="http://localhost:8080"
            value={draft.host}
            onChange={(e) => set({ host: e.target.value })}
          />
        </label>
        {draft.type === "qbittorrent" ? (
          <>
            <label>
              Username
              <input
                value={draft.username}
                onChange={(e) => set({ username: e.target.value })}
              />
            </label>
            <label>
              Password
              <input
                type="password"
                value={draft.password}
                onChange={(e) => set({ password: e.target.value })}
              />
            </label>
          </>
        ) : (
          <label>
            API key
            <input value={draft.apiKey} onChange={(e) => set({ apiKey: e.target.value })} />
          </label>
        )}
        <button
          type="button"
          className="toggle"
          style={{ alignSelf: "flex-start" }}
          onClick={() => setAdvanced(!advanced)}
        >
          {advanced ? "▾ hide advanced" : "▸ advanced (category)"}
        </button>
        {advanced && (
          <label>
            Category
            <input
              title="Downloads are tagged with this category so LibriNode only tracks its own"
              value={draft.category}
              onChange={(e) => set({ category: e.target.value })}
            />
          </label>
        )}
        <div className="settings-actions">
          <button
            disabled={busy || !draftValid}
            onClick={() => {
              setBusy(true);
              setNotice("");
              api
                .testDownloadClient(draft)
                .then(() => setNotice("✓ Connection OK"))
                .catch((err: unknown) =>
                  setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
                )
                .finally(() => setBusy(false));
            }}
          >
            Test
          </button>
          <button
            disabled={busy || !draftValid}
            onClick={() =>
              act(() => api.addDownloadClient(draft).then(() => setDraft(emptyDownloadClient)), "✓ Client added")
            }
          >
            Add
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

function QualityProfilesCard({
  onError,
  activeTypes,
}: {
  onError: (message: string) => void;
  activeTypes: string[];
}) {
  const profileTypes = activeTypes.length > 0 ? activeTypes : ["ebook"];
  const defaultFormats: Record<string, string> = {
    ebook: "epub,azw3,mobi",
    audiobook: "m4b,m4a,mp3",
    manga: "cbz,cbr",
    comic: "cbz,cbr",
    magazine: "pdf,epub",
  };
  const [profiles, setProfiles] = useState<QualityProfile[]>([]);
  const [name, setName] = useState("");
  const [profileType, setProfileType] = useState("ebook");
  const [formats, setFormats] = useState(defaultFormats.ebook);
  const [language, setLanguage] = useState("english");
  const [upgrades, setUpgrades] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .listProfiles()
      .then(setProfiles)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const run = (action: () => Promise<unknown>) => {
    setBusy(true);
    setNotice("");
    action()
      .then(reload)
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const add = () =>
    run(() =>
      api
        .addProfile({
          name: name.trim(),
          mediaType: profileType,
          formats: formats.split(",").map((f) => f.trim()).filter(Boolean),
          language,
          retailBonus: 25,
          upgradesAllowed: upgrades,
        })
        .then(() => setName("")),
    );

  return (
    <section className="card">
      <h2>Quality Profiles</h2>
      <p className="muted">
        Which release formats are grabbable, best first — release search
        rejects formats a profile doesn't list. The <strong>default</strong>{" "}
        profile drives scoring; per-author profiles come later.
      </p>

      <ul className="rows">
        {profiles.map((p) => (
          <li key={p.id}>
            <div className="row">
              <span>
                {p.name} <span className="muted">({p.mediaType})</span>{" "}
                {p.isDefault && <span className="owned yes">default</span>}
              </span>
              <span className="row-actions">
                <span className="muted">
                  {p.formats.join(" › ")}
                  {p.language ? ` · ${p.language}` : " · any language"}
                </span>
                <button
                  className={p.upgradesAllowed ? "toggle on" : "toggle"}
                  disabled={busy}
                  title="When on, owning a lesser format keeps the book wanted until the profile's best format"
                  onClick={() => run(() => api.updateProfile({ ...p, upgradesAllowed: !p.upgradesAllowed }))}
                >
                  {p.upgradesAllowed ? "upgrades on" : "upgrades off"}
                </button>
                {!p.isDefault && (
                  <>
                    <button
                      className="toggle"
                      disabled={busy}
                      onClick={() => run(() => api.setDefaultProfile(p.id))}
                    >
                      make default
                    </button>
                    <button
                      className="danger"
                      disabled={busy}
                      onClick={() => run(() => api.deleteProfile(p.id))}
                    >
                      remove
                    </button>
                  </>
                )}
              </span>
            </div>
          </li>
        ))}
      </ul>

      <div className="settings-form" style={{ marginTop: "0.75rem" }}>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label>
          Media type
          <select
            value={profileType}
            onChange={(e) => {
              setProfileType(e.target.value);
              setFormats(defaultFormats[e.target.value] ?? "");
            }}
          >
            {profileTypes.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <label>
          Formats (best first)
          <input
            value={formats}
            onChange={(e) => setFormats(e.target.value)}
            placeholder="epub,azw3,mobi"
          />
        </label>
        <label>
          <span>
            <input
              type="checkbox"
              checked={upgrades}
              onChange={(e) => setUpgrades(e.target.checked)}
            />{" "}
            Allow upgrades (keep wanted until the best format is owned)
          </span>
        </label>
        <label>
          Language
          <select value={language} onChange={(e) => setLanguage(e.target.value)}>
            <option value="english">English only</option>
            <option value="">Any language</option>
            <option value="german">German</option>
            <option value="french">French</option>
            <option value="spanish">Spanish</option>
          </select>
        </label>
        <div className="settings-actions">
          <button disabled={busy || !name.trim() || !formats.trim()} onClick={add}>
            Add profile
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

const emptyIndexer: Omit<Indexer, "id" | "addedAt"> = {
  name: "",
  type: "torznab",
  baseUrl: "",
  apiKey: "",
  categories: "7000,7020",
  audioCategories: "3030",
  comicCategories: "7030",
  magazineCategories: "7010",
  enabled: true,
  priority: 25,
};

function IndexersCard({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const [indexers, setIndexers] = useState<Indexer[]>([]);
  const [draft, setDraft] = useState(emptyIndexer);
  const [advanced, setAdvanced] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .listIndexers()
      .then(setIndexers)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const set = (patch: Partial<typeof emptyIndexer>) =>
    setDraft((d) => ({ ...d, ...patch }));

  const run = (action: () => Promise<unknown>, done?: string) => {
    setBusy(true);
    setNotice("");
    action()
      .then(() => {
        if (done) setNotice(done);
        reload();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const testDraft = () => {
    setBusy(true);
    setNotice("");
    api
      .testIndexer(draft)
      .then(() => setNotice("✓ Connection OK"))
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const add = () => {
    setBusy(true);
    setNotice("");
    api
      .addIndexer(draft)
      .then(() => {
        setDraft(emptyIndexer);
        setNotice("✓ Indexer added");
        reload();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const toggle = (ind: Indexer) =>
    run(() => api.updateIndexer({ ...ind, enabled: !ind.enabled }));

  const remove = (ind: Indexer) => {
    if (!confirm(`Remove indexer ${ind.name}?`)) return;
    run(() => api.deleteIndexer(ind.id));
  };

  const draftValid =
    draft.name.trim() !== "" && /^https?:\/\//.test(draft.baseUrl.trim());

  return (
    <section className="card">
      <h2>Indexers</h2>
      <p className="muted">
        Newznab (usenet) and Torznab (torrents — Prowlarr/Jackett feeds work)
        endpoints. Add them here by hand, or add LibriNode to Prowlarr as a{" "}
        <strong>Readarr</strong> application and Prowlarr keeps them in sync.
        Categories default to books (<code>7000,7020</code>); per-type
        categories are under advanced.
      </p>

      {indexers.length > 0 && (
        <ul className="rows">
          {indexers.map((ind) => (
            <li key={ind.id}>
              <div className="row">
                <span>
                  {ind.name} <span className="muted">({ind.type})</span>
                </span>
                <span className="row-actions">
                  <span className="muted file-path">{ind.baseUrl}</span>
                  <button
                    className="toggle"
                    disabled={busy}
                    title="Check the saved connection still works"
                    onClick={() => run(async () => {
                      await api.testIndexer(ind);
                    }, `✓ ${ind.name}: connection OK`)}
                  >
                    test
                  </button>
                  <button
                    className={ind.enabled ? "toggle on" : "toggle"}
                    disabled={busy}
                    onClick={() => toggle(ind)}
                  >
                    {ind.enabled ? "enabled" : "disabled"}
                  </button>
                  <button className="danger" disabled={busy} onClick={() => remove(ind)}>
                    remove
                  </button>
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div className="settings-form" style={{ marginTop: "0.75rem" }}>
        <label>
          Name
          <input value={draft.name} onChange={(e) => set({ name: e.target.value })} />
        </label>
        <label>
          Type
          <select
            value={draft.type}
            onChange={(e) => set({ type: e.target.value as Indexer["type"] })}
          >
            <option value="torznab">Torznab (torrents)</option>
            <option value="newznab">Newznab (usenet)</option>
          </select>
        </label>
        <label>
          URL
          <input
            placeholder="https://indexer.example (or a Prowlarr/Jackett feed URL)"
            value={draft.baseUrl}
            onChange={(e) => set({ baseUrl: e.target.value })}
          />
        </label>
        <label>
          API key
          <input value={draft.apiKey} onChange={(e) => set({ apiKey: e.target.value })} />
        </label>
        <button
          type="button"
          className="toggle"
          style={{ alignSelf: "flex-start" }}
          onClick={() => setAdvanced(!advanced)}
        >
          {advanced ? "▾ hide advanced" : "▸ advanced (per-type categories)"}
        </button>
        {advanced && (
          <>
            <label>
              Book categories
              <input
                title="Newznab categories used for ebook searches (7000 = Books, 7020 = Books/Ebook)"
                value={draft.categories}
                onChange={(e) => set({ categories: e.target.value })}
              />
            </label>
            <label>
              Audio categories
              <input
                title="Newznab categories used for audiobook searches (3030 = Audio/Audiobook)"
                value={draft.audioCategories}
                onChange={(e) => set({ audioCategories: e.target.value })}
              />
            </label>
            <label>
              Comic categories
              <input
                title="Newznab categories used for manga and comic searches (7030 = Books/Comics)"
                value={draft.comicCategories}
                onChange={(e) => set({ comicCategories: e.target.value })}
              />
            </label>
            <label>
              Magazine categories
              <input
                title="Newznab categories used for magazine searches (7010 = Books/Mags)"
                value={draft.magazineCategories}
                onChange={(e) => set({ magazineCategories: e.target.value })}
              />
            </label>
          </>
        )}
        <div className="settings-actions">
          <button disabled={busy || !draftValid} onClick={testDraft}>
            Test
          </button>
          <button disabled={busy || !draftValid} onClick={add}>
            Add
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

function NamingCard({
  onError,
  activeTypes,
}: {
  onError: (message: string) => void;
  activeTypes: string[];
}) {
  const show = (t: string) => activeTypes.length === 0 || activeTypes.includes(t);
  const [settings, setSettings] = useState<NamingSettings | null>(null);
  const [t, setT] = useState<Partial<NamingSettings>>({});
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    api
      .getNamingSettings()
      .then((s) => {
        setSettings(s);
        setT(s);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  if (!settings) return null;

  const save = () => {
    setBusy(true);
    setNotice("");
    api
      .saveNamingSettings(t)
      .then((s) => {
        setSettings(s);
        setT(s);
        setNotice("✓ Saved — use Organize on a library page to apply to existing files");
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const field = (label: string, key: keyof NamingSettings, title?: string) => (
    <label>
      {label}
      <input
        title={title}
        value={(t[key] as string) ?? ""}
        onChange={(e) => setT({ ...t, [key]: e.target.value })}
      />
    </label>
  );

  return (
    <section className="card">
      <h2>File Naming</h2>
      <p className="muted">
        How organized files are placed inside a root folder, per media type.
        Available tokens: {settings.tokens.map((tok, i) => (
          <span key={tok}>
            {i > 0 && " "}
            <code>{tok}</code>
          </span>
        ))}
        . Tokens without a value (e.g. series, for standalone books) drop out
        cleanly; emptied fields revert to the defaults.
      </p>
      <div className="settings-form">
        {show("ebook") && (
          <>
            {field("Ebook folder template", "ebookFolder")}
            {field("Ebook file template", "ebookFile")}
            <p className="muted">
              Example: <code>{settings.example}</code>
            </p>
          </>
        )}
        {show("audiobook") && (
          <>
            {field("Audiobook folder template", "audiobookFolder")}
            {field(
              "Audiobook book-folder template",
              "audiobookFile",
              "Names the per-book folder (Audiobookshelf layout); multi-file books keep their track names inside it",
            )}
            <p className="muted">
              Audiobook example: <code>{settings.audiobookExample}</code>
            </p>
          </>
        )}
        {show("manga") && (
          <>
            {field("Manga folder template", "mangaFolder")}
            {field("Manga file template", "mangaFile")}
          </>
        )}
        {show("comic") && (
          <>
            {field("Comic folder template", "comicFolder")}
            {field("Comic file template", "comicFile")}
          </>
        )}
        {show("magazine") && (
          <>
            {field("Magazine folder template", "magazineFolder")}
            {field("Magazine file template", "magazineFile")}
          </>
        )}
        <div className="settings-actions">
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

function MetadataCard({
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
  const [cacheNotice, setCacheNotice] = useState("");

  const runClear = (
    fn: () => Promise<{ removed?: number; freedBytes?: number; descriptionsCleared?: number }>,
  ) => {
    setCacheNotice("");
    fn()
      .then((r) => {
        const parts: string[] = [];
        if (r.removed !== undefined) {
          parts.push(`${r.removed} image(s) (${((r.freedBytes ?? 0) / (1 << 20)).toFixed(1)} MiB)`);
        }
        if (r.descriptionsCleared !== undefined) {
          parts.push(`${r.descriptionsCleared} description(s)`);
        }
        const total = (r.removed ?? 0) + (r.descriptionsCleared ?? 0);
        setCacheNotice(total === 0 ? "Nothing to clear." : `✓ Cleared ${parts.join(", ")}.`);
      })
      .catch((err: unknown) => setCacheNotice(`✗ ${err instanceof Error ? err.message : String(err)}`));
  };

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

        <p className="muted">
          Manga metadata comes from AniList (no key needed). Comics need a
          free <a href="https://comicvine.gamespot.com/api/" target="_blank" rel="noreferrer">ComicVine API key</a>:
        </p>
        <label>
          ComicVine API key
          <input
            type="password"
            placeholder="Required for comic search"
            value={providers["comicvine"]?.token ?? ""}
            onChange={(e) => {
              setProviders({
                ...providers,
                comicvine: { ...(providers["comicvine"] ?? {}), token: e.target.value },
              });
              setNotice("");
            }}
          />
        </label>

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

      <div className="settings-form" style={{ marginTop: "1.25rem" }}>
        <p className="muted" style={{ margin: 0 }}>
          Cached metadata rebuilds on demand. <strong>Provider art</strong>
          {" "}(author portraits, cover images) and <strong>extracted
          covers</strong> (the first page of your owned manga/comic archives)
          live under the data directory and re-fetch as you browse.{" "}
          <strong>Descriptions</strong> are stored in the database and only
          return on the next metadata refresh (per author/series, or the daily
          sync).
        </p>
        <div className="settings-actions">
          <button className="danger" onClick={() => runClear(api.clearMetadataCache)}>
            Clear provider art
          </button>
          <button className="danger" onClick={() => runClear(api.clearCoverCache)}>
            Clear extracted covers
          </button>
          <button
            className="danger"
            onClick={() => {
              if (confirm("Clear all stored descriptions?\n\nThey stay blank until a metadata refresh re-fetches them.")) {
                runClear(api.clearDescriptions);
              }
            }}
          >
            Clear descriptions
          </button>
          <button
            className="danger"
            onClick={() => {
              if (confirm("Clear ALL caches — provider art, extracted covers, and descriptions?\n\nImages re-fetch as you browse; descriptions return on the next metadata refresh.")) {
                runClear(api.clearAllCache);
              }
            }}
          >
            Clear all
          </button>
          {cacheNotice && (
            <span className={cacheNotice.startsWith("✗") ? "notice bad" : "notice ok"}>
              {cacheNotice}
            </span>
          )}
        </div>
      </div>
    </section>
  );
}

const mediaTypes = ["ebook", "audiobook", "manga", "comic", "magazine"] as const;

function RootFoldersCard({
  onError,
  onChanged,
}: {
  onError: (message: string) => void;
  onChanged?: () => void;
}) {
  const [folders, setFolders] = useState<RootFolder[]>([]);
  const [mediaType, setMediaType] = useState<string>("ebook");
  const [variant, setVariant] = useState<string>("mono");
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .listRootFolders()
      .then(setFolders)
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const add = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = path.trim();
    if (!trimmed) return;
    setBusy(true);
    setNotice("");
    api
      .addRootFolder(mediaType, trimmed, mediaType === "manga" ? variant : undefined)
      .then(() => {
        setPath("");
        reload();
        onChanged?.();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const variantLabel = (v?: string) =>
    v === "color" ? "colorized" : v === "mono" ? "monochrome" : "";

  const remove = (f: RootFolder) => {
    if (!confirm(`Remove root folder ${f.path}? Files on disk are not touched.`)) return;
    api
      .deleteRootFolder(f.id)
      .then(() => {
        reload();
        onChanged?.();
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  };

  return (
    <section className="card">
      <h2>Root Folders</h2>
      <p className="muted">
        Where your libraries live on disk. The scanner walks these to match
        files you already own; note the path must exist on the machine running
        LibriNode (in WSL, Windows drives are under <code>/mnt/c/…</code>).
        Manga stays one library, but you add a <strong>separate root per
        variant</strong> — colorized and monochrome — so each volume can own
        one, the other, or both.
      </p>

      {folders.length > 0 && (
        <ul className="rows">
          {folders.map((f) => (
            <li key={f.id}>
              <div className="row">
                <span className="file-path">
                  {f.path}
                  {!f.accessible && <span className="notice bad"> (not accessible)</span>}
                </span>
                <span className="row-actions">
                  <span className="muted">
                    {f.mediaType}
                    {f.variant && ` · ${variantLabel(f.variant)}`}
                  </span>
                  <button className="danger" onClick={() => remove(f)}>
                    remove
                  </button>
                </span>
              </div>
            </li>
          ))}
        </ul>
      )}

      <form onSubmit={add} className="search-form" style={{ marginTop: "0.75rem" }}>
        <select value={mediaType} onChange={(e) => setMediaType(e.target.value)}>
          {mediaTypes.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
        {mediaType === "manga" && (
          <select value={variant} onChange={(e) => setVariant(e.target.value)}>
            <option value="mono">monochrome</option>
            <option value="color">colorized</option>
          </select>
        )}
        <input
          placeholder="/data/ebooks or /mnt/c/Users/…/Ebooks"
          value={path}
          onChange={(e) => setPath(e.target.value)}
        />
        <button type="submit" disabled={busy}>
          Add
        </button>
      </form>
      {notice && <p className="notice bad">{notice}</p>}
    </section>
  );
}
