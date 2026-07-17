// Typed client for LibriNode's REST API. The API key is kept in localStorage
// for now; proper session handling comes with the settings UI.

export interface SystemStatus {
  appName: string;
  version: string;
  appVersion?: string;
  os: string;
  arch: string;
  uptime: string;
  dataDir: string;
  startTime: string;
  ipAddresses: string[];
  port: number;
}

export interface Author {
  id: number;
  metadataSource: string;
  foreignAuthorId: string;
  name: string;
  sortName: string;
  description: string;
  imageUrl: string;
  monitored: boolean;
  inEbookLibrary: boolean;
  inAudiobookLibrary: boolean;
  providerOverride: string;
  bookCount?: number;
  ownedCount: number;
  books?: Book[];
}

export interface Book {
  id: number;
  authorId: number;
  foreignBookId: string;
  // "book" for prose; "manga"/"comic" volumes and "magazine" issues carry
  // their series' type.
  mediaType: string;
  title: string;
  sortTitle: string;
  description: string;
  releaseDate: string;
  rating: number;
  coverUrl: string;
  monitored: boolean;
  inEbookLibrary: boolean;
  ebookMonitored: boolean;
  inAudiobookLibrary: boolean;
  audiobookMonitored: boolean;
  hasFile: boolean;
  hasEbookFile: boolean;
  hasAudiobookFile: boolean;
  hasColorFile: boolean;
  hasMonoFile: boolean;
  editions?: Edition[];
  series?: SeriesLink[];
  files?: BookFile[];
}

export interface Edition {
  id: number;
  bookId: number;
  foreignEditionId: string;
  title: string;
  isbn13: string;
  asin: string;
  format: string;
  publisher: string;
  language: string;
  releaseDate: string;
  coverUrl: string;
  monitored: boolean;
}

export interface SeriesLink {
  seriesId: number;
  title: string;
  position: number;
}

export interface BookFile {
  id: number;
  rootFolderId: number;
  bookId?: number;
  mediaType: string;
  variant?: string;
  path: string;
  size: number;
  format: string;
  modifiedAt: string;
  addedAt: string;
  // Multi-file audiobook units list their audio files (relative to the folder).
  tracks?: { name: string; size: number }[];
}

export interface RootFolder {
  id: number;
  mediaType: string;
  variant?: string;
  path: string;
  accessible: boolean;
}

export interface ScanResult {
  roots: number;
  scanned: number;
  matched: number;
  unmatched: number;
  removed: number;
  errors?: string[];
}

export interface Indexer {
  id: number;
  name: string;
  type: "newznab" | "torznab";
  baseUrl: string;
  apiKey: string;
  categories: string;
  audioCategories: string;
  comicCategories: string;
  magazineCategories: string;
  enabled: boolean;
  priority: number;
  addedAt?: string;
}

export interface DownloadClient {
  id: number;
  name: string;
  type: "qbittorrent" | "sabnzbd";
  host: string;
  username: string;
  password: string;
  apiKey: string;
  category: string;
  enabled: boolean;
  priority: number;
}

export interface QueueItem {
  client: string;
  clientConfigId: number;
  id: string;
  title: string;
  status: string;
  progress: number;
  path?: string;
  // Set when the item belongs to a tracked grab — links it to its book.
  grabId?: number;
  bookId?: number;
  mediaType?: string;
}

export interface SeriesResult {
  foreignSeriesId: string;
  title: string;
  description?: string;
  authorName?: string;
  year?: number;
  coverUrl?: string;
  issueCount: number;
}

export interface Series {
  id: number;
  metadataSource: string;
  foreignSeriesId: string;
  title: string;
  description: string;
  mediaType: string;
  monitored: boolean;
  monitorNew: boolean;
  providerOverride: string;
  coverUrl: string;
  itemCount: number;
  ownedCount: number;
  volumes?: Book[];
}

export interface Release {
  indexerId: number;
  indexer: string;
  protocol: "usenet" | "torrent";
  title: string;
  guid: string;
  infoUrl?: string;
  downloadUrl: string;
  size: number;
  publishDate?: string;
  seeders: number;
  peers: number;
}

export interface ReleaseCandidate extends Release {
  parsed: {
    author?: string;
    title?: string;
    year?: number;
    formats?: string[];
    language?: string;
    retail: boolean;
  };
  score: number;
  approved: boolean;
  rejections?: string[];
}

export interface SearchOutcome {
  bookId: number;
  bookTitle: string;
  grabbed: boolean;
  release?: string;
  client?: string;
  message?: string;
}

export interface GrabRecord {
  id: number;
  bookId?: number;
  title: string;
  protocol: string;
  status: "grabbed" | "imported" | "failed";
  message?: string;
  grabbedAt: string;
  completedAt?: string;
}

export interface AuthStatus {
  authEnabled: boolean;
  authenticated: boolean;
}

export interface BackupInfo {
  name: string;
  size: number;
  createdAt: string;
}

export interface HealthIssue {
  source: string;
  level: "error" | "warning";
  message: string;
}

export interface HealthResult {
  issues: HealthIssue[];
  checkedAt: string; // zero time before the first background run
}

export interface LibraryStatus {
  mediaType: string;
  active: boolean;
  items: number;
  wanted: number;
}

export interface HomeItem {
  bookId: number;
  authorId?: number;
  seriesId?: number;
  title: string;
  subtitle?: string;
  coverUrl?: string;
  hasFile: boolean;
}

export interface CalendarItem {
  bookId: number;
  title: string;
  subtitle?: string;
  mediaType: string;
  releaseDate: string;
  owned: boolean;
}

export interface HomeSection {
  mediaType: string;
  items: number;
  wantedCount: number;
  recentlyAdded: HomeItem[];
  wanted: HomeItem[];
}

export interface ImportResult {
  imported: number;
  failed: number;
  skipped: number;
  messages?: string[];
}

export interface QualityProfile {
  id: number;
  name: string;
  mediaType: string;
  formats: string[];
  language: string;
  retailBonus: number;
  minSize: number;
  maxSize: number;
  upgradesAllowed: boolean;
  cutoff?: string;
  isDefault: boolean;
}

export interface BlockEntry {
  id: number;
  guid?: string;
  title: string;
  reason?: string;
  blockedAt: string;
}

export interface NamingSettings {
  ebookFolder: string;
  ebookFile: string;
  audiobookFolder: string;
  audiobookFile: string;
  mangaFolder: string;
  mangaFile: string;
  comicFolder: string;
  comicFile: string;
  magazineFolder: string;
  magazineFile: string;
  tokens: string[];
  example: string;
  audiobookExample: string;
}

export interface RenameMove {
  fileId: number;
  bookId: number;
  bookTitle: string;
  from: string;
  to: string;
}

export interface RenameResult {
  moves: RenameMove[];
  skips: string[];
}

// Metadata provider search results (not yet in the library).
export interface SearchAuthor {
  foreignAuthorId: string;
  name: string;
  imageUrl: string;
  bookCount?: number;
}

export interface SearchBook {
  foreignBookId: string;
  title: string;
  authorName: string;
  releaseDate: string;
  rating: number;
  coverUrl: string;
}

export interface ProviderSettings {
  token: string;
}

export interface MetadataSettings {
  active: string;
  available: string[];
  providers: Record<string, ProviderSettings>;
  mangaProviders: string[];
  mangaProvider: string;
  comicProviders: string[];
  comicProvider: string;
  mangaCoverSource: string;
  comicCoverSource: string;
  language: string;
  country: string;
  includeAdult: boolean;
}

export interface ImportSettings {
  packImportAll: boolean;
  removeCompleted: boolean;
  deleteCompletedFiles: boolean;
}

// UserAccount is one login; the default user is protected from removal.
export interface UserAccount {
  username: string;
  default: boolean;
}

// UnmatchedOption is an unmatched file plus its existing-file import choices:
// the parsed author, a confident suggestion when the filename singles out one
// book, and the author's importable books (owned-in-format excluded).
export interface UnmatchedOption {
  file: BookFile;
  authorName?: string;
  authorId?: number;
  // Series-first libraries: the parsed series/magazine and volume/issue.
  seriesName?: string;
  seriesId?: number;
  volume?: number;
  issue?: string;
  suggested?: number;
  confident: boolean;
  confidence: number; // 0–100
  candidates: { id: number; title: string; year?: string }[];
  // Set when this file duplicates a book already owned in the library.
  duplicate?: {
    bookId: number;
    title: string;
    year?: string;
    file: BookFile; // the library's current copy
    confidence: number;
  };
}

// FolderListing is one level of the server's filesystem for the folder picker.
export interface FolderListing {
  path: string;
  parent: string;
  directories: { name: string; path: string }[];
}

const KEY_STORAGE = "librinode-api-key";

export function getApiKey(): string {
  return localStorage.getItem(KEY_STORAGE) ?? "";
}

// proxiedImage routes a provider image (Hardcover/AniList/ComicVine art)
// through LibriNode's caching proxy so it's served locally and survives the
// provider's URL rot. Local API URLs (our own /cover endpoint) pass through
// unchanged; empty URLs return undefined so callers can fall back.
export function proxiedImage(url?: string): string | undefined {
  if (!url) return undefined;
  if (url.startsWith("/")) return url;
  return `/api/v1/image?url=${encodeURIComponent(url)}&apikey=${encodeURIComponent(getApiKey())}`;
}

export function setApiKey(key: string) {
  localStorage.setItem(KEY_STORAGE, key);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    ...init,
    headers: { "X-Api-Key": getApiKey(), ...init?.headers },
  });
  if (!resp.ok) {
    let message = resp.statusText;
    try {
      const body = await resp.json();
      if (body.error) message = body.error;
    } catch {
      // non-JSON error body; keep statusText
    }
    throw new ApiError(resp.status, message);
  }
  if (resp.status === 204) {
    return undefined as T;
  }
  return resp.json() as Promise<T>;
}

const json = (body: unknown): RequestInit => ({
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

export const api = {
  systemStatus: () => request<SystemStatus>("/api/v1/system/status"),
  authStatus: () => request<AuthStatus>("/api/v1/auth/status"),
  // First-run wizard: only answers/claims on a fresh instance — no API key.
  setupStatus: () => request<{ needed: boolean }>("/api/v1/setup/status"),
  setupInstance: (username: string, password: string) =>
    request<{ ok: boolean }>("/api/v1/auth/setup", json({ username, password })),
  login: (username: string, password: string) =>
    request<{ ok: boolean }>("/api/v1/auth/login", json({ username, password })),
  logout: () => request<void>("/api/v1/auth/logout", { method: "POST" }),
  setCredentials: (username: string, password: string) =>
    request<{ authEnabled: boolean }>("/api/v1/auth/credentials", {
      ...json({ username, password }),
      method: "PUT",
    }),
  listUsers: () => request<{ users: UserAccount[] }>("/api/v1/auth/users"),
  addUser: (username: string, password: string) =>
    request<{ users: UserAccount[] }>("/api/v1/auth/users", json({ username, password })),
  removeUser: (username: string) =>
    request<{ users: UserAccount[] }>(`/api/v1/auth/users/${encodeURIComponent(username)}`, {
      method: "DELETE",
    }),
  setUserPassword: (username: string, password: string) =>
    request<{ ok: boolean }>(`/api/v1/auth/users/${encodeURIComponent(username)}/password`, {
      ...json({ password }),
      method: "PUT",
    }),
  makeDefaultUser: (username: string) =>
    request<{ users: UserAccount[] }>(`/api/v1/auth/users/${encodeURIComponent(username)}/default`, {
      method: "PUT",
    }),
  regenerateApiKey: () =>
    request<{ apiKey: string }>("/api/v1/auth/apikey/regenerate", {
      method: "POST",
    }),
  logTail: (lines = 200) =>
    request<{ lines: string[]; path: string }>(`/api/v1/log?lines=${lines}`),
  listBackups: () => request<BackupInfo[]>("/api/v1/backup"),
  createBackup: () => request<BackupInfo>("/api/v1/backup", { method: "POST" }),
  deleteBackup: (name: string) =>
    request<void>(`/api/v1/backup/${name}`, { method: "DELETE" }),
  restoreBackup: (name: string) =>
    request<{ staged: number; message: string }>(
      `/api/v1/backup/${name}/restore`,
      { method: "POST" },
    ),
  downloadBackup: async (name: string): Promise<Blob> => {
    const resp = await fetch(`/api/v1/backup/${name}/download`, {
      headers: { "X-Api-Key": getApiKey() },
    });
    if (!resp.ok) throw new ApiError(resp.status, resp.statusText);
    return resp.blob();
  },
  health: () => request<HealthResult>("/api/v1/health"),
  checkHealth: () =>
    request<HealthResult>("/api/v1/health/check", { method: "POST" }),
  libraries: () => request<LibraryStatus[]>("/api/v1/libraries"),
  home: () => request<HomeSection[]>("/api/v1/home"),
  wanted: (library: string) =>
    request<{ items: HomeItem[] }>(`/api/v1/wanted?library=${library}`),
  calendar: (past = 30, days = 90) =>
    request<{ items: CalendarItem[]; from: string; to: string }>(
      `/api/v1/calendar?past=${past}&days=${days}`,
    ),
  setBookLibrary: (
    id: number,
    library: string,
    member: boolean,
    monitored: boolean,
    deleteFiles = false,
  ) =>
    request<Book>(`/api/v1/book/${id}/library`, {
      ...json({ library, member, monitored, deleteFiles }),
      method: "PUT",
    }),

  searchAuthors: (term: string) =>
    request<SearchAuthor[]>(
      `/api/v1/search?type=author&term=${encodeURIComponent(term)}`,
    ),
  searchBooks: (term: string) =>
    request<SearchBook[]>(
      `/api/v1/search?type=book&term=${encodeURIComponent(term)}`,
    ),

  listAuthors: (library?: string) =>
    request<Author[]>(`/api/v1/author${library ? `?library=${library}` : ""}`),
  getAuthor: (id: number) => request<Author>(`/api/v1/author/${id}`),
  addAuthor: (foreignAuthorId: string, library: string = "ebook") =>
    request<Author>("/api/v1/author", json({ foreignAuthorId, library })),
  refreshAuthor: (id: number) =>
    request<Author>(`/api/v1/author/${id}/refresh`, { method: "POST" }),
  setAuthorProvider: (id: number, provider: string) =>
    request<Author>(`/api/v1/author/${id}/provider`, {
      ...json({ provider }),
      method: "PUT",
    }),
  authorMissing: (id: number, library: string) =>
    request<Book[]>(`/api/v1/author/${id}/missing?library=${library}`),
  searchAuthorWanted: (id: number, library: string) =>
    request<{ searched: number; grabbed: number; outcomes: SearchOutcome[] }>(
      `/api/v1/author/${id}/search?library=${library}`,
      { method: "POST" },
    ),
  removeAuthorFromLibrary: (id: number, library: string, deleteFiles: boolean) =>
    request<unknown>(`/api/v1/author/${id}/library`, {
      ...json({ library, member: false, deleteFiles }),
      method: "PUT",
    }),
  listBooks: (authorId?: number) =>
    request<Book[]>(authorId ? `/api/v1/book?authorId=${authorId}` : "/api/v1/book"),
  getBook: (id: number) => request<Book>(`/api/v1/book/${id}`),
  addBook: (foreignBookId: string, library: string = "ebook") =>
    request<Book>("/api/v1/book", json({ foreignBookId, library })),
  monitorBook: (id: number, monitored: boolean) =>
    request(`/api/v1/book/${id}/monitor`, {
      ...json({ monitored }),
      method: "PUT",
    }),
  scan: () => request<ScanResult>("/api/v1/library/scan", { method: "POST" }),
  listUnmatchedFiles: () =>
    request<BookFile[]>("/api/v1/bookfile?unmatched=true"),
  renamePreview: (authorId?: number, seriesId?: number) =>
    request<RenameResult>(
      `/api/v1/library/rename${
        seriesId ? `?seriesId=${seriesId}` : authorId ? `?authorId=${authorId}` : ""
      }`,
    ),
  renameApply: (authorId?: number, seriesId?: number) =>
    request<RenameResult>("/api/v1/library/rename", {
      ...json(seriesId ? { seriesId } : authorId ? { authorId } : {}),
      method: "POST",
    }),
  unmatchedOptions: (mediaType: string) =>
    request<UnmatchedOption[]>(
      `/api/v1/bookfile/unmatched/options?mediaType=${mediaType}`,
    ),
  importMatched: (mediaType: string) =>
    request<{ imported: number; needsReview: number; messages: string[] }>(
      "/api/v1/bookfile/import-matched",
      json({ mediaType }),
    ),
  // materializeIssue imports an unmatched magazine file: the issue book is
  // created on the spot and the file adopted into it.
  materializeIssue: (fileId: number, seriesId: number, issue: string) =>
    request<{ file: BookFile; skips: string[] }>(
      `/api/v1/bookfile/${fileId}/match`,
      json({ seriesId, issue }),
    ),
  matchFile: (fileId: number, bookId: number) =>
    request<{ file: BookFile; skips: string[] }>(
      `/api/v1/bookfile/${fileId}/match`,
      json({ bookId }),
    ),
  replaceFile: (fileId: number, bookId: number) =>
    request<{ file: BookFile; skips: string[]; deletedFiles: number; errors: string[] }>(
      `/api/v1/bookfile/${fileId}/replace`,
      json({ bookId }),
    ),
  dismissFile: (fileId: number, deleteFiles = false) =>
    request<void>(`/api/v1/bookfile/${fileId}${deleteFiles ? "?deleteFiles=true" : ""}`, {
      method: "DELETE",
    }),

  listDownloadClients: () =>
    request<DownloadClient[]>("/api/v1/downloadclient"),
  addDownloadClient: (c: Omit<DownloadClient, "id">) =>
    request<DownloadClient>("/api/v1/downloadclient", json(c)),
  updateDownloadClient: (c: DownloadClient) =>
    request<DownloadClient>(`/api/v1/downloadclient/${c.id}`, {
      ...json(c),
      method: "PUT",
    }),
  deleteDownloadClient: (id: number) =>
    request<void>(`/api/v1/downloadclient/${id}`, { method: "DELETE" }),
  testDownloadClient: (c: Omit<DownloadClient, "id">) =>
    request<{ ok: boolean }>("/api/v1/downloadclient/test", json(c)),
  grabRelease: (
    title: string,
    downloadUrl: string,
    protocol: string,
    bookId?: number,
    mediaType: string = "ebook",
    guid: string = "",
  ) =>
    request<{ client: string; id?: string; grabId: number }>(
      "/api/v1/release/grab",
      json({ title, downloadUrl, protocol, bookId, mediaType, guid }),
    ),
  blocklist: () => request<BlockEntry[]>("/api/v1/blocklist"),
  unblock: (id: number) =>
    request<void>(`/api/v1/blocklist/${id}`, { method: "DELETE" }),
  searchReleasesForBook: (bookId: number, mediaType: string = "ebook") =>
    request<{ releases: ReleaseCandidate[]; errors: string[] }>(
      `/api/v1/release?bookId=${bookId}&mediaType=${mediaType}`,
    ),
  autoSearchBook: (bookId: number, mediaType: string = "ebook") =>
    request<SearchOutcome>(
      `/api/v1/book/${bookId}/search?mediaType=${mediaType}`,
      { method: "POST" },
    ),
  searchWanted: () =>
    request<{ searched: number; grabbed: number; outcomes: SearchOutcome[] }>(
      "/api/v1/library/search",
      { method: "POST" },
    ),
  queue: () =>
    request<{ items: QueueItem[]; errors: string[] }>("/api/v1/queue"),
  removeQueueItem: (clientConfigId: number, itemId: string, grabId?: number) =>
    request<{ removed: string }>(
      `/api/v1/queue/${clientConfigId}/${encodeURIComponent(itemId)}${grabId ? `?grabId=${grabId}` : ""}`,
      { method: "DELETE" },
    ),
  history: () => request<GrabRecord[]>("/api/v1/history"),
  runImport: () =>
    request<ImportResult>("/api/v1/library/import", { method: "POST" }),

  listProfiles: () => request<QualityProfile[]>("/api/v1/qualityprofile"),
  addProfile: (p: Partial<QualityProfile>) =>
    request<QualityProfile>("/api/v1/qualityprofile", json(p)),
  updateProfile: (p: QualityProfile) =>
    request<QualityProfile>(`/api/v1/qualityprofile/${p.id}`, {
      ...json(p),
      method: "PUT",
    }),
  deleteProfile: (id: number) =>
    request<void>(`/api/v1/qualityprofile/${id}`, { method: "DELETE" }),
  setDefaultProfile: (id: number) =>
    request<QualityProfile>(`/api/v1/qualityprofile/${id}/default`, {
      method: "PUT",
    }),

  listIndexers: () => request<Indexer[]>("/api/v1/indexer"),
  addIndexer: (ind: Omit<Indexer, "id" | "addedAt">) =>
    request<Indexer>("/api/v1/indexer", json(ind)),
  updateIndexer: (ind: Indexer) =>
    request<Indexer>(`/api/v1/indexer/${ind.id}`, {
      ...json(ind),
      method: "PUT",
    }),
  deleteIndexer: (id: number) =>
    request<void>(`/api/v1/indexer/${id}`, { method: "DELETE" }),
  testIndexer: (ind: Omit<Indexer, "id" | "addedAt">) =>
    request<{ ok: boolean }>("/api/v1/indexer/test", json(ind)),

  getNamingSettings: () => request<NamingSettings>("/api/v1/settings/naming"),
  saveNamingSettings: (templates: Partial<NamingSettings>) =>
    request<NamingSettings>("/api/v1/settings/naming", {
      ...json(templates),
      method: "PUT",
    }),

  getImportSettings: () => request<ImportSettings>("/api/v1/settings/import"),
  saveImportSettings: (settings: ImportSettings) =>
    request<ImportSettings>("/api/v1/settings/import", {
      ...json(settings),
      method: "PUT",
    }),

  searchSeries: (term: string, mediaType: string) =>
    request<SeriesResult[]>(
      `/api/v1/search?term=${encodeURIComponent(term)}&type=${mediaType}`,
    ),
  listSeries: () => request<Series[]>("/api/v1/series"),
  addSeries: (mediaType: string, foreignSeriesId: string) =>
    request<Series>("/api/v1/series", json({ mediaType, foreignSeriesId })),
  addMagazine: (title: string) =>
    request<Series>("/api/v1/series", json({ mediaType: "magazine", title })),
  getSeries: (id: number) => request<Series>(`/api/v1/series/${id}`),
  setSeriesProvider: (id: number, provider: string) =>
    request<Series>(`/api/v1/series/${id}/provider`, {
      ...json({ provider }),
      method: "PUT",
    }),
  monitorSeries: (id: number, monitored: boolean, monitorNew: boolean) =>
    request<Series>(`/api/v1/series/${id}/monitor`, {
      ...json({ monitored, monitorNew }),
      method: "PUT",
    }),
  refreshSeries: (id: number) =>
    request<Series>(`/api/v1/series/${id}/refresh`, { method: "POST" }),
  searchSeriesWanted: (id: number) =>
    request<{ searched: number; grabbed: number; outcomes: SearchOutcome[] }>(
      `/api/v1/series/${id}/search`,
      { method: "POST" },
    ),
  deleteSeries: (id: number, deleteFiles = false) =>
    request<{ deletedFiles: number; errors: string[] } | undefined>(
      `/api/v1/series/${id}${deleteFiles ? "?deleteFiles=true" : ""}`,
      { method: "DELETE" },
    ),

  listRootFolders: () => request<RootFolder[]>("/api/v1/rootfolder"),
  browseFolders: (path?: string) =>
    request<FolderListing>(
      `/api/v1/filesystem${path ? `?path=${encodeURIComponent(path)}` : ""}`,
    ),
  addRootFolder: (mediaType: string, path: string, variant?: string) =>
    request<RootFolder>("/api/v1/rootfolder", json({ mediaType, path, variant })),
  deleteRootFolder: (id: number) =>
    request<void>(`/api/v1/rootfolder/${id}`, { method: "DELETE" }),

  getMetadataSettings: () =>
    request<MetadataSettings>("/api/v1/settings/metadata"),
  saveMetadataSettings: (
    active: string,
    providers: Record<string, ProviderSettings>,
    extra?: {
      mangaProvider?: string;
      comicProvider?: string;
      mangaCoverSource?: string;
      comicCoverSource?: string;
      language?: string;
      country?: string;
      includeAdult?: boolean;
    },
  ) =>
    request<MetadataSettings>("/api/v1/settings/metadata", {
      ...json({ active, providers, ...extra }),
      method: "PUT",
    }),
  testMetadataProvider: (provider: string, settings: ProviderSettings) =>
    request<{ ok: boolean }>("/api/v1/settings/metadata/test",
      json({ provider, settings })),
  clearMetadataCache: () =>
    request<{ removed: number; freedBytes: number }>("/api/v1/settings/metadata/cache", {
      method: "DELETE",
    }),
  clearCoverCache: () =>
    request<{ removed: number; freedBytes: number }>("/api/v1/library/covers/cache", {
      method: "DELETE",
    }),
  clearDescriptions: () =>
    request<{ descriptionsCleared: number }>("/api/v1/settings/metadata/descriptions", {
      method: "DELETE",
    }),
  clearAllCache: () =>
    request<{ removed: number; freedBytes: number; descriptionsCleared: number }>(
      "/api/v1/cache",
      { method: "DELETE" },
    ),
};
