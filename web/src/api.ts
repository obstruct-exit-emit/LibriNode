// Minimal typed client for Quillarr's REST API. The API key is kept in
// localStorage for now; proper session handling comes with the full UI.

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
  releaseDate: string;
  rating: number;
  coverUrl: string;
  monitored: boolean;
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
  return resp.json() as Promise<T>;
}

export const api = {
  systemStatus: () => request<SystemStatus>("/api/v1/system/status"),
  listAuthors: () => request<Author[]>("/api/v1/author"),
  listBooks: () => request<Book[]>("/api/v1/book"),
};
