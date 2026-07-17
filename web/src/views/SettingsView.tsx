import { type ReactNode, useCallback, useEffect, useState } from "react";
import FolderBrowser from "../components/FolderBrowser";
import {
  api,
  getApiKey,
  setApiKey,
  type AuthStatus,
  type DownloadClient,
  type ImportSettings,
  type Indexer,
  type MetadataSettings,
  type NamingSettings,
  type ProviderSettings,
  type QualityProfile,
  type RootFolder,
  type SystemStatus,
  type UserAccount,
} from "../api";
import { formatBytes } from "../format";
import { useUi } from "../ui";

// Settings groups, *arr-style: pages organized by concern instead of one
// long scroll. Order matches the README spec.
const settingsGroups = [
  "Media Management",
  "Quality Profiles",
  "Metadata",
  "Indexers",
  "Download Clients",
  "General",
] as const;
type SettingsGroup = (typeof settingsGroups)[number];

// --- Shared layout primitives (visual polish; no behavior of their own) ---

// Section groups related fields inside a card under a small heading, with
// optional help text — so a long form reads as a few labelled blocks.
function Section({
  title,
  help,
  children,
}: {
  title: string;
  help?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="settings-section">
      <h3>{title}</h3>
      {help != null && <p className="muted">{help}</p>}
      {children}
    </div>
  );
}

// Disclosure hides advanced/optional fields behind a native <details> toggle,
// collapsed by default, so the common path stays uncluttered.
function Disclosure({
  summary,
  defaultOpen,
  children,
}: {
  summary: string;
  defaultOpen?: boolean;
  children: ReactNode;
}) {
  return (
    <details className="disclosure" open={defaultOpen}>
      <summary>{summary}</summary>
      <div className="disclosure-body">{children}</div>
    </details>
  );
}

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
      {group === "Quality Profiles" && (
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
  const { confirmDlg } = useUi();
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

  const regenerate = async () => {
    const ok = await confirmDlg({
      title: "Regenerate API key",
      message:
        "Regenerate the API key?\n\nProwlarr and any scripts using the current key stop working until you update them.",
      confirmLabel: "Regenerate",
      danger: true,
    });
    if (!ok) return;
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

// SecurityCard manages the login accounts: a compact user list with per-user
// actions (change password, make default, remove) plus add-user and
// disable-login. The default user is protected — promote another user first.
// The API key keeps working for scripts and Prowlarr either way.
function SecurityCard({ onError }: { onError: (message: string) => void }) {
  const { confirmDlg } = useUi();
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [users, setUsers] = useState<UserAccount[]>([]);
  // One inline form open at a time: adding a user, or changing one password.
  const [form, setForm] = useState<"" | "add" | `pw:${string}`>("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");

  const reload = useCallback(() => {
    api
      .authStatus()
      .then((s) => {
        setStatus(s);
        return s.authEnabled ? api.listUsers().then((r) => setUsers(r.users)) : setUsers([]);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  useEffect(reload, [reload]);

  const openForm = (f: "" | "add" | `pw:${string}`) => {
    setForm(f);
    setUsername("");
    setPassword("");
    setConfirmPw("");
    setNotice("");
  };

  const run = (action: () => Promise<unknown>, done?: string) => {
    setBusy(true);
    setNotice("");
    action()
      .then(() => {
        openForm("");
        if (done) setNotice(done); // after openForm — it clears the notice
        reload();
      })
      .catch((err: unknown) =>
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`),
      )
      .finally(() => setBusy(false));
  };

  const passwordsOK = password.length >= 8 && password === confirmPw;
  const pwHint =
    password && password.length < 8
      ? "min. 8 characters"
      : confirmPw && password !== confirmPw
        ? "passwords don't match"
        : "";

  const submitForm = () => {
    if (form === "add") {
      // The very first account goes through the credentials endpoint, which
      // also signs this browser in before the login requirement kicks in.
      run(
        () =>
          users.length === 0
            ? api.setCredentials(username.trim(), password)
            : api.addUser(username.trim(), password),
        users.length === 0
          ? "✓ Login required from now on — this browser is already signed in"
          : `✓ Added ${username.trim()}`,
      );
    } else if (form.startsWith("pw:")) {
      const user = form.slice(3);
      run(() => api.setUserPassword(user, password), `✓ Password changed for ${user}`);
    }
  };

  const remove = async (u: UserAccount) => {
    const ok = await confirmDlg({
      title: "Remove user",
      message: `Remove user "${u.username}"? Their sessions keep working until the next restart.`,
      confirmLabel: "Remove user",
      danger: true,
    });
    if (ok) run(() => api.removeUser(u.username), `✓ Removed ${u.username}`);
  };

  const disable = async () => {
    const ok = await confirmDlg({
      title: "Disable login",
      message:
        "Disable the login requirement? All users are removed and the UI goes back to the API-key prompt.",
      confirmLabel: "Disable login",
      danger: true,
    });
    if (ok) run(() => api.setCredentials("", ""), "✓ Login disabled");
  };

  // The inline username/password form (add user or change password).
  const credentialForm = (title: string, withUsername: boolean) => (
    <div className="settings-form user-form">
      {withUsername && (
        <label>
          Username
          <input autoFocus value={username} onChange={(e) => setUsername(e.target.value)} />
        </label>
      )}
      <label>
        Password
        <input
          type="password"
          autoFocus={!withUsername}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </label>
      <label>
        Confirm password
        <input type="password" value={confirmPw} onChange={(e) => setConfirmPw(e.target.value)} />
      </label>
      <div className="settings-actions">
        <button
          disabled={busy || (withUsername && !username.trim()) || !passwordsOK}
          onClick={submitForm}
        >
          {title}
        </button>
        <button className="toggle" onClick={() => openForm("")}>
          Cancel
        </button>
        {pwHint && <span className="muted">{pwHint}</span>}
      </div>
    </div>
  );

  return (
    <section className="card">
      <h2>Security</h2>
      <p className="muted">
        {status?.authEnabled
          ? "Signing in is required. The default user is protected — make another user the default before removing it. The API key keeps working for Prowlarr and scripts."
          : "No login account yet — the UI asks for the raw API key. Add a user to switch to a login page (sessions last 30 days; a restart signs everyone out)."}
      </p>

      {users.length > 0 && (
        <ul className="rows">
          {users.map((u) => (
            <li key={u.username}>
              <div className="row">
                <span>
                  👤 {u.username}
                  {u.default && (
                    <span className="pill user-default" title="Protected — cannot be removed">
                      default
                    </span>
                  )}
                </span>
                <span className="row-actions">
                  <button
                    className="toggle"
                    disabled={busy}
                    onClick={() => openForm(`pw:${u.username}`)}
                  >
                    change password
                  </button>
                  {!u.default && (
                    <>
                      <button
                        className="toggle"
                        disabled={busy}
                        title="Make this the protected primary account"
                        onClick={() => run(() => api.makeDefaultUser(u.username), `✓ ${u.username} is now the default`)}
                      >
                        make default
                      </button>
                      <button className="danger" disabled={busy} onClick={() => remove(u)}>
                        remove
                      </button>
                    </>
                  )}
                </span>
              </div>
              {form === `pw:${u.username}` && credentialForm("Change password", false)}
            </li>
          ))}
        </ul>
      )}

      {form === "add" && credentialForm(users.length === 0 ? "Enable login" : "Add user", true)}

      <div className="settings-actions" style={{ marginTop: "0.6rem" }}>
        {form !== "add" && (
          <button disabled={busy} onClick={() => openForm("add")}>
            + Add user
          </button>
        )}
        {status?.authEnabled && (
          <button className="danger" disabled={busy} onClick={disable}>
            Disable login
          </button>
        )}
        {notice && (
          <span className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>{notice}</span>
        )}
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
  const { confirmDlg } = useUi();
  const [clients, setClients] = useState<DownloadClient[]>([]);
  const [draft, setDraft] = useState(emptyDownloadClient);
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

  // The SABnzbd API key is optional — SABnzbd-compatible endpoints like
  // Real-Debrid's need no key (real SABnzbd rejects unauthenticated calls,
  // which the Test button surfaces).
  const draftValid =
    draft.name.trim() !== "" && /^https?:\/\//.test(draft.host.trim());

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
                    onClick={async () => {
                      if (
                        await confirmDlg({
                          message: `Remove download client ${c.name}?`,
                          confirmLabel: "Remove",
                          danger: true,
                        })
                      ) {
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

      <h3 className="settings-subhead">{clients.length > 0 ? "Add another client" : "Add a download client"}</h3>
      <div className="settings-form">
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
            <input
              placeholder="Optional — leave blank for Real-Debrid / keyless SABnzbd endpoints"
              value={draft.apiKey}
              onChange={(e) => set({ apiKey: e.target.value })}
            />
          </label>
        )}
        <Disclosure summary="Advanced">
          <label>
            Category
            <input
              value={draft.category}
              onChange={(e) => set({ category: e.target.value })}
            />
          </label>
          <p className="muted" style={{ margin: 0, fontSize: "0.82rem" }}>
            Downloads are tagged with this category so LibriNode only tracks its
            own — change it only if it collides with another app on the same
            client.
          </p>
        </Disclosure>
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

      <ImportOptions onError={onError} />
    </section>
  );
}

// ImportOptions: Completed Download Handling knobs (saved on toggle).
const DEFAULT_IMPORT_SETTINGS: ImportSettings = {
  packImportAll: true,
  removeCompleted: true,
  deleteCompletedFiles: true,
};

function ImportOptions({ onError }: { onError: (message: string) => void }) {
  const [settings, setSettings] = useState<ImportSettings>(DEFAULT_IMPORT_SETTINGS);
  const [loaded, setLoaded] = useState(false);
  const [notice, setNotice] = useState("");

  useEffect(() => {
    api
      .getImportSettings()
      .then((s) => {
        setSettings(s);
        setLoaded(true);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  if (!loaded) return null;

  const update = (patch: Partial<ImportSettings>) => {
    const next = { ...settings, ...patch };
    // Deleting the files necessarily removes the download from the client.
    if (next.deleteCompletedFiles) next.removeCompleted = true;
    const prev = settings;
    setSettings(next);
    setNotice("");
    api
      .saveImportSettings(next)
      .then((s) => {
        setSettings(s);
        setNotice("✓ Saved");
      })
      .catch((err: unknown) => {
        setSettings(prev);
        setNotice(`✗ ${err instanceof Error ? err.message : String(err)}`);
      });
  };

  return (
    <div style={{ marginTop: "1.25rem" }}>
      <Section
        title="Import handling"
        help="How LibriNode imports finished downloads and tidies up afterwards. All on by default — changes save immediately."
      >
        <div className="opt">
          <label className="check">
            <span>
              <input
                type="checkbox"
                checked={settings.packImportAll}
                onChange={(e) => update({ packImportAll: e.target.checked })}
              />{" "}
              Import whole packs
            </span>
          </label>
          <p className="muted opt-help">
            On (default): a multi-book pack fills every book it matches. Off: a
            pack only fills the grabbed book plus other{" "}
            <strong>monitored</strong> books. Either way, a book that already
            owns the format is only replaced by a genuine quality upgrade, and
            nothing gets monitored automatically.
          </p>
        </div>

        <div className="opt">
          <label className="check">
            <span>
              <input
                type="checkbox"
                checked={settings.removeCompleted}
                onChange={(e) => update({ removeCompleted: e.target.checked })}
              />{" "}
              Remove completed downloads from the client after import
            </span>
          </label>
          <p className="muted opt-help">
            On (default): the download is removed from the client — for torrents
            too — once LibriNode has imported it. Off: usenet history entries are
            cleared either way (the file stays), and torrents keep seeding until
            the client's own goal is met.
          </p>
        </div>

        <div className="opt">
          <label className="check">
            <span>
              <input
                type="checkbox"
                checked={settings.deleteCompletedFiles}
                onChange={(e) => update({ deleteCompletedFiles: e.target.checked })}
              />{" "}
              Delete the downloaded files after import
            </span>
          </label>
          <p className="muted opt-help">
            On (default): LibriNode copies imported files into the library, then
            deletes the originals (this also removes the download from the
            client). Turn off if the download folder is shared with other apps.
          </p>
        </div>

        {notice && (
          <span className={notice.startsWith("✗") ? "notice bad" : "notice ok"}>
            {notice}
          </span>
        )}
      </Section>
    </div>
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

      <h3 className="settings-subhead">Add a quality profile</h3>
      <div className="settings-form">
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
  const { confirmDlg } = useUi();
  const [indexers, setIndexers] = useState<Indexer[]>([]);
  const [draft, setDraft] = useState(emptyIndexer);
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

  const remove = async (ind: Indexer) => {
    const ok = await confirmDlg({
      message: `Remove indexer ${ind.name}?`,
      confirmLabel: "Remove",
      danger: true,
    });
    if (ok) run(() => api.deleteIndexer(ind.id));
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

      <h3 className="settings-subhead">{indexers.length > 0 ? "Add another indexer" : "Add an indexer"}</h3>
      <div className="settings-form">
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
        <Disclosure summary="Advanced — per-type categories">
          <p className="muted" style={{ margin: 0, fontSize: "0.82rem" }}>
            Newznab/Torznab category IDs searched per media type. Defaults cover
            the standard book categories; change only for an unusual indexer.
          </p>
          <label>
            Book categories
            <input
              title="7000 = Books, 7020 = Books/Ebook"
              value={draft.categories}
              onChange={(e) => set({ categories: e.target.value })}
            />
          </label>
          <label>
            Audio categories
            <input
              title="3030 = Audio/Audiobook"
              value={draft.audioCategories}
              onChange={(e) => set({ audioCategories: e.target.value })}
            />
          </label>
          <label>
            Comic categories
            <input
              title="7030 = Books/Comics (manga and comics)"
              value={draft.comicCategories}
              onChange={(e) => set({ comicCategories: e.target.value })}
            />
          </label>
          <label>
            Magazine categories
            <input
              title="7010 = Books/Mags"
              value={draft.magazineCategories}
              onChange={(e) => set({ magazineCategories: e.target.value })}
            />
          </label>
        </Disclosure>
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
          <Section title="Ebooks">
            {field("Folder template", "ebookFolder")}
            {field("File template", "ebookFile")}
            <p className="muted" style={{ margin: 0, fontSize: "0.82rem" }}>
              Example: <code>{settings.example}</code>
            </p>
          </Section>
        )}
        {show("audiobook") && (
          <Section title="Audiobooks">
            {field("Folder template", "audiobookFolder")}
            {field(
              "Book-folder template",
              "audiobookFile",
              "Names the per-book folder (Audiobookshelf layout); multi-file books keep their track names inside it",
            )}
            <p className="muted" style={{ margin: 0, fontSize: "0.82rem" }}>
              Example: <code>{settings.audiobookExample}</code>
            </p>
          </Section>
        )}
        {show("manga") && (
          <Section title="Manga">
            {field("Folder template", "mangaFolder")}
            {field("File template", "mangaFile")}
          </Section>
        )}
        {show("comic") && (
          <Section title="Comics">
            {field("Folder template", "comicFolder")}
            {field("File template", "comicFile")}
          </Section>
        )}
        {show("magazine") && (
          <Section title="Magazines">
            {field("Folder template", "magazineFolder")}
            {field("File template", "magazineFile")}
          </Section>
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

// Global metadata preference options (values stored lower-case; providers
// match them case-insensitively against their own data).
const LANGUAGES = [
  "english", "spanish", "french", "german", "italian", "portuguese",
  "dutch", "polish", "russian", "japanese", "chinese", "korean",
];
const COUNTRIES = [
  "united states", "united kingdom", "canada", "australia", "germany",
  "france", "spain", "italy", "brazil", "netherlands", "poland", "japan",
];

function MetadataCard({
  onError,
}: {
  onError: (message: string) => void;
}) {
  const { confirmDlg } = useUi();
  const [settings, setSettings] = useState<MetadataSettings | null>(null);
  const [active, setActive] = useState("");
  const [providers, setProviders] = useState<Record<string, ProviderSettings>>({});
  const [showToken, setShowToken] = useState(false);
  const [mangaProvider, setMangaProvider] = useState("");
  const [comicProvider, setComicProvider] = useState("");
  const [mangaCoverSource, setMangaCoverSource] = useState("provider");
  const [comicCoverSource, setComicCoverSource] = useState("provider");
  const [language, setLanguage] = useState("english");
  const [country, setCountry] = useState("united states");
  const [includeAdult, setIncludeAdult] = useState(false);
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
          parts.push(`${r.removed} image(s) (${formatBytes(r.freedBytes ?? 0) || "0 KiB"})`);
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
        setMangaProvider(s.mangaProvider);
        setComicProvider(s.comicProvider);
        setMangaCoverSource(s.mangaCoverSource);
        setComicCoverSource(s.comicCoverSource);
        setLanguage(s.language);
        setCountry(s.country);
        setIncludeAdult(s.includeAdult);
      })
      .catch((err: unknown) => onError(String(err instanceof Error ? err.message : err)));
  }, [onError]);

  if (!settings) return <p className="muted">Loading…</p>;

  const hardcoverSettings = providers["hardcover"] ?? { token: "" };

  const setProviderToken = (name: string, token: string) => {
    setProviders({ ...providers, [name]: { ...(providers[name] ?? {}), token } });
    setNotice("");
  };

  const test = () => {
    setBusy(true);
    setNotice("");
    api
      .testMetadataProvider("hardcover", hardcoverSettings)
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
      .saveMetadataSettings(active, providers, {
        mangaProvider,
        comicProvider,
        mangaCoverSource,
        comicCoverSource,
        language,
        country,
        includeAdult,
      })
      .then((s) => {
        setSettings(s);
        setActive(s.active);
        setProviders(s.providers);
        setMangaProvider(s.mangaProvider);
        setComicProvider(s.comicProvider);
        setMangaCoverSource(s.mangaCoverSource);
        setComicCoverSource(s.comicCoverSource);
        setLanguage(s.language);
        setCountry(s.country);
        setIncludeAdult(s.includeAdult);
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
      <h2>Metadata</h2>
      <p className="muted">
        Where each library's metadata (authors, series, volumes, covers,
        descriptions) comes from. Providers are pluggable — more sources can
        be added without touching the rest of the app.
      </p>

      <div className="settings-form">
        <div className="settings-section">
          <h3>API keys</h3>
          <label>
            Hardcover API token
            <span className="token-row">
              <input
                type={showToken ? "text" : "password"}
                placeholder="Token from hardcover.app/account/api"
                value={hardcoverSettings.token}
                onChange={(e) => setProviderToken("hardcover", e.target.value)}
              />
              <button
                type="button"
                className="toggle"
                onClick={() => setShowToken(!showToken)}
              >
                {showToken ? "hide" : "show"}
              </button>
              <button
                type="button"
                disabled={busy || !hardcoverSettings.token}
                onClick={test}
              >
                Test
              </button>
            </span>
          </label>
          <label>
            ComicVine API key
            <input
              type="password"
              placeholder="Required for comic search (free key)"
              value={providers["comicvine"]?.token ?? ""}
              onChange={(e) => setProviderToken("comicvine", e.target.value)}
            />
          </label>
          <p className="muted">
            Hardcover tokens come from{" "}
            <a href="https://hardcover.app/account/api" target="_blank" rel="noreferrer">hardcover.app/account/api</a>;
            ComicVine keys from{" "}
            <a href="https://comicvine.gamespot.com/api/" target="_blank" rel="noreferrer">comicvine.gamespot.com/api</a>.
            AniList needs no key.
          </p>
        </div>

        <div className="settings-section">
          <h3>Preferences</h3>
          <p className="muted">
            Global metadata preferences, honored by every provider that
            carries the data (with graceful fallback when nothing matches).
            They shape metadata only — what to grab is the quality profiles'
            job.
          </p>
          <label>
            Language
            <select
              value={language}
              onChange={(e) => {
                setLanguage(e.target.value);
                setNotice("");
              }}
            >
              {LANGUAGES.map((l) => (
                <option key={l} value={l}>
                  {l[0].toUpperCase() + l.slice(1)}
                </option>
              ))}
              <option value="none">No preference</option>
            </select>
          </label>
          <label>
            Country
            <select
              value={country}
              onChange={(e) => {
                setCountry(e.target.value);
                setNotice("");
              }}
            >
              {COUNTRIES.map((c) => (
                <option key={c} value={c}>
                  {c.replace(/\b\w/g, (ch) => ch.toUpperCase())}
                </option>
              ))}
              <option value="none">No preference</option>
            </select>
          </label>
          <label className="check">
            <span>
              <input
                type="checkbox"
                checked={includeAdult}
                onChange={(e) => {
                  setIncludeAdult(e.target.checked);
                  setNotice("");
                }}
              />{" "}
              Include adult content in metadata search results
            </span>
          </label>
        </div>

        <div className="settings-section">
          <h3>Ebooks &amp; Audiobooks</h3>
          <label>
            Book provider
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
        </div>

        <div className="settings-section">
          <h3>Manga</h3>
          <label>
            Manga provider
            <select
              value={mangaProvider || settings.mangaProviders[0] || "anilist"}
              onChange={(e) => {
                setMangaProvider(e.target.value);
                setNotice("");
              }}
            >
              {settings.mangaProviders.map((name) => (
                <option key={name} value={name}>
                  {name[0].toUpperCase() + name.slice(1)}
                  {name === "anilist" ? " (no key)" : ""}
                  {name === "hardcover" ? " (uses your Hardcover token)" : ""}
                </option>
              ))}
              <option value="none">None (disabled)</option>
            </select>
          </label>
          <label>
            Volume covers
            <select
              value={mangaCoverSource}
              onChange={(e) => {
                setMangaCoverSource(e.target.value);
                setNotice("");
              }}
            >
              <option value="provider">Use the provider's cover art</option>
              <option value="file">Extract from the owned file (first page)</option>
            </select>
          </label>
        </div>

        <div className="settings-section">
          <h3>Comics</h3>
          <label>
            Comic provider
            <select
              value={comicProvider || settings.comicProviders[0] || "hardcover"}
              onChange={(e) => {
                setComicProvider(e.target.value);
                setNotice("");
              }}
            >
              {settings.comicProviders.map((name) => (
                <option key={name} value={name}>
                  {name[0].toUpperCase() + name.slice(1)}
                  {name === "comicvine" ? " (key above)" : ""}
                  {name === "hardcover" ? " (uses your Hardcover token)" : ""}
                </option>
              ))}
              <option value="none">None (disabled)</option>
            </select>
          </label>
          <label>
            Issue covers
            <select
              value={comicCoverSource}
              onChange={(e) => {
                setComicCoverSource(e.target.value);
                setNotice("");
              }}
            >
              <option value="provider">Use the provider's cover art</option>
              <option value="file">Extract from the owned file (first page)</option>
            </select>
          </label>
        </div>

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

      <div style={{ marginTop: "1.25rem" }}>
        <Disclosure summary="Cache maintenance">
        <p className="muted" style={{ margin: 0, fontSize: "0.85rem" }}>
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
            onClick={async () => {
              if (
                await confirmDlg({
                  title: "Clear descriptions",
                  message:
                    "Clear all stored descriptions?\n\nThey stay blank until a metadata refresh re-fetches them.",
                  confirmLabel: "Clear",
                  danger: true,
                })
              ) {
                runClear(api.clearDescriptions);
              }
            }}
          >
            Clear descriptions
          </button>
          <button
            className="danger"
            onClick={async () => {
              if (
                await confirmDlg({
                  title: "Clear all caches",
                  message:
                    "Clear ALL caches — provider art, extracted covers, and descriptions?\n\nImages re-fetch as you browse; descriptions return on the next metadata refresh.",
                  confirmLabel: "Clear everything",
                  danger: true,
                })
              ) {
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
        </Disclosure>
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
  const { confirmDlg } = useUi();
  const [folders, setFolders] = useState<RootFolder[]>([]);
  const [mediaType, setMediaType] = useState<string>("ebook");
  const [variant, setVariant] = useState<string>("mono");
  const [path, setPath] = useState("");
  const [browsing, setBrowsing] = useState(false);
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

  const remove = async (f: RootFolder) => {
    const ok = await confirmDlg({
      title: "Remove root folder",
      message: `Remove root folder ${f.path}? Files on disk are not touched.`,
      confirmLabel: "Remove folder",
      danger: true,
    });
    if (!ok) return;
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

      <h3 className="settings-subhead">Add a root folder</h3>
      <form onSubmit={add} className="search-form">
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
        <button
          type="button"
          className="toggle"
          onClick={() => setBrowsing(!browsing)}
          title="Pick the folder visually on the server's filesystem"
        >
          {browsing ? "Hide browser" : "Browse…"}
        </button>
        <button type="submit" disabled={busy}>
          Add
        </button>
      </form>
      {browsing && (
        <FolderBrowser
          initial={path}
          onPick={(p) => {
            setPath(p);
            setBrowsing(false);
          }}
          onClose={() => setBrowsing(false)}
        />
      )}
      {notice && <p className="notice bad">{notice}</p>}
    </section>
  );
}
