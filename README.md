# Quillarr

A self-hosted media automation server for **written media** — the Readarr / LazyLibrarian successor that treats ebooks, audiobooks, manga, and comics as first-class citizens.

Quillarr monitors your wanted list, searches your indexers, sends releases to your download client, then imports, renames, and organizes files into per-type libraries — automatically.

Runs on **Windows** and **Linux** (bare metal or Docker).

---

## Why another *arr?

- **Readarr** is retired/unmaintained and never handled manga or comics.
- **LazyLibrarian** covers a lot but has an aging UI and inconsistent metadata.
- **Mylar** does comics only; **Kavita/Komga** are readers, not automation.

Nothing today automates **all four** written-media types in one app with modern metadata (Hardcover) and clean *arr-style integrations. That's the gap Quillarr fills.

## Core Features

### 📚 Four media types, four libraries
Each media type is a fully independent library with its **own root folder(s)**, naming scheme, quality profile, and monitoring rules:

| Type | Root folder (example) | Formats |
|---|---|---|
| Ebooks | `D:\Media\Ebooks` / `/data/ebooks` | epub, mobi, azw3, pdf |
| Audiobooks | `D:\Media\Audiobooks` / `/data/audiobooks` | m4b, mp3, flac, opus |
| Manga | `D:\Media\Manga` / `/data/manga` | cbz, cbr, epub |
| Comics | `D:\Media\Comics` / `/data/comics` | cbz, cbr, pdf |

An author/series can exist in multiple libraries at once (e.g. own the ebook *and* the audiobook) without conflicts.

### ⬇️ Download clients
- **qBittorrent** (torrents) — category support, seed-goal awareness, remove-after-import
- **SABnzbd** (usenet) — category support, post-processing hand-off
- Per-media-type category mapping (e.g. `quillarr-ebooks`, `quillarr-manga`)
- Completed Download Handling: watch, import, rename, clean up

### 🔍 Indexers via Prowlarr
- Full **Prowlarr application sync** — Prowlarr pushes indexers to Quillarr automatically, just like it does for Sonarr/Radarr
- Standard **Newznab / Torznab** API support for manual indexer entry
- Per-indexer category mapping to media types (books, audio, comics categories)
- Interactive (manual) search and automatic RSS-based grabbing

### 🏷️ Metadata via Hardcover
- **Hardcover.app** as the primary metadata provider for books and audiobooks (authors, series, editions, covers, descriptions, release dates)
- Pluggable provider architecture so manga/comic sources (e.g. AniList, ComicVine, Metron) slot in behind the same interface
- Metadata refresh on schedule + manual refresh
- Writes sidecar metadata (OPF / ComicInfo.xml) for readers like Kavita, Komga, Calibre, and Audiobookshelf

### ⚙️ Clean, organized settings
Settings are grouped by concern, not dumped on one page:

```
Settings
├── Media Management     (root folders per type, file naming, import behavior)
├── Libraries            (per-type: quality, formats, monitoring defaults)
├── Metadata             (Hardcover account/API, provider priorities, sidecar files)
├── Indexers             (Prowlarr sync, manual Newznab/Torznab, categories)
├── Download Clients     (qBittorrent, SABnzbd, category mapping)
├── Connect              (notifications: Discord, webhooks, email, ...)
├── General              (host/port, auth, SSL, proxy, logging, backups)
└── UI                   (theme, language, date formats)
```

Every settings page follows the same pattern: sensible defaults, a **Test** button on every connection, and advanced options hidden behind a toggle.

---

## Architecture

- **Backend:** **Go** — compiles to a single self-contained binary per OS, no runtime installs for the user
- **Frontend:** **React** SPA (Vite), embedded into the binary and served on one port (*arr-style)
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no cgo) with embedded schema migrations
- **API:** versioned REST API (`/api/v1`) with API-key auth — the same API the UI uses, so everything is scriptable; Prowlarr-compatible surface for app sync
- **Default port:** `7845` (Q-U-I-L on a phone keypad)
- **Distribution:** Windows installer + service, Linux systemd unit, Docker image (linuxserver-style paths/PUID/PGID conventions)
- **License:** GPL-3.0 (same family as Sonarr/Radarr/Prowlarr)

## Development

Requires Go 1.24+.

> **Windows note:** if Smart App Control is enabled, Windows blocks locally
> compiled (unsigned) binaries from running. Develop inside WSL, or turn off
> SAC (Windows Security → App & browser control) — official releases will be
> code-signed so end users are unaffected.

```sh
go run ./cmd/quillarr          # starts on http://localhost:7845
go test ./...                  # run tests
go build ./cmd/quillarr        # produce the quillarr binary
```

Frontend (requires Node 22+):

```sh
cd web
npm install
npm run dev                    # Vite dev server, proxies /api to :7845
npm run build                  # production build into web/dist
```

`go build` embeds `web/dist` into the binary, which then serves the UI on
its own port; without a frontend build it falls back to a plain status page.

On first run Quillarr creates its data directory (`%AppData%\Quillarr` on
Windows, `~/.config/quillarr` on Linux) containing `config.yaml` — with a
generated API key — and the SQLite database. Override the location with
`--data <dir>`; settings can also be set via `QUILLARR_*` env vars
(`QUILLARR_PORT`, `QUILLARR_API_KEY`, ...).

API calls need the key from `config.yaml`:

```sh
curl -H "X-Api-Key: <key>" http://localhost:7845/api/v1/system/status
```

Metadata search and add require a [Hardcover](https://hardcover.app) API
token: paste it under **Settings → Metadata Provider** in the web UI (it
takes effect immediately — no restart) or set `QUILLARR_HARDCOVER_TOKEN`.
Tokens live under the `metadata:` section of `config.yaml`. Without one,
metadata endpoints return 503.

---

## Roadmap

### Phase 0 — Foundation
- [x] Finalize name (Quillarr), license (GPL-3.0), repo structure
- [x] Choose stack (Go backend, React frontend) and scaffold the project
- [x] SQLite schema + migrations framework
- [x] Config system (file + env vars), logging, cross-platform paths
- [x] REST API skeleton with API-key auth (system status, root folder CRUD)
- [ ] CI: build + test on Windows and Linux

### Phase 1 — Library core (ebooks first)
- [x] Media type + root folder model (multiple roots per type)
- [x] Author / Series / Book / Edition data model
- [x] Hardcover metadata provider: search, author/series/book lookup, covers *(mock-tested; live API verification pending a Hardcover token)*
- [x] Add author or book → monitor wanted editions
- [x] React + Vite web UI: library browsing, search-and-add, monitor toggles (embedded in the binary)
- [x] Scheduled + manual metadata refresh
- [x] Provider registry + metadata settings in the UI (token entry, Test button, hot-swap without restart)
- [ ] Library scanning: detect existing files, match to metadata
- [ ] File naming templates + rename engine
- [ ] Manual import with match correction

### Phase 2 — Acquisition pipeline
- [ ] Indexer framework: Newznab + Torznab clients
- [ ] **Prowlarr application sync** (Prowlarr adds/updates/removes indexers in Quillarr)
- [ ] Release parsing + scoring (format, quality, language, revision)
- [ ] Quality profiles per library
- [ ] **qBittorrent** client: add, track, seed goals, remove
- [ ] **SABnzbd** client: add, track, post-process
- [ ] Completed Download Handling: import, rename, cleanup, failed-download handling
- [ ] Automatic search for wanted items + RSS sync loop
- [ ] Interactive search UI

### Phase 3 — Audiobooks
- [ ] Audiobook library type with its own root folders, formats, quality profile
- [ ] Edition awareness: same book as ebook vs audiobook, monitored independently
- [ ] Audio-specific parsing (narrator, bitrate, m4b vs mp3, chapterized)
- [ ] Audiobookshelf-friendly folder layout + metadata

### Phase 4 — Manga & comics
- [ ] Volume/chapter/issue data model (series-first instead of author-first)
- [ ] Manga metadata provider (AniList/MangaUpdates) behind the provider interface
- [ ] Comic metadata provider (ComicVine/Metron)
- [ ] CBZ/CBR handling + `ComicInfo.xml` sidecars
- [ ] Issue/volume monitoring ("monitor future volumes")
- [ ] Kavita/Komga-friendly folder layouts

### Phase 5 — Polish & 1.0
- [ ] Full settings UI as specced above, with Test buttons everywhere
- [ ] Notifications (Discord, webhook, email) on grab/import/upgrade/failure
- [ ] Calendar view (upcoming releases across all libraries)
- [ ] Upgrade handling (replace when a better-quality release appears)
- [ ] Backups + restore
- [ ] Windows installer/service, Linux packages/systemd, official Docker image
- [ ] Docs site + API reference

### Post-1.0 ideas
- [ ] Import lists (Hardcover want-to-read shelf → auto-monitor)
- [ ] Multi-user / permissions
- [ ] Direct integrations: Calibre, Kavita, Komga, Audiobookshelf notify-on-import
- [ ] Additional metadata providers (Open Library, Google Books) as fallbacks
- [ ] Localization

---

## Status

🚧 **Pre-alpha — Phase 1 in progress.** The library core works end-to-end: search Hardcover, add authors/books, monitor editions, scheduled metadata refresh — from the embedded web UI or the REST API. Library scanning and the rename engine are next.

## License

[GPL-3.0](LICENSE)
