# LibriNode

A self-hosted media automation server for **written media** — the Readarr / LazyLibrarian successor that treats ebooks, audiobooks, manga, comics, and magazines as first-class citizens.

LibriNode monitors your wanted list, searches your indexers, sends releases to your download client, then imports, renames, and organizes files into per-type libraries — automatically.

Runs on **Windows** and **Linux** (bare metal or Docker).

> 🚧 **Pre-1.0, but feature-complete.** Phases 0–5 are built: all five
> media types work end-to-end, from metadata search through automatic
> grabbing to organized imports, wrapped in the Plex-style UI with health
> checks, login, backups, calendar, and packaging —
> [see what works now](#getting-started-what-works-today). What stands
> between here and calling it 1.0 is hardening: real-world burn-in, live
> verification of the remaining integrations (Prowlarr, download clients,
> ComicVine), and code-signed release builds.

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
- Writes sidecar metadata for readers: `ComicInfo.xml` into imported CBZs (Kavita/Komga), and OPF sidecars for ebooks (Calibre) and audiobooks (Audiobookshelf)

### ⚙️ Clean, organized settings
Settings are grouped by concern, not dumped on one page:

```
Settings
├── Media Management     (root folders per type, file naming templates)
├── Libraries            (quality profiles per media type: formats, language, upgrades)
├── Metadata             (provider choice + API tokens: Hardcover, ComicVine)
├── Indexers             (manual Newznab/Torznab or Prowlarr sync; per-type categories under Advanced)
├── Download Clients     (qBittorrent, SABnzbd; category mapping under Advanced)
└── General              (login account, API-key regeneration, instance info; health, logs, and backups live on the System page)
```

Every settings page follows the same pattern: sensible defaults, a **Test**
button on every connection (both when adding and on every saved entry), and
advanced options hidden behind a toggle. A UI-preferences page (theme,
language, date formats) is planned post-1.0.

---

## Architecture

- **Backend:** **Go** — compiles to a single self-contained binary per OS, no runtime installs for the user
- **Frontend:** **React** SPA (Vite), embedded into the binary and served on one port (*arr-style)
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no cgo) with embedded schema migrations
- **API:** versioned REST API (`/api/v1`) with API-key auth — the same API the UI uses, so everything is scriptable; Prowlarr-compatible surface for app sync
- **Default port:** `7845`
- **Distribution:** Dockerfile + compose example (PUID/PGID conventions), systemd unit, Windows startup scripts, and a tag-triggered release workflow with version-stamped binaries today; code-signed installers, a native Windows service, and published images land with 1.0
- **License:** GPL-3.0 (same family as Sonarr/Radarr/Prowlarr)

## Getting started (what works today)

1. Build and run the server (see [Development](#development) below), then
   open `http://localhost:7845` and paste the API key from `config.yaml` in
   your data directory (or set a login account under **Settings → General →
   Security**, which replaces the key prompt with a sign-in page). The UI
   is Plex-style: a sidebar with **Home** plus
   one entry per library — and a library only appears once you've set it up
   (added its root folder, or already own content of that type). Home shows
   per-library Recently-added and Wanted rows with cover art; each library
   page is a *arr-style poster grid (authors for books, series for
   manga/comics/magazines, with owned/total counts on every card) with its
   own **+ Add**, scan, and search controls. For prose books, browsing goes
   three levels deep: the Ebooks/Audiobooks grid → an **author page**
   (portrait, bio, author-scoped Search wanted/Organize/Scan/Refresh/Remove,
   a poster grid of that author's monitored-or-owned books, and a
   **Missing** section below it listing the rest of the bibliography,
   grouped by series then year, with a one-click Monitor) → a **book page**
   (cover, description, monitor toggle, Auto grab/Search releases, and
   cross-add to the other format). Manga/comics/magazines stay two levels
   deep: the grid → a series page listing volumes/issues as rows with
   owned/wanted badges and per-volume Auto grab. Ebooks and Audiobooks are
   separate libraries throughout: a book (and its author) belongs only to
   the format libraries added or owned, and the book page's cross-add
   button switches to a status badge once it's in both.
2. **Settings → Metadata:** paste your
   [Hardcover API token](https://hardcover.app/account/api), hit **Test**,
   then **Save** — search goes live immediately, no restart.
3. **Settings → Media Management → Root Folders:** add the folder(s) where
   your media lives — one root per media type (ebook, audiobook, manga,
   comic, magazine).
4. **Search:** find authors or books on Hardcover and add them to the
   library. Adding an author pulls their full bibliography as metadata and
   makes the author a member of that library, but does **not** monitor every
   book — new books land in the author's **Missing** section for you to
   monitor selectively (owning a file enrolls its book automatically).
   Adding a specific book pulls its editions and monitors just that one.
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
   history, and format preferences live under **Settings → Libraries**
   (quality profiles).
8. Beyond the library pages: every library has a **Wanted** card with
   per-item search, the **Calendar** page lists dated releases across all
   libraries, and the **System** page carries health checks, backups (with
   staged restore), and the log viewer.

**Audiobooks:** add an audiobook root folder, and scanning understands both
`Author/Title.m4b` and multi-file `Author/Title/*.mp3` layouts (each book
folder is one unit). To acquire a book as an audiobook, add it to the
Audiobooks library — either add the author/book from the Audiobooks page,
or open the book in Ebooks and hit **+ Add to Audiobooks** (you'll be asked
whether to monitor it) — then wait for the automatic sweep or use the
book's Auto grab / Search releases from the Audiobooks side. Audiobook
searches use each indexer's **Audio categories** (default `3030`), and
imports land as `Author/Book Title/` folders — Audiobookshelf-ready.

**Manga & comics** are series-first: from the Manga or Comics library page,
search AniList (no key needed) or ComicVine (needs a free API key, entered
under **Settings → Metadata**), add the series, and its detail page lists
every volume/issue with owned/wanted badges and per-volume Auto grab.
"Monitor future volumes" is the series' monitor toggle — refreshes (manual
or the daily sweep) discover new volumes and start monitoring them. One
AniList quirk: *ongoing* manga often have no official volume count yet, so
they add with zero volumes and fill in once AniList publishes totals —
completed series (e.g. Death Note) arrive with all volumes immediately.
Manga/comic searches use each indexer's **Comic categories** (default
`7030`), scans understand `Series/Series v05.cbz` layouts, and imports
write `ComicInfo.xml` into CBZ archives for Kavita/Komga.

Manga can be owned in **colorized and monochrome** variants at once: it
stays one library, but you add a separate root folder per variant (a
monochrome/colorized selector appears when adding a manga root under
**Settings → Media Management**). Each volume tracks both variants
independently — the series page shows one owned/wanted badge plus a
`🎨 colorized` and/or `◻️ monochrome` flag for whichever copies are on disk
— while sharing a single series/volume metadata row.

**Magazines** work LazyLibrarian-style — periodicals have no metadata
provider, so you add one **by name** (Magazines library → **+ Add**). LibriNode
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

File naming templates live under **Settings → Media Management**. Tokens:
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

Full documentation lives in [`docs/`](docs/index.md) (`pip install
mkdocs-material && mkdocs serve` for the site).

API calls need the key from `config.yaml`:

```sh
curl -H "X-Api-Key: <key>" http://localhost:7845/api/v1/system/status
```

### API overview (v1)

Everything the UI does goes through `/api/v1` — same endpoints, fully
scriptable:

| Area | Endpoints |
|---|---|
| System | `GET /system/status`, `GET /ping` (no auth), `GET /health` (cached check results), `POST /health/check` (re-run now), `GET /log?lines=N` (tail the log file) |
| Auth | `GET /auth/status` + `POST /auth/login` (both unauthenticated), `POST /auth/logout`, `PUT /auth/credentials` (empty username disables), `POST /auth/apikey/regenerate` |
| Backups | `GET/POST /backup`, `DELETE /backup/{name}`, `POST /backup/{name}/restore` (staged, applied on restart), `GET /backup/{name}/download` |
| Root folders | `GET/POST /rootfolder` (manga roots take a `"variant"`: `color`\|`mono`, default `mono`), `DELETE /rootfolder/{id}` |
| Search | `GET /search?term=&type=author\|book\|manga\|comic` (metadata provider proxy) |
| Series | `GET/POST /series` (manga/comic by foreign id; magazines by `{"mediaType":"magazine","title":"..."}`), `GET/DELETE /series/{id}` (`?deleteFiles=true` also removes files), `PUT /series/{id}/monitor`, `POST /series/{id}/refresh` |
| Libraries | `GET /libraries` (which media types are set up), `GET /home` (per-library Recently-added/Wanted sections), `GET /wanted?library=X` (Wanted page), `GET /calendar?past=&days=` (dated releases) |
| Authors | `GET/POST /author` (`?library=` scopes; adds take `"library"`), `GET/DELETE /author/{id}` (`?deleteFiles=true` also removes files — deletes outright, every library), `PUT /author/{id}/library` (scoped add/remove from ONE format library; `deleteFiles` on remove; auto-deletes the author once they're in no library), `PUT /author/{id}/monitor`, `POST /author/{id}/refresh` (never changes membership or monitoring), `GET /author/{id}/missing?library=` (bibliography gaps), `POST /author/{id}/search?library=` (search this author's wanted books only) |
| Books | `GET/POST /book` (adds take `"library"`), `GET/DELETE /book/{id}` (`?deleteFiles=true` also removes files), `PUT /book/{id}/library` (per-format membership + monitored; `deleteFiles` removes that format's files on leave), `PUT /book/{id}/monitor`, `POST /book/{id}/refresh` |
| Files | `POST /library/scan`, `GET/POST /library/rename` (preview/apply; `?bookId=` or `?authorId=`/`{"authorId":N}` scopes, otherwise everything), `GET /bookfile?bookId=N\|unmatched=true`, `POST /bookfile/{id}/match`, `DELETE /bookfile/{id}` |
| Indexers | `GET/POST /indexer`, `GET/PUT/DELETE /indexer/{id}`, `GET /indexer/schema`, `POST /indexer/test`, `GET /release?term=` or `?bookId=N` (+ `&mediaType=ebook\|audiobook\|manga\|comic\|magazine`; volumes imply their own type) — parsed + scored candidates from all enabled indexers |
| Quality | `GET/POST /qualityprofile`, `PUT/DELETE /qualityprofile/{id}`, `PUT /qualityprofile/{id}/default` |
| Downloads | `GET/POST /downloadclient`, `PUT/DELETE /downloadclient/{id}`, `POST /downloadclient/test`, `POST /release/grab` (with `bookId` for auto-import), `GET /queue`, `POST /library/import`, `GET /history`, `GET /blocklist`, `DELETE /blocklist/{id}` |
| Auto search | `POST /book/{id}/search?mediaType=` (grab best release for one book), `POST /library/search` (sweep all wanted books and formats) |
| Settings | `GET/PUT /settings/metadata`, `POST /settings/metadata/test`, `GET/PUT /settings/naming` (templates for all five media types) |

`POST /author` takes `{"foreignAuthorId": "..."}` and pulls the full
bibliography as metadata; the author joins the target library, but no book
is enrolled or monitored — every book starts in Missing. `POST /book` takes
`{"foreignBookId": "..."}` and pulls one book with its editions, monitored
and enrolled in the target library, creating an unmonitored author stub if
one doesn't exist yet (the stub still joins the target library, via the
book's membership).

Metadata search and add require a [Hardcover](https://hardcover.app) API
token: paste it under **Settings → Metadata** in the web UI (it
takes effect immediately — no restart) or set `LIBRINODE_HARDCOVER_TOKEN`.
Tokens live under the `metadata:` section of `config.yaml`. Without one,
metadata endpoints return 503.

## Security & remote access

- **Login page:** set a username and password under **Settings → General →
  Security** and the UI switches from the API-key prompt to a login page.
  Sessions are 30-day cookies held in memory, so a server restart signs
  everyone out. Disable login in the same place.
- **API key:** always works for automation (Prowlarr sync, scripts) via the
  `X-Api-Key` header or `?apikey=`. Regenerate it under **Settings →
  General** — the old key stops working immediately, so update Prowlarr and
  any scripts right after.
- **Passwords** are stored only as PBKDF2-SHA256 hashes in `config.yaml`;
  failed login attempts are logged and throttled.
- **HTTPS:** LibriNode itself serves plain HTTP. For access beyond your LAN,
  put it behind a TLS-terminating reverse proxy **and enable the login**.
  Caddy makes it a two-liner (automatic certificates):

  ```
  librinode.example.com {
      reverse_proxy 127.0.0.1:7845
  }
  ```

  nginx equivalent:

  ```nginx
  server {
      listen 443 ssl;
      server_name librinode.example.com;
      # ssl_certificate / ssl_certificate_key ...
      location / {
          proxy_pass http://127.0.0.1:7845;
          proxy_set_header Host $host;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
      }
  }
  ```

  Never expose the raw HTTP port directly to the internet.

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
- [x] Add author or book → monitor wanted editions *(this acquisition signal was later replaced by explicit per-format library membership, Phase 5)*
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
- [x] Edition awareness: a book's ebook and audiobook are owned independently *(originally opted in via a monitored audiobook edition — later replaced by explicit per-format library membership, Phase 5)*
- [x] Audio-specific parsing: narrator ("read by"), bitrate, abridged (rejected by default), audio formats; multi-file books scanned and imported as one unit
- [x] Audiobookshelf-friendly folder layout (`Author/Book Title/` with tracks inside) *(the `metadata.opf` sidecar landed with Phase 5 polish, below)*

### Phase 4 — Manga, comics & magazines
- [x] Volume/issue data model: series-first — monitored series own their volumes/issues, browsable on the series' own page
- [x] Manga metadata provider: **AniList** (public API, no key; verified live) behind the series-provider registry
- [x] Comic metadata provider: **ComicVine** (free API key, entered in Settings) *(mock-tested; live verification pending a key)*
- [x] CBZ/CBR handling + `ComicInfo.xml` written into imported CBZ archives (CBR is read-only — pure Go can't write RAR)
- [x] Issue/volume monitoring: per-series "monitor future volumes" — refresh discovers new volumes and monitors them automatically
- [x] Kavita/Komga-friendly folder layouts (`Series/Series Vol. N.cbz`, templates editable)
- [x] Magazines as provider-less series: add by name, issues recognized by date/number parsing (ISO dates, "July 2026", "Issue 452")
- [x] Magazine scanning materializes owned issues; automatic search grabs new issues (capped per pass); `Magazine/Magazine - date.pdf` layout; indexer categories 7010

### Phase 5 — Polish & 1.0
- [x] **Plex-style library layout**: a media type appears in the UI only once its library is set up (root folder added, or content already owned); each active library gets its own sidebar area with *arr-style browsing — a poster grid (author-first for books, series-first for manga/comics/magazines, owned/total counts on each card), scoped add-and-search, and unmatched files. Prose books browse three levels deep: library grid → author page (bio, author-scoped actions, a poster grid of that author's books, and Missing below it) → book page (cover, description, acquisition controls); manga/comics/magazines stay two levels deep, with volumes/issues as rows on the series page. The Home page is the only place types meet, as stacked per-library sections (Recently added / Wanted with cover art) that never interleave types; type-specific settings render only for active libraries
- [x] **Explicit per-format library membership**: both authors and books carry their own independent membership per format library — an author (and their books) appears in Audiobooks only if you added them there or own an audiobook of theirs (and vice versa for ebooks); adding/removing in one format never touches the other. Book membership is set by scanning/importing (owning it), by which library you add from, or by cross-add from the book detail ("Add to Audiobooks" with a monitor prompt); each book membership has its own monitored flag, replacing edition monitoring as the wanted signal. A library's Books grid lists only the books you've actually added — monitored or owned in that format; unmonitored, unowned members stay enrolled but hidden, surfaced instead in the author's Missing section. Refreshing metadata never enrolls, un-enrolls, or re-monitors anything — it only updates descriptions/covers/new-book metadata
- [x] Full settings UI as specced above: grouped pages (Media Management / Libraries / Metadata / Indexers / Download Clients / General) with Test buttons on every connection — including saved indexers and download clients — advanced options behind toggles, and a General page with instance info and per-browser API key. UI-preferences page (theme/language/dates) deferred post-1.0
- [x] Failed-release blocklist: a release that failed to download is never grabbed again (matched by guid or title); search falls to the next candidate, and entries can be removed from the Activity tab
- [x] Health checks: background monitoring every 15 minutes — root folder unreachable, indexer failing its connection check, download client down or misconfigured, metadata provider token invalid, plus warnings when no indexer/download client/provider is set up at all. Issues show as a warning banner on every page and in a System-page Health card with a run-now button (`GET /health`, `POST /health/check`)
- [x] Authentication: optional login account (Settings → General → Security) switches the UI from the API-key prompt to a username/password login page with 30-day cookie sessions (in-memory — a restart signs everyone out); passwords stored as PBKDF2-SHA256 hashes only; failed logins logged and throttled; the API key keeps working for Prowlarr/scripts and can be regenerated from the UI; SSL/reverse-proxy guidance in the README
- [x] Wanted page per library: every library page carries a Wanted card — monitored but missing that format's file — with per-item Auto grab (`GET /wanted?library=X`)
- [x] Delete options: removing an author/series — or taking a book out of a format library — can optionally delete the files from disk via an opt-in checkbox in the removal panel (`?deleteFiles=true` on DELETE endpoints, `deleteFiles` on `PUT /book/{id}/library`; book-level removal deletes only that format's files). Only paths inside a configured root folder are ever touched, emptied folders are pruned up to (never including) the root, and without the option the next scan re-finds the files as strays
- [x] Log file on disk with size rotation (`<data>/logs/librinode.log`, 5 MB, 3 old files kept) + log viewer on the System page: tail up to 2000 lines with a text filter and refresh (`GET /log?lines=N`)
- [x] Calendar view: agenda-style sidebar page — dated releases across all libraries, Upcoming and Recently released, grouped by date with owned/wanted badges (`GET /calendar?past=&days=`)
- [x] Upgrade handling: profiles with "allow upgrades" keep owned books wanted until the cutoff format (default: the profile's best); upgrade searches only approve strictly better formats, and imports replace the old file on disk and in the library
- [x] Per-indexer failure backoff: a failing indexer rests 5 minutes, doubling per consecutive failure up to 6 hours, with a resting-until notice in search errors; one success clears it
- [x] Seed-goal handling: configure ratio/time goals in qBittorrent; when it pauses a finished torrent (goal met), LibriNode removes imported grabs' torrents with their data — foreign torrents in the category are never touched
- [x] Scalable browsing: library grids over 10 cards get a client-side filter box and incremental rendering (60 cards + Show more), keeping large libraries responsive
- [x] Backups + restore: System-page card zips a consistent database snapshot (VACUUM INTO) + config.yaml into `<data>/backups`, with download/delete; restore stages the files and applies them on the next start, keeping the replaced ones as `*.pre-restore`
- [x] Packaging: multi-stage Dockerfile (PUID/PGID entrypoint) + compose example, systemd unit with hardening, Windows install/uninstall scripts (Task-Scheduler startup), and a tag-triggered release workflow building version-stamped binaries (ldflags) for linux amd64/arm64 + windows amd64. *Still for 1.0: code-signed Windows installer with a native service, published Docker images*
- [x] Docs site + API reference: `docs/` (mkdocs-material) — installation, quickstart, libraries, acquisition, configuration, full API table, development guide
- [x] OPF sidecar files: imports write `metadata.opf` into audiobook folders (Audiobookshelf) and `<file>.opf` beside ebooks (Calibre) — title, author, description, ISBN/language, calibre:series. Rename/organize now covers every media type: series templates for manga/comics/magazines, multi-file audiobooks moving as whole folders with their sidecars, and per-type templates all editable in Settings → Media Management
- [x] "Missing" view per author *(pulled forward from post-1.0)*: the author page ends with a Missing section listing bibliography gaps from the metadata provider — books neither monitored nor owned in that format library — grouped by series (ordered by position) then standalones by year; rows expand to a compact thumbnail + blurb, and a one-click **+ Monitor** adds the book to the library and starts searching (`GET /author/{id}/missing?library=`). Adding an author pulls their bibliography as metadata only — no book is auto-monitored or auto-enrolled, so a freshly added author's whole bibliography starts here in Missing, and an author with zero visible books still shows (an empty Books grid pointing at Missing) rather than disappearing
- [x] Author page actions are author-scoped: **Search wanted**, **Organize…**, and **Scan files** in the author header only touch that author's books (`POST /author/{id}/search?library=`, `GET/POST /library/rename?authorId=`/`{"authorId":N}`); **Remove from Ebooks/Audiobooks** takes the author out of one format library only (the other is untouched) with an opt-in delete-files checkbox, auto-deleting the author outright once they're in no library left
- [x] Manga colorized/monochrome variants *(pulled forward from post-1.0)*: manga stays **one** library (unlike the ebook/audiobook split) with a variant as a sub-dimension of its files. Each manga root folder is tagged colorized or monochrome (a variant selector appears when adding a manga root; monochrome is the default and existing roots backfill there), and a volume tracks each variant's ownership independently while sharing one series/volume metadata row — the series page shows one owned/wanted badge plus a colorized and/or monochrome flag for whichever copies are on disk. Imports stay variant-agnostic (a release doesn't reveal its variant); the scanner records per-variant ownership as files land under their variant root

### Post-1.0 ideas
- [ ] Multi-book archive imports (a "complete series" release currently imports only its largest file)
- [ ] Fuzzy / ISBN- and embedded-metadata-based file matching (scanning is exact-normalized-match today)
- [ ] `ComicInfo.xml` for CBR archives (needs a RAR writer)
- [ ] Multi-user / permissions
- [ ] Additional metadata providers (Open Library, Google Books) as fallbacks
- [ ] Localization

### Maybe ideas
- [ ] Direct integrations: Calibre, Kavita, Komga, Audiobookshelf notify-on-import
- [ ] Import lists (Hardcover want-to-read shelf → auto-monitor)
- [ ] External notifications (Discord, webhook, email) on grab/import/upgrade/failure

---

## Status

🚧 **Pre-1.0 — Phases 0–5 built, hardening remains.** All five media types work end-to-end: ebooks and audiobooks flow author-first from Hardcover into separate per-format libraries with explicit membership; manga and comics flow series-first from AniList and ComicVine, with volumes/issues monitored per series ("monitor future volumes" included) and manga ownable in colorized and monochrome variants at once; magazines are provider-less periodicals added by name, with issues recognized by date. One acquisition pipeline serves everything: per-type indexer categories with failure backoff, release parsing that understands formats, narrators, volume numbers, and issue dates, quality profiles with upgrade handling, a failed-release blocklist, qBittorrent/SABnzbd grabbing with seed-goal cleanup, and imports that land in reader-friendly layouts (Audiobookshelf folders with `metadata.opf` for audio, Kavita/Komga layouts with `ComicInfo.xml` for comics, OPF sidecars for Calibre) — with rename/organize covering every type. The UI is Plex-style (sidebar libraries, filterable poster grids, detail pages, grouped settings) with per-library Wanted pages, per-author Missing sections with author-scoped actions, a release calendar, health-check banners, an optional login, delete-from-disk options, backups with staged restore, and a log viewer; packaging (Docker, systemd, Windows scripts, release CI) and a docs site are in the repo. Hardcover and AniList are verified against their live APIs; Prowlarr, the download clients, and ComicVine are mock-tested and await live confirmation. **1.0 waits on**: real-world burn-in, those live verifications, and code-signed installers/published images — external notifications and the rest of the [Post-1.0 ideas](#post-10-ideas) stay parked for now.

## License

[GPL-3.0](LICENSE)
