// Typed client for Quillarr's REST API. The API key is kept in localStorage
// for now; proper session handling comes with the settings UI.

export interface SystemStatus {
  appName: string;
  version: string;
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
  editions?: Edition[];
  series?: SeriesLink[];
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

const KEY_STORAGE = "quillarr-api-key";

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
};
