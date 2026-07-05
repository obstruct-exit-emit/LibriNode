# LibriNode

A self-hosted media automation server for **written media** — the Readarr / LazyLibrarian successor that treats ebooks, audiobooks, manga, comics, and magazines as first-class citizens.

LibriNode monitors your wanted list, searches your indexers, sends releases to your download client, then imports, renames, and organizes files into per-type libraries — automatically.

Runs on **Windows** and **Linux** (bare metal or Docker).

> 🚧 **Pre-alpha, but the loop is closed.** Phases 0–4 are complete: all
> five media types work end-to-end, from metadata search through automatic
> grabbing to organized imports —
> [see what works now](#getting-started-what-works-today). Phase 5 (UI
> overhaul, upgrade handling, packaging) is what stands between here and 1.0.

---

## Why another *arr?

- **Readarr** is retired/unmaintained and never handled manga or comics.
- **LazyLibrarian** covers a lot (including magazines, which LibriNode also automates) but has an aging UI and inconsistent metadata.
- **Mylar** does comics only; **Kavita/Komga** are readers, not automation.

Nothing today automates **all five** written-media types in one app with modern metadata (Hardcover) and clean *arr-style integrations. That's the gap LibriNode fills.

## Core Features

### 📚 Five media types, five libraries
Each media type is a fully independent library with its **own root folder(s)**, naming scheme, quality profile, and monitoring rules — and, Plex-style, a library only appears in the UI once you set it up:

| Type | Root folder (example) | Formats |
|---|---|---|
| Ebooks | `D:\Media\Ebooks` / `/data/ebooks` | epub, mobi, azw3, pdf |
| Audiobooks | `D:\Media\Audiobooks` / `/data/audiobooks` | m4b, m4a, mp3, flac, opus |
| Manga | `D:\Media\Manga` / `/data/manga` | cbz, cbr, epub |
| Comics | `D:\Media\Comics` / `/data/comics` | cbz, cbr, pdf |
| Magazines | `D:\Media\Magazines` / `/data/magazines` | pdf, epub, cbz |

An author/series can exist in multiple libraries at once (e.g. own the ebook *and* the audiobook) without conflicts — but only in the libraries where you actually own or added that format.

### ⬇️ Download clients
- **qBittorrent** (torrents) — category support, seed-goal awareness, remove-after-import
- **SABnzbd** (usenet) — category support, post-processing hand-off
- Per-media-type category mapping (e.g. `librinode-ebooks`, `librinode-manga`)
- Completed Download Handling: watch, import, rename, clean up

### 🔍 Indexers via Prowlarr
- Full **Prowlarr application sync** — Prowlarr pushes indexers to LibriNode automatically, just like it does for Sonarr/Radarr
- Standard **Newznab / Torznab** API support for manual indexer entry
- Per-indexer category mapping to media types (book, audio, comic, and magazine categories)
- Interactive (manual) search and automatic scheduled grabbing of wanted items

### 🏷️ Metadata via Hardcover
- **Hardcover.app** as the primary metadata provider for books and audiobooks (authors, series, editions, covers, descriptions, release dates)
- Pluggable provider architecture: **AniList** (manga, no key needed) and **ComicVine** (comics) already slot in behind the series-provider interface; more sources can follow
- Metadata refresh on schedule + manual refresh
- Writes sidecar metadata for readers: `ComicInfo.xml` into imported CBZs today (Kavita/Komga); OPF for Calibre/Audiobookshelf comes with Phase 5

### ⚙️ Clean, organized settings
Settings are grouped by concern, not dumped on one page:

```
Settings
├── Media Management     (root folders per type, file naming, import behavior)
├── Libraries            (per-type: quality, formats, monitoring defaults)
├── Metadata             (Hardcover account/API, provider priorities, sidecar files)
├── Indexers             (Prowlarr sync, manual Newznab/Torznab, categories)
├── Download Clients     (qBittorrent, SABnzbd, category mapping)
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
- **Default port:** `7845`
- **Distribution:** Windows installer + service, Linux systemd unit, Docker image (linuxserver-style paths/PUID/PGID conventions)
- **License:** GPL-3.0 (same family as Sonarr/Radarr/Prowlarr)

## Getting started (what works today)

1. Build and run the server (see [Development](#development) below), then
   open `http://localhost:7845` and paste the API key from `config.yaml` in
   your data directory.
2. **Settings → Metadata Provider:** paste your
   [Hardcover API token](https://hardcover.app/account/api), hit **Test**,
   then **Save** — search goes live immediately, no restart.
3. **Settings → Root Folders:** add the folder(s) where your media lives —
   one root per media type (ebook, audiobook, manga, comic, magazine).
4. **Search:** find authors or books on Hardcover and add them to the
   library (adding an author pulls the full bibliography; adding a book
   pulls its editions).
5. **Library → Scan files:** match files you already own to library books —
   every book shows an **owned**/**wanted** badge. Strays land in an
   unmatched list and attach automatically the moment you add their book
   from Search; you can also match them by hand or dismiss them.
6. **Library → Organize…:** preview, then apply, moving files into the
   naming-template layout.
7. **Settings → Download Clients:** point LibriNode at qBittorrent
   (torrents) and/or SABnzbd (usenet). Then search releases
   (`GET /api/v1/release?bookId=N` returns parsed, scored, ranked
   candidates). Every wanted book in the Library has **Auto grab** (search
   indexers, grab the best release) and **Search releases** (pick from the
   ranked candidates yourself); **Search wanted** in the Library header
   sweeps everything at once, and a background pass does the same every six
   hours. Finished downloads import automatically (checked every minute, or
   via **Import now**), the **Activity** tab shows the live queue and grab
   history, and format preferences live under **Settings → Quality
   Profiles**.

**Audiobooks:** add an audiobook root folder, and scanning understands both
`Author/Title.m4b` and multi-file `Author/Title/*.mp3` layouts (each book
folder is one unit). To acquire a book as an audiobook, open the book,
monitor one of its audiobook editions, and either wait for the automatic
sweep or pick **audiobook** in the book's search controls. Audiobook
searches use each indexer's **Audio categories** (default `3030`), and
imports land as `Author/Book Title/` folders — Audiobookshelf-ready.

**Manga & comics** are series-first: search with type **Manga** (AniList,
no key needed) or **Comics** (needs a free ComicVine API key, entered on
the Metadata settings card), add the series, and every volume/issue lands
in the **Series** tab with owned/wanted badges and per-volume Auto grab.
"Monitor future volumes" is the series' monitor toggle — refreshes (manual
or the daily sweep) discover new volumes and start monitoring them. One
AniList quirk: *ongoing* manga often have no official volume count yet, so
they add with zero volumes and fill in once AniList publishes totals —
completed series (e.g. Death Note) arrive with all volumes immediately.
Manga/comic searches use each indexer's **Comic categories** (default
`7030`), scans understand `Series/Series v05.cbz` layouts, and imports
write `ComicInfo.xml` into CBZ archives for Kavita/Komga.

**Magazines** work LazyLibrarian-style — periodicals have no metadata
provider, so you add one **by name** (Series tab → "Add magazine"). LibriNode
then recognizes issues by date or number in release and file names
(`The Economist - 2026-07-04.pdf`, `Retro Gamer Issue 261`): scanning a
magazine root materializes owned issues into the library automatically, and
the automatic search grabs newly published issues (capped at 3 per pass)
using each indexer's **Magazine categories** (default `7010`). Imports land
as `Magazine/Magazine - date.pdf`.

**Indexers** can be added two ways: manually under **Settings → Indexers**
(any Newznab/Torznab endpoint, including per-indexer feed URLs from
Prowlarr or Jackett), or automatically via **Prowlarr application sync** —
in Prowlarr, add an application of type **Readarr** with LibriNode's URL
(`http://localhost:7845`) and API key, and Prowlarr will push its indexers
into LibriNode and keep them in sync. The indexer endpoints accept both
LibriNode's native JSON and Readarr v1 resources, and
`/api/v1/system/status` reports a Readarr-compatible `version` for
Prowlarr's checks (LibriNode's own version is `appVersion`).

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
go run ./cmd/librinode          # starts on http://localhost:7845
go test ./...                  # run tests
go build ./cmd/librinode        # produce the librinode binary
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

On first run LibriNode creates its data directory (`%AppData%\LibriNode` on
Windows, `~/.config/librinode` on Linux) containing `config.yaml` — with a
generated API key — and the SQLite database. Override the location with
`--data <dir>`; settings can also be set via `LIBRINODE_*` env vars
(`LIBRINODE_PORT`, `LIBRINODE_API_KEY`, ...).

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
| Search | `GET /search?term=&type=author\|book\|manga\|comic` (metadata provider proxy) |
| Series | `GET/POST /series` (manga/comic by foreign id; magazines by `{"mediaType":"magazine","title":"..."}`), `GET/DELETE /series/{id}`, `PUT /series/{id}/monitor`, `POST /series/{id}/refresh` |
| Authors | `GET/POST /author`, `GET/DELETE /author/{id}`, `PUT /author/{id}/monitor`, `POST /author/{id}/refresh` |
| Books | `GET/POST /book`, `GET/DELETE /book/{id}`, `PUT /book/{id}/monitor`, `POST /book/{id}/refresh` |
| Editions | `PUT /edition/{id}/monitor` |
| Files | `POST /library/scan`, `GET/POST /library/rename` (preview/apply), `GET /bookfile?bookId=N\|unmatched=true`, `POST /bookfile/{id}/match`, `DELETE /bookfile/{id}` |
| Indexers | `GET/POST /indexer`, `GET/PUT/DELETE /indexer/{id}`, `GET /indexer/schema`, `POST /indexer/test`, `GET /release?term=` or `?bookId=N` (+ `&mediaType=ebook\|audiobook\|manga\|comic\|magazine`; volumes imply their own type) — parsed + scored candidates from all enabled indexers |
| Quality | `GET/POST /qualityprofile`, `PUT/DELETE /qualityprofile/{id}`, `PUT /qualityprofile/{id}/default` |
| Downloads | `GET/POST /downloadclient`, `PUT/DELETE /downloadclient/{id}`, `POST /downloadclient/test`, `POST /release/grab` (with `bookId` for auto-import), `GET /queue`, `POST /library/import`, `GET /history` |
| Auto search | `POST /book/{id}/search?mediaType=` (grab best release for one book), `POST /library/search` (sweep all wanted books and formats) |
| Settings | `GET/PUT /settings/metadata`, `POST /settings/metadata/test`, `GET/PUT /settings/naming` |

`POST /author` takes `{"foreignAuthorId": "..."}` and pulls the full
bibliography; `POST /book` takes `{"foreignBookId": "..."}` and pulls one
book with its editions (creating an unmonitored author stub if needed).

Metadata search and add require a [Hardcover](https://hardcover.app) API
token: paste it under **Settings → Metadata Provider** in the web UI (it
takes effect immediately — no restart) or set `LIBRINODE_HARDCOVER_TOKEN`.
Tokens live under the `metadata:` section of `config.yaml`. Without one,
metadata endpoints return 503.

---

## Roadmap

### Phase 0 — Foundation
- [x] Finalize name (LibriNode), license (GPL-3.0), repo structure
- [x] Choose stack (Go backend, React frontend) and scaffold the project
- [x] SQLite schema + migrations framework
- [x] Config system (file + env vars), logging, cross-platform paths
- [x] REST API skeleton with API-key auth (system status, root folder CRUD)
- [x] CI: build + test on Windows and Linux

### Phase 1 — Library core
- [x] Media type + root folder model (multiple roots per type)
- [x] Author / Series / Book / Edition data model
- [x] Hardcover metadata provider: search, author/series/book lookup, covers *(verified against the live Hardcover API)*
- [x] Add author or book → monitor wanted editions
- [x] React + Vite web UI: library browsing, search-and-add, monitor toggles (embedded in the binary)
- [x] Scheduled + manual metadata refresh
- [x] Provider registry + metadata settings in the UI (token entry, Test button, hot-swap without restart)
- [x] Library scanning: detect existing files, match to metadata (owned/wanted per book, unmatched-file list)
- [x] File naming templates + rename engine (token templates, live example, preview-then-apply organize)
- [x] Manual import with match correction (assign unmatched files to books, auto-move into place, dismiss)

### Phase 2 — Acquisition pipeline
- [x] Indexer framework: Newznab + Torznab clients (add/test in Settings, manual release search across enabled indexers)
- [x] **Prowlarr application sync** — add LibriNode to Prowlarr as a *Readarr* application; Prowlarr pushes/updates/removes indexers automatically *(Readarr v1 API emulation; live verification against a real Prowlarr pending)*
- [x] Release parsing + scoring (formats, retail, language, year, scene names; book-aware search rejects wrong author/title/dead torrents and ranks the rest)
- [x] Quality profiles per library (ordered format preferences, language, size bounds; default profile drives search scoring; managed in Settings)
- [x] **qBittorrent** client: add, track, remove (category-scoped; seed goals with CDH)
- [x] **SABnzbd** client: add, track, remove (category-scoped; post-process hand-off with CDH)
- [x] Completed Download Handling: finished grabs import automatically (copy into naming-template layout, torrents keep seeding, usenet history cleaned up, failed downloads resolved + removed), with grab history and a manual Import Now
- [x] Automatic search for wanted items: periodic sweep (6h) + Search Wanted button + per-book Auto Grab; grabs the best approved release, skips books with pending grabs
- [x] Interactive search UI: per-book release candidates with scores/rejections and Grab buttons in the Library

### Phase 3 — Audiobooks
- [x] Audiobook library type with its own root folders, formats (m4b/m4a/mp3/flac/opus), and quality profiles
- [x] Edition awareness: a book's ebook and audiobook are owned and monitored independently (audiobook acquisition opts in via a monitored audiobook edition)
- [x] Audio-specific parsing: narrator ("read by"), bitrate, abridged (rejected by default), audio formats; multi-file books scanned and imported as one unit
- [x] Audiobookshelf-friendly folder layout (`Author/Book Title/` with tracks inside) *(sidecar metadata files come with Phase 5 polish)*

### Phase 4 — Manga, comics & magazines
- [x] Volume/issue data model: series-first — monitored series own their volumes/issues, browsable in the Series tab
- [x] Manga metadata provider: **AniList** (public API, no key; verified live) behind the series-provider registry
- [x] Comic metadata provider: **ComicVine** (free API key, entered in Settings) *(mock-tested; live verification pending a key)*
- [x] CBZ/CBR handling + `ComicInfo.xml` written into imported CBZ archives (CBR is read-only — pure Go can't write RAR)
- [x] Issue/volume monitoring: per-series "monitor future volumes" — refresh discovers new volumes and monitors them automatically
- [x] Kavita/Komga-friendly folder layouts (`Series/Series Vol. N.cbz`, templates editable)
- [x] Magazines as provider-less series: add by name, issues recognized by date/number parsing (ISO dates, "July 2026", "Issue 452")
- [x] Magazine scanning materializes owned issues; automatic search grabs new issues (capped per pass); `Magazine/Magazine - date.pdf` layout; indexer categories 7010

### Phase 5 — Polish & 1.0
- [ ] **Plex-style library layout**: a media type appears in the UI only once its library is set up (root folder added, or content already owned); each active library gets its own area — sidebar entry, scoped browsing (author-first for books, series-first for manga/comics/magazines), scoped search-and-add and wanted list; the Home page is the only place types meet, as stacked per-library sections ("Recently added — Ebooks", "Wanted — Manga") that never interleave types within a row; type-specific settings render only for active libraries
- [ ] **Explicit per-format library membership**: a book appears in the Audiobooks library only if you own or deliberately added its audiobook (and vice versa for ebooks) — never inferred from owning the other format. Membership is set by scanning (you own it), by which library you add it from, or by cross-add: a book's detail page shows that other formats exist, with an "Add to Audiobooks/Ebooks" button that prompts whether to monitor. This replaces the current edition-monitoring opt-in as the wanted signal
- [ ] Full settings UI as specced above, with Test buttons everywhere
- [ ] Failed-release blocklist: a release that failed to download is never grabbed again; search falls to the next candidate
- [ ] Health checks: background monitoring (root folder unreachable, indexer failing repeatedly, download client down, provider token invalid) with a warning banner in the UI
- [ ] Authentication: login page with username/password sessions (replacing the raw API-key prompt), API-key regeneration; SSL/reverse-proxy guidance
- [ ] Wanted page per library: everything missing, with search buttons
- [ ] Delete options: removing a book/author/series can optionally delete its files from disk (otherwise the next scan re-finds them as strays)
- [ ] Log file on disk (with rotation) + log viewer in the UI (System → events)
- [ ] Calendar view (upcoming releases across all libraries)
- [ ] Upgrade handling (replace when a better-quality release appears)
- [ ] Per-indexer failure backoff (rest an indexer that keeps erroring instead of hammering it every sweep)
- [ ] Seed-goal handling: qBittorrent ratio polling + remove-after-seeding
- [ ] Pagination/virtualized browsing in the new UI (built in from the start, not retrofitted)
- [ ] Backups + restore
- [ ] Windows installer/service, Linux packages/systemd, official Docker image — with real version stamping (ldflags) in CI release builds
- [ ] Docs site + API reference
- [ ] OPF sidecar files for Calibre/Audiobookshelf; rename/organize support for non-ebook libraries

### Post-1.0 ideas
- [ ] "Missing" view per author: a dropdown in the author area listing media you don't have — bibliography gaps from the metadata provider, organized (by series, then year) — with one-click add/monitor
- [ ] Manga colorized/monochrome variants: own both, with separate root folders per variant — but **one** shared Manga library (unlike ebook/audiobook, no split areas); every volume shows both variants with per-variant owned state, sharing the same series/volume metadata
- [ ] External notifications (Discord, webhook, email) on grab/import/upgrade/failure
- [ ] Multi-book archive imports (a "complete series" release currently imports only its largest file)
- [ ] Fuzzy / ISBN- and embedded-metadata-based file matching (scanning is exact-normalized-match today)
- [ ] `ComicInfo.xml` for CBR archives (needs a RAR writer)
- [ ] Import lists (Hardcover want-to-read shelf → auto-monitor)
- [ ] Multi-user / permissions
- [ ] Direct integrations: Calibre, Kavita, Komga, Audiobookshelf notify-on-import
- [ ] Additional metadata providers (Open Library, Google Books) as fallbacks
- [ ] Localization

---

## Status

🚧 **Pre-alpha — Phases 0–4 complete: all five media types work end-to-end.** Ebooks and audiobooks flow author-first from Hardcover; manga and comics flow series-first from AniList and ComicVine, with volumes/issues monitored per series ("monitor future volumes" included); magazines are provider-less periodicals added by name, with issues recognized by date. One acquisition pipeline serves everything: per-type indexer categories, release parsing that understands formats, narrators, volume numbers, and issue dates, quality profiles, qBittorrent/SABnzbd grabbing, and imports that land in reader-friendly layouts (Audiobookshelf for audio, Kavita/Komga for comics — with `ComicInfo.xml` written into CBZs). Hardcover and AniList are verified against their live APIs; Prowlarr, the download clients, and ComicVine are mock-tested and await live confirmation. Phase 5 (polish & 1.0 — full UI overhaul, upgrade handling, calendar, backups, packaging; external notifications deferred to post-1.0) is next.

## License

[GPL-3.0](LICENSE)
