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
  books?: Book[];
}

export interface Book {
  id: number;
  authorId: number;
  foreignBookId: string;
  title: string;
  sortTitle: string;
  description: string;
  releaseDate: string;
  rating: number;
  coverUrl: string;
  monitored: boolean;
  hasFile: boolean;
  hasEbookFile: boolean;
  hasAudiobookFile: boolean;
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
  path: string;
  size: number;
  format: string;
  modifiedAt: string;
  addedAt: string;
}

export interface RootFolder {
  id: number;
  mediaType: string;
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
  id: string;
  title: string;
  status: string;
  progress: number;
  path?: string;
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
  coverUrl: string;
  volumes?: Book[];
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
  isDefault: boolean;
}

export interface NamingSettings {
  ebookFolder: string;
  ebookFile: string;
  audiobookFolder: string;
  audiobookFile: string;
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
}

const KEY_STORAGE = "librinode-api-key";

export function getApiKey(): string {
  return localStorage.getItem(KEY_STORAGE) ?? "";
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

  searchAuthors: (term: string) =>
    request<SearchAuthor[]>(
      `/api/v1/search?type=author&term=${encodeURIComponent(term)}`,
    ),
  searchBooks: (term: string) =>
    request<SearchBook[]>(
      `/api/v1/search?type=book&term=${encodeURIComponent(term)}`,
    ),

  listAuthors: () => request<Author[]>("/api/v1/author"),
  getAuthor: (id: number) => request<Author>(`/api/v1/author/${id}`),
  addAuthor: (foreignAuthorId: string) =>
    request<Author>("/api/v1/author", json({ foreignAuthorId })),
  refreshAuthor: (id: number) =>
    request<Author>(`/api/v1/author/${id}/refresh`, { method: "POST" }),
  monitorAuthor: (id: number, monitored: boolean) =>
    request(`/api/v1/author/${id}/monitor`, {
      ...json({ monitored }),
      method: "PUT",
    }),
  deleteAuthor: (id: number) =>
    request<void>(`/api/v1/author/${id}`, { method: "DELETE" }),

  listBooks: (authorId?: number) =>
    request<Book[]>(authorId ? `/api/v1/book?authorId=${authorId}` : "/api/v1/book"),
  getBook: (id: number) => request<Book>(`/api/v1/book/${id}`),
  addBook: (foreignBookId: string) =>
    request<Book>("/api/v1/book", json({ foreignBookId })),
  monitorBook: (id: number, monitored: boolean) =>
    request(`/api/v1/book/${id}/monitor`, {
      ...json({ monitored }),
      method: "PUT",
    }),
  deleteBook: (id: number) =>
    request<void>(`/api/v1/book/${id}`, { method: "DELETE" }),

  monitorEdition: (id: number, monitored: boolean) =>
    request(`/api/v1/edition/${id}/monitor`, {
      ...json({ monitored }),
      method: "PUT",
    }),

  scan: () => request<ScanResult>("/api/v1/library/scan", { method: "POST" }),
  listUnmatchedFiles: () =>
    request<BookFile[]>("/api/v1/bookfile?unmatched=true"),
  renamePreview: () => request<RenameResult>("/api/v1/library/rename"),
  renameApply: () =>
    request<RenameResult>("/api/v1/library/rename", { method: "POST" }),
  matchFile: (fileId: number, bookId: number) =>
    request<{ file: BookFile; skips: string[] }>(
      `/api/v1/bookfile/${fileId}/match`,
      json({ bookId }),
    ),
  dismissFile: (fileId: number) =>
    request<void>(`/api/v1/bookfile/${fileId}`, { method: "DELETE" }),

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
  ) =>
    request<{ client: string; id?: string; grabId: number }>(
      "/api/v1/release/grab",
      json({ title, downloadUrl, protocol, bookId, mediaType }),
    ),
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
  history: () => request<GrabRecord[]>("/api/v1/history"),
  runImport: () =>
    request<ImportResult>("/api/v1/library/import", { method: "POST" }),

  listProfiles: () => request<QualityProfile[]>("/api/v1/qualityprofile"),
  addProfile: (p: Partial<QualityProfile>) =>
    request<QualityProfile>("/api/v1/qualityprofile", json(p)),
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
  searchReleases: (term: string) =>
    request<{ releases: Release[]; errors: string[] }>(
      `/api/v1/release?term=${encodeURIComponent(term)}`,
    ),

  getNamingSettings: () => request<NamingSettings>("/api/v1/settings/naming"),
  saveNamingSettings: (
    ebookFolder: string,
    ebookFile: string,
    audiobookFolder: string,
    audiobookFile: string,
  ) =>
    request<NamingSettings>("/api/v1/settings/naming", {
      ...json({ ebookFolder, ebookFile, audiobookFolder, audiobookFile }),
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
  monitorSeries: (id: number, monitored: boolean, monitorNew: boolean) =>
    request<Series>(`/api/v1/series/${id}/monitor`, {
      ...json({ monitored, monitorNew }),
      method: "PUT",
    }),
  refreshSeries: (id: number) =>
    request<Series>(`/api/v1/series/${id}/refresh`, { method: "POST" }),
  deleteSeries: (id: number) =>
    request<void>(`/api/v1/series/${id}`, { method: "DELETE" }),

  listRootFolders: () => request<RootFolder[]>("/api/v1/rootfolder"),
  addRootFolder: (mediaType: string, path: string) =>
    request<RootFolder>("/api/v1/rootfolder", json({ mediaType, path })),
  deleteRootFolder: (id: number) =>
    request<void>(`/api/v1/rootfolder/${id}`, { method: "DELETE" }),

  getMetadataSettings: () =>
    request<MetadataSettings>("/api/v1/settings/metadata"),
  saveMetadataSettings: (active: string, providers: Record<string, ProviderSettings>) =>
    request<MetadataSettings>("/api/v1/settings/metadata", {
      ...json({ active, providers }),
      method: "PUT",
    }),
  testMetadataProvider: (provider: string, settings: ProviderSettings) =>
    request<{ ok: boolean }>("/api/v1/settings/metadata/test",
      json({ provider, settings })),
};
