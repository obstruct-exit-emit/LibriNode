# Quillarr

A self-hosted media automation server for **written media** — the Readarr / LazyLibrarian successor that treats ebooks, audiobooks, manga, and comics as first-class citizens.

Quillarr monitors your wanted list, searches your indexers, sends releases to your download client, then imports, renames, and organizes files into per-type libraries — automatically.

Runs on **Windows** and **Linux** (bare metal or Docker).

> 🚧 **Pre-alpha.** The Phase 1 library core is feature-complete and usable
> today — [see what works now](#getting-started-what-works-today). The
> acquisition pipeline (indexers, download clients) is Phase 2, in planning.

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

## Getting started (what works today)

1. Build and run the server (see [Development](#development) below), then
   open `http://localhost:7845` and paste the API key from `config.yaml` in
   your data directory.
2. **Settings → Metadata Provider:** paste your
   [Hardcover API token](https://hardcover.app/account/api), hit **Test**,
   then **Save** — search goes live immediately, no restart.
3. **Settings → Root Folders:** add the folder(s) where your ebooks live.
4. **Search:** find authors or books on Hardcover and add them to the
   library (adding an author pulls the full bibliography; adding a book
   pulls its editions).
5. **Library → Scan files:** match files you already own to library books —
   every book shows an **owned**/**wanted** badge; strays land in an
   unmatched list where you can import them against the right book or
   dismiss them.
6. **Library → Organize…:** preview, then apply, moving files into the
   naming-template layout.

File naming templates live under **Settings → File Naming**. Tokens:
`{Author Name}`, `{Author SortName}`, `{Book Title}`, `{Series Title}`,
`{Series Position}`, `{Release Year}` — tokens without a value drop out
cleanly, so the default
`{Author Name}/{Series Title} {Series Position} - {Book Title}` renders
`Terry Pratchett/Discworld 1 - The Colour of Magic.epub` for series books
and `Neil Gaiman/Coraline.epub` for standalones.

## Development

Requires Go 1.25+.

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

### API overview (v1)

Everything the UI does goes through `/api/v1` — same endpoints, fully
scriptable:

| Area | Endpoints |
|---|---|
| System | `GET /system/status`, `GET /ping` (no auth) |
| Root folders | `GET/POST /rootfolder`, `DELETE /rootfolder/{id}` |
| Search | `GET /search?term=&type=author\|book` (metadata provider proxy) |
| Authors | `GET/POST /author`, `GET/DELETE /author/{id}`, `PUT /author/{id}/monitor`, `POST /author/{id}/refresh` |
| Books | `GET/POST /book`, `GET/DELETE /book/{id}`, `PUT /book/{id}/monitor`, `POST /book/{id}/refresh` |
| Editions | `PUT /edition/{id}/monitor` |
| Files | `POST /library/scan`, `GET/POST /library/rename` (preview/apply), `GET /bookfile?bookId=N\|unmatched=true`, `POST /bookfile/{id}/match`, `DELETE /bookfile/{id}` |
| Settings | `GET/PUT /settings/metadata`, `POST /settings/metadata/test`, `GET/PUT /settings/naming` |

`POST /author` takes `{"foreignAuthorId": "..."}` and pulls the full
bibliography; `POST /book` takes `{"foreignBookId": "..."}` and pulls one
book with its editions (creating an unmonitored author stub if needed).

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
- [x] Library scanning: detect existing files, match to metadata (owned/wanted per book, unmatched-file list)
- [x] File naming templates + rename engine (token templates, live example, preview-then-apply organize)
- [x] Manual import with match correction (assign unmatched files to books, auto-move into place, dismiss)

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

🚧 **Pre-alpha — Phase 1 (library core) feature-complete.** Everything in [Getting started](#getting-started-what-works-today) works end-to-end from the embedded web UI or the REST API. One asterisk: Hardcover calls are mock-tested pending a live API token. Phase 2 (indexers, download clients — the acquisition pipeline) is next.

## License

[GPL-3.0](LICENSE)
