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
- **LazyLibrarian** covers a lot (including magazines, which LibriNode also manages — organize-only for now) but has an aging UI and inconsistent metadata.
- **Mylar** does comics only; **Kavita/Komga** are readers, not automation.

Nothing today automates **all five** written-media types in one app with modern metadata (Hardcover) and clean *arr-style integrations. That's the gap LibriNode fills.

## Core Features

### 📚 Five media types, five libraries
Each media type is a fully independent library with its **own root folder(s)**, naming scheme, quality profile, and monitoring rules — and, Plex-style, a library only appears in the UI once you create it by adding its root folder:

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
- Pluggable provider architecture: manga comes from **AniList** (no key) or **Hardcover** (selectable), comics from **Hardcover** (default) or **ComicVine**, all behind the series-provider interface; more sources can follow
- Global metadata preferences — **language** (default English), **country** (default United States), **include adult content** (default off) — honored by every provider that carries the data, with strict→lenient fallback (e.g. Hardcover picks the edition matching your language, then the standard printing, then your country, then the richest description). Metadata only: what to *grab* stays the quality profiles' job
- Every provider selector includes **None (disabled)** — and libraries always honor the settings: under None, nothing is fetched, not even on refresh. The escape hatch is the per-record **provider override** on each author and series page (off by default): pin a record to a provider and its refreshes use that provider, beating the global selection — including None
- Metadata refresh on schedule + manual refresh
- Writes sidecar metadata for readers: `ComicInfo.xml` into imported CBZs (Kavita/Komga), and OPF sidecars for ebooks (Calibre) and audiobooks (Audiobookshelf)
- Caches provider cover/portrait art locally (downloaded on add/refresh), so the UI serves images from LibriNode instead of the provider CDN — and they survive the provider's link rot

### ⚙️ Clean, organized settings
Settings are grouped by concern, not dumped on one page:

```
Settings
├── Media Management     (root folders per type, file naming templates)
├── Quality Profiles     (per media type: formats, language, upgrades)
├── Metadata             (provider choice + API tokens: Hardcover, ComicVine)
├── Indexers             (manual Newznab/Torznab or Prowlarr sync; per-type categories under Advanced)
├── Download Clients     (qBittorrent, SABnzbd; category mapping under Advanced)
└── General              (login accounts/users, API-key regeneration, instance info; health, logs, and backups live on the System page)
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
   open `http://localhost:7845`. A fresh instance opens a **first-run setup
   wizard** — create an account (no API key needed) and it guides you through
   the rest; otherwise paste the API key from `config.yaml` in your data
   directory (or add a login account under **Settings → General → Security**,
   which replaces the key prompt with a sign-in page). The UI
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
   library. Adding an author pulls their bibliography as metadata — the
   canonical works by Hardcover readership, not a random slice of reprints —
   and makes the author a member of that library, but does **not** monitor
   every book — new books land in the author's **Missing** section for you to
   monitor selectively (owning a file enrolls its book automatically).
   Adding a specific book pulls its editions and monitors just that one.
5. **Library → Scan files:** match files you already own to library books —
   every book shows an **owned**/**wanted** badge. Strays land in an
   unmatched list with the library's best guess and a 0–100% confidence
   rating: import one in a click (or every confident match at once), resolve
   duplicates with Replace/Delete, and add a missing author, series, or
   magazine right from the row. Exact-named strays still attach automatically
   the moment you add their book from Search.
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
   history, and format preferences live under **Settings → Quality Profiles**
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
search the provider, add the series, and its page lists every volume/issue
with owned/wanted badges. Manga metadata can come from **AniList** (no key)
or **Hardcover** (reuses your Hardcover token); comic metadata from
**Hardcover** (the default) or **ComicVine** (free API key) — pick each
provider under **Settings → Metadata**.
Switching a provider re-sources existing series on their next
refresh: LibriNode re-matches each series on the newly selected provider by
title, re-binds it in place (keeping your monitoring and owned files), and
pulls that provider's volumes — so moving from AniList to Hardcover swaps in
Hardcover's real per-volume descriptions. Like adding an author, **adding a
series pulls metadata only**: every volume/issue starts unmonitored in the
series' Missing section (and a fresh magazine doesn't auto-grab), until you
monitor items selectively or flip the series' monitor toggle — which
monitors every volume at once and doubles as "monitor future volumes", so
refreshes (manual or the periodic sweep — every 30 days by default, tunable
in Settings) monitor newly discovered ones too. Provider quirks: *ongoing* manga on
AniList often report no volume count yet (they add with zero volumes and fill
in on later refresh) and synthesize volumes without per-volume descriptions
(left blank rather than repeating the series blurb); Hardcover carries real
per-volume descriptions and covers, numbered by the series' volume positions
(spin-offs and reissue/box-set editions dropped, one standard edition kept
per volume), falling back to sequential order only when a series has no
positions at all. Manga/comic searches use each indexer's
**Comic categories** (default `7030`), scans understand `Series/Series
v05.cbz` layouts, and imports write `ComicInfo.xml` into CBZ archives for
Kavita/Komga.

Manga and comics get the full author/book treatment. The series page carries
series-scoped **Search wanted**, **Organize…**, **Scan files**, and
**Refresh** actions (each touches only this series). Its volume/issue list
stays compact — title + owned/wanted badge — and every row expands to a
cover, blurb, file locations, and the same controls an individual book has:
a monitor toggle, **Auto grab**, **Search releases**, and **Remove from
library** (with an opt-in delete-files). Volume/issue covers default to the
provider's art; a per-library **Settings → Metadata** toggle switches manga
or comics to extract the cover from the owned
file's first page instead — both CBZ and CBR (read via pure-Go rardecode),
falling back to the provider's art when extraction yields nothing. Below the
list, a per-series **Missing** section lists the volumes/issues you're not
tracking — neither monitored nor owned — each with a one-click **Monitor**
to add it back, mirroring the per-author Missing view.

Manga can also be owned in **colorized and monochrome** variants at once: it
stays one library, but you add a separate root folder per variant (a
monochrome/colorized selector appears when adding a manga root under
**Settings → Media Management**). Each volume tracks both variants
independently while sharing a single metadata row; expanding an owned volume
shows which variants it owns (`🎨 colorized` / `◻️ monochrome`) and where
each file lives on disk.

**Magazines** work LazyLibrarian-style — periodicals have no metadata
provider, so you add one **by name** (Magazines library → **+ Add**). LibriNode
then recognizes issues by date or number in file names
(`The Economist - 2026-07-04.pdf`, `Retro Gamer Issue 261`): scanning a
magazine root materializes owned issues into the library automatically, and
the existing-file import adopts strays (fuzzy-named files, unknown magazines
added by name in one click). **The magazine library is organize-only for
now** — searching and downloading are disabled everywhere (the magazine
usenet landscape proved to be mostly disguised malware), and the search/grab
engine stays in the tree for when acquisition returns. Organized issues land
as `Magazine/Magazine - date.pdf`.

**Indexers** can be added two ways: manually under **Settings → Indexers**
(any Newznab/Torznab endpoint, including per-indexer feed URLs from
Prowlarr or Jackett), or automatically via **Prowlarr application sync** —
in Prowlarr, add an application of type **Readarr** with LibriNode's URL
(`http://localhost:7845`) and API key, and Prowlarr will push its indexers —
both Newznab (usenet) and Torznab (torrent) — into LibriNode and keep them in
sync. LibriNode emulates the Readarr v1 API completely enough for Prowlarr's
sync: the indexer endpoints accept both LibriNode's native JSON and Readarr
resources, and the capability endpoints Prowlarr reads during sync — root
folders, quality profiles, **metadata profiles** (a Readarr-only concept),
download clients (which advertise `protocol` so torrent indexers sync), and
`system/status` — return Readarr-shaped resources to Prowlarr while the web UI
keeps its native shapes.

File naming templates live under **Settings → Media Management**. Tokens:
`{Author Name}`, `{Author SortName}`, `{Book Title}`, `{Series Title}`,
`{Series Position}`, `{Series Position 00}` (zero-padded so `Vol. 01` sorts
before `Vol. 10`), `{Release Year}` — tokens without a value drop out
cleanly, and folder templates may span levels with `/` (an empty level drops
away). The defaults keep filenames informative on their own: each ebook gets
its own folder, `Frank Herbert/Dune (1965)/Frank Herbert - Dune 1 - Dune
(1965).epub` (standalones: `Andy Weir/The Martian (2011)/Andy Weir - The
Martian (2011).epub`); audiobook book-folders carry series and year; manga
and comics render `Series Vol. 01 (2003).cbz` / `Series #01 (2002).cbz`; and
magazines file under per-year subfolders, `The Economist/2025/…`.

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
| System | `GET /system/status`, `GET /ping` (no auth), `GET /health` (cached check results), `POST /health/check` (re-run now), `GET /log?lines=N` (tail the log file), `GET /image?url=` (cached provider-image proxy; redirects to origin on miss+failure) |
| Auth | `GET /auth/status` + `POST /auth/login` (both unauthenticated), `POST /auth/logout`, `PUT /auth/credentials` (create/change one account; empty username removes all), `GET/POST /auth/users`, `DELETE /auth/users/{username}` (default user protected), `PUT /auth/users/{username}/password`, `PUT /auth/users/{username}/default`, `POST /auth/apikey/regenerate` |
| Setup | `GET /setup/status` (unauthenticated — fresh instance?), `POST /auth/setup` (first-run wizard claims a fresh instance and creates the default account, no API key) |
| Backups | `GET/POST /backup`, `DELETE /backup/{name}`, `POST /backup/{name}/restore` (staged, applied on restart), `GET /backup/{name}/download` |
| Root folders | `GET/POST /rootfolder` (manga roots take a `"variant"`: `color`\|`mono`, default `mono`), `DELETE /rootfolder/{id}`, `GET /filesystem?path=` (visual folder picker: a directory's subfolders + parent; empty path = filesystem root, or drive list on Windows) |
| Search | `GET /search?term=&type=author\|book\|manga\|comic` (metadata provider proxy) |
| Series | `GET/POST /series` (manga/comic by foreign id; magazines by `{"mediaType":"magazine","title":"..."}`; adds pull metadata only — volumes start unmonitored unless `"monitored":true`), `GET/DELETE /series/{id}` (`?deleteFiles=true` also removes files), `PUT /series/{id}/monitor`, `PUT /series/{id}/provider` (per-series provider override; `""` follows settings), `POST /series/{id}/refresh`, `POST /series/{id}/search` (search this series' wanted volumes only) |
| Libraries | `GET /libraries` (which media types are set up), `GET /home` (per-library Recently-added/Wanted sections), `GET /wanted?library=X` (Wanted page), `GET /calendar?past=&days=` (dated releases) |
| Authors | `GET/POST /author` (`?library=` scopes; adds take `"library"`), `GET/DELETE /author/{id}` (`?deleteFiles=true` also removes files — deletes outright, every library), `PUT /author/{id}/library` (scoped add/remove from ONE format library; `deleteFiles` on remove; auto-deletes the author once they're in no library), `PUT /author/{id}/monitor`, `PUT /author/{id}/provider` (per-author provider override; `""` follows settings), `POST /author/{id}/refresh` (never changes membership or monitoring), `GET /author/{id}/missing?library=` (bibliography gaps), `POST /author/{id}/search?library=` (search this author's wanted books only) |
| Books | `GET/POST /book` (adds take `"library"`), `GET/DELETE /book/{id}` (`?deleteFiles=true` also removes files), `GET /book/{id}/cover` (cover image extracted from the owned CBZ/CBR's first page; cached under `<data>/covers`, refreshed when the file changes), `PUT /book/{id}/library` (per-format membership + monitored; `deleteFiles` removes that format's files on leave; `library:"manga"`/`"comic"` adds/removes a volume/issue from its series — `member:false` forgets its file records so it drops to Missing), `PUT /book/{id}/monitor`, `POST /book/{id}/refresh` |
| Files | `POST /library/scan`, `GET/POST /library/rename` (preview/apply; `?bookId=`, `?authorId=`/`{"authorId":N}`, or `?seriesId=`/`{"seriesId":N}` scopes, otherwise everything), `GET /bookfile?bookId=N\|unmatched=true`, `GET /bookfile/unmatched/options?mediaType=` (existing-file import: per-file suggestions with confidence, candidates, duplicates — all five types), `POST /bookfile/import-matched` (bulk-import every confident match), `POST /bookfile/{id}/match` (`{"bookId":N}`, or `{"seriesId":N,"issue":"…"}` to materialize a magazine issue), `POST /bookfile/{id}/replace` (swap an owned file for this one; manga replaces only the matching variant), `DELETE /bookfile/{id}` (`?deleteFiles=true` also deletes from disk), `DELETE /library/covers/cache` (clear extracted comic covers) |
| Indexers | `GET/POST /indexer`, `GET/PUT/DELETE /indexer/{id}`, `GET /indexer/schema`, `POST /indexer/test`, `GET /release?term=` or `?bookId=N` (+ `&mediaType=ebook\|audiobook\|manga\|comic`; volumes imply their own type; magazines are rejected — organize-only) — parsed + scored candidates from all enabled indexers |
| Quality | `GET/POST /qualityprofile`, `PUT/DELETE /qualityprofile/{id}`, `PUT /qualityprofile/{id}/default` |
| Downloads | `GET/POST /downloadclient`, `PUT/DELETE /downloadclient/{id}`, `POST /downloadclient/test`, `POST /release/grab` (with `bookId` for auto-import), `GET /queue` (items enriched with their book/grab + live progress; short-cached snapshot shared across pollers), `DELETE /queue/{clientId}/{itemId}` (remove one download + its data, not blocklisted), `POST /library/import`, `GET /history`, `GET /blocklist`, `DELETE /blocklist/{id}` |
| Auto search | `POST /book/{id}/search?mediaType=` (grab best release for one book), `POST /library/search` (sweep all wanted books and formats) |
| Settings | `GET/PUT /settings/metadata`, `POST /settings/metadata/test`, `DELETE /settings/metadata/cache` (clear provider images), `DELETE /settings/metadata/descriptions` (blank stored descriptions; re-fetched on refresh), `DELETE /cache` (clear all: provider art + extracted covers + descriptions), `GET/PUT /settings/naming` (templates for all five media types), `GET/PUT /settings/import` (import handling: pack import, remove-from-client, delete-files) |

`POST /author` takes `{"foreignAuthorId": "..."}` and pulls the
bibliography as metadata (the most-read entries on Hardcover — canonical
works first, zero-reader reprints skipped); the author joins the target
library, but no book is enrolled or monitored — every book starts in
Missing. `POST /book` takes
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

- **Login page:** add a user under **Settings → General → Security** and the
  UI switches from the API-key prompt to a login page. Manage multiple
  accounts there — change password, add/remove users, and pick the protected
  default (which can't be removed until another user is promoted). A fresh
  install offers a first-run setup wizard that creates the first account
  without needing the API key. Sessions are 30-day cookies held in memory, so
  a server restart signs everyone out. Disable login (removes all accounts) in
  the same place.
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
- [x] **Prowlarr application sync** — add LibriNode to Prowlarr as a *Readarr* application; Prowlarr pushes/updates/removes indexers automatically *(Readarr v1 API emulation; **verified live against a real Prowlarr** — syncs both Newznab and Torznab indexers. Getting there required completing the emulation: the capability endpoints Prowlarr reads mid-sync (root folders, quality profiles, the Readarr-only metadata-profile endpoint, and download clients carrying `protocol`) must return Readarr-shaped resources or Prowlarr's `BuildReadarrIndexer` throws a NullReferenceException and refuses torrent indexers)*
- [x] Release parsing + scoring (formats, retail, language, year, scene names; book-aware search rejects wrong author/title/dead torrents and ranks the rest; manga/comic/magazine/audiobook release names may omit the file format — accepted, with the real format read from the downloaded files at import, while ebooks still require a named format; series/magazine title matching is whole-word so a short title like "Saga" doesn't match "Saga of the Swamp Thing")
- [x] Quality profiles per library (ordered format preferences, language, size bounds; default profile drives search scoring; managed in Settings)
- [x] **qBittorrent** client: add, track, remove (category-scoped; seed goals with CDH). LibriNode resolves the release its side — magnet pass-through, or follow the indexer URL to its magnet, else fetch the `.torrent` and upload the bytes — so a NAT'd client or a qBittorrent-compatible **debrid bridge** (Real-Debrid/TorBox) works; a slow bridge's add is confirmed against the torrent list so a late response never loses the grab
- [x] **SABnzbd** client: add, track, remove (category-scoped; post-process hand-off with CDH). LibriNode fetches the NZB and uploads the file (`addfile`) rather than handing over a URL, so a SABnzbd-compatible debrid bridge that can't reach your LAN indexers still gets the download
- [x] Completed Download Handling: finished grabs import automatically (into the naming-template layout, with grab history and a manual Import Now; failed downloads resolved + removed). Three **Import handling** options (Settings → Download Clients), **all on by default**, govern the aftermath — import whole packs, remove the completed download from the client, and delete the downloaded files (`import.*`); turn the latter two off to keep torrents seeding and leave originals in place (usenet history is cleared either way, since LibriNode only copies from it)
- [x] Multi-book pack imports *(pulled forward from post-1.0)*: when a grabbed release is a bundle ("complete series"), the grabbed book's file is matched by volume number (manga/comics) or title (ebooks) — never by size, so a v01–v12 pack can't file volume 12 as the volume you grabbed. With **Import whole packs** on (the default, `import.pack_import_all`) the pack's other files fill every book they match — imported ebooks join their format library like scanned files, though nothing gets monitored automatically; turn it off to fill only the grabbed book plus other **monitored** books. Either way owned books are only replaced by genuine quality upgrades
- [x] Automatic search for wanted items: periodic sweep (6h) + Search Wanted button + per-book Auto Grab; grabs the best approved release, skips books with pending grabs
- [x] Interactive search UI: per-book release candidates with scores/rejections and Grab buttons in the Library

### Phase 3 — Audiobooks
- [x] Audiobook library type with its own root folders, formats (m4b/m4a/mp3/flac/opus), and quality profiles
- [x] Edition awareness: a book's ebook and audiobook are owned independently *(originally opted in via a monitored audiobook edition — later replaced by explicit per-format library membership, Phase 5)*
- [x] Audio-specific parsing: narrator ("read by"), bitrate, abridged (rejected by default), audio formats; multi-file books scanned and imported as one unit
- [x] Audiobookshelf-friendly folder layout (`Author/Book Title/` with tracks inside) *(the `metadata.opf` sidecar landed with Phase 5 polish, below)*

### Phase 4 — Manga, comics & magazines
- [x] Volume/issue data model: series-first — monitored series own their volumes/issues, browsable on the series' own page
- [x] Manga metadata provider: **AniList** (public API, no key; verified live) behind the series-provider registry; **Hardcover** later became a selectable alternative (same token as books; verified live) — the manga provider is chosen in Settings → Metadata, switching it re-sources existing series on their next refresh (re-matched by title, monitoring and owned files kept), and Hardcover volumes carry real per-volume descriptions/covers with spin-offs and reissue editions filtered out
- [x] Comic metadata provider: **ComicVine** (free API key, entered in Settings) *(mock-tested; live verification pending a key)*
- [x] CBZ/CBR handling + `ComicInfo.xml` written into imported CBZ archives (CBR is write-only-blocked — pure Go can't write RAR — but is read for cover extraction via rardecode); volume covers are pulled from the owned archive's first page (`GET /book/{id}/cover`)
- [x] Issue/volume monitoring: per-series "monitor future volumes" — refresh discovers new volumes and monitors them automatically. *(Series adds later became metadata-only, like author adds: volumes start unmonitored in the series' Missing section, and the series toggle is the bulk opt-in that also enables monitor-future-volumes)*
- [x] Kavita/Komga-friendly folder layouts (`Series/Series Vol. N.cbz`, templates editable)
- [x] Magazines as provider-less series: add by name, issues recognized by date/number parsing (ISO dates, "July 2026", "Issue 452")
- [x] Magazine scanning materializes owned issues; automatic search grabs new issues (capped per pass); `Magazine/Magazine - date.pdf` layout; indexer categories 7010 *(magazine acquisition was later disabled — the library is organize-only for now, the search engine kept in the tree; see Status)*

### Phase 5 — Polish & 1.0
- [x] **Plex-style library layout**: a media type appears in the UI only once the user creates its library by adding a root folder (content alone never surfaces one); each active library gets its own sidebar area with *arr-style browsing — a poster grid (author-first for books, series-first for manga/comics/magazines, owned/total counts on each card), scoped add-and-search, and unmatched files. Prose books browse three levels deep: library grid → author page (bio, author-scoped actions, a poster grid of that author's books, and Missing below it) → book page (cover, description, acquisition controls); manga/comics/magazines stay two levels deep, with volumes/issues as rows on the series page. The Home page is the only place types meet, as stacked per-library sections (Recently added / Wanted with cover art) that never interleave types; type-specific settings render only for active libraries
- [x] **Explicit per-format library membership**: both authors and books carry their own independent membership per format library — an author (and their books) appears in Audiobooks only if you added them there or own an audiobook of theirs (and vice versa for ebooks); adding/removing in one format never touches the other. Book membership is set by scanning/importing (owning it), by which library you add from, or by cross-add from the book detail ("Add to Audiobooks" with a monitor prompt); each book membership has its own monitored flag, replacing edition monitoring as the wanted signal. A library's Books grid lists only the books you've actually added — monitored or owned in that format; unmonitored, unowned members stay enrolled but hidden, surfaced instead in the author's Missing section. Refreshing metadata never enrolls, un-enrolls, or re-monitors anything — it only updates descriptions/covers/new-book metadata
- [x] Full settings UI as specced above: grouped pages (Media Management / Quality Profiles / Metadata / Indexers / Download Clients / General — the profiles group was named "Libraries" until it proved to hold only profiles) with Test buttons on every connection — including saved indexers and download clients — advanced options behind toggles, and a General page with instance info and per-browser API key. UI-preferences page (theme/language/dates) deferred post-1.0
- [x] Failed-release blocklist: a release that failed to download is never grabbed again (matched by guid or title); search falls to the next candidate, and entries can be removed from the Activity tab
- [x] Health checks: background monitoring every 15 minutes — root folder unreachable, indexer failing its connection check, download client down or misconfigured, metadata provider token invalid, plus warnings when no indexer/download client/provider is set up at all. Issues show as a warning banner on every page and in a System-page Health card with a run-now button (`GET /health`, `POST /health/check`)
- [x] Authentication: optional login accounts (Settings → General → Security) switch the UI from the API-key prompt to a username/password login page with 30-day cookie sessions (in-memory — a restart signs everyone out); passwords stored as PBKDF2-SHA256 hashes only; failed logins logged and throttled; the API key keeps working for Prowlarr/scripts and can be regenerated from the UI; SSL/reverse-proxy guidance in the README. *(Later extended to **multiple users** with a compact management UI — change password, add/remove, and a protected default account — and a **first-run setup wizard** that claims a fresh instance without the API key, plus a **visual folder browser** for picking root folders.)*
- [x] Wanted page per library: every library page carries a Wanted card — monitored but missing that format's file — with per-item Auto grab (`GET /wanted?library=X`)
- [x] Delete options: removing an author/series — or taking a book out of a format library — can optionally delete the files from disk via an opt-in checkbox in the removal panel (`?deleteFiles=true` on DELETE endpoints, `deleteFiles` on `PUT /book/{id}/library`; book-level removal deletes only that format's files). Only paths inside a configured root folder are ever touched, emptied folders are pruned up to (never including) the root, and without the option the next scan re-finds the files as strays
- [x] Log file on disk with size rotation (`<data>/logs/librinode.log`, 5 MB, 3 old files kept) + log viewer on the System page: tail up to 2000 lines with a text filter and refresh (`GET /log?lines=N`)
- [x] Calendar view: agenda-style sidebar page — dated releases across all libraries, Upcoming and Recently released, grouped by date with owned/wanted badges (`GET /calendar?past=&days=`)
- [x] Upgrade handling: profiles with "allow upgrades" keep owned books wanted until the cutoff format (default: the profile's best); upgrade searches only approve strictly better formats, and imports replace the old file on disk and in the library
- [x] Per-indexer failure backoff: a failing indexer rests 5 minutes, doubling per consecutive failure up to 6 hours, with a resting-until notice in search errors; one success clears it
- [x] Seed-goal handling: with *remove completed downloads* off, configure ratio/time goals in qBittorrent; when it pauses a finished torrent (goal met), LibriNode removes imported grabs' torrents with their data — foreign torrents in the category are never touched (with remove-completed on, the default, an imported torrent is removed right away instead)
- [x] Scalable browsing: library grids over 10 cards get a client-side filter box and incremental rendering (60 cards + Show more), keeping large libraries responsive
- [x] Backups + restore: System-page card zips a consistent database snapshot (VACUUM INTO) + config.yaml into `<data>/backups`, with download/delete; restore stages the files and applies them on the next start, keeping the replaced ones as `*.pre-restore`
- [x] Packaging: multi-stage Dockerfile (PUID/PGID entrypoint) + compose example, systemd unit with hardening, Windows install/uninstall scripts (Task-Scheduler startup), and a tag-triggered release workflow building version-stamped binaries (ldflags) for linux amd64/arm64 + windows amd64. *Still for 1.0: code-signed Windows installer with a native service, published Docker images*
- [x] Docs site + API reference: `docs/` (mkdocs-material) — installation, quickstart, libraries, acquisition, configuration, full API table, development guide
- [x] OPF sidecar files: imports write `metadata.opf` into audiobook folders (Audiobookshelf) and `<file>.opf` beside ebooks (Calibre) — title, author, description, ISBN/language, calibre:series. Rename/organize now covers every media type: series templates for manga/comics/magazines, multi-file audiobooks moving as whole folders with their sidecars, and per-type templates all editable in Settings → Media Management
- [x] "Missing" view per author *(pulled forward from post-1.0)*: the author page ends with a Missing section listing bibliography gaps from the metadata provider — books neither monitored nor owned in that format library — grouped by series (ordered by position) then standalones by year; rows expand to a compact thumbnail + blurb, and a one-click **+ Monitor** adds the book to the library and starts searching (`GET /author/{id}/missing?library=`). Adding an author pulls their bibliography as metadata only — no book is auto-monitored or auto-enrolled, so a freshly added author's whole bibliography starts here in Missing, and an author with zero visible books still shows (an empty Books grid pointing at Missing) rather than disappearing
- [x] Author page actions are author-scoped: **Search wanted**, **Organize…**, and **Scan files** in the author header only touch that author's books (`POST /author/{id}/search?library=`, `GET/POST /library/rename?authorId=`/`{"authorId":N}`); **Remove from Ebooks/Audiobooks** takes the author out of one format library only (the other is untouched) with an opt-in delete-files checkbox, auto-deleting the author outright once they're in no library left
- [x] Manga colorized/monochrome variants *(pulled forward from post-1.0)*: manga stays **one** library (unlike the ebook/audiobook split) with a variant as a sub-dimension of its files. Each manga root folder is tagged colorized or monochrome (a variant selector appears when adding a manga root; monochrome is the default and existing roots backfill there), and a volume tracks each variant's ownership independently while sharing one series/volume metadata row. The volume list stays compact (title + owned/wanted badge); an owned volume expands to show which variants it owns (colorized/monochrome) and each file's on-disk location. Imports stay variant-agnostic (a release doesn't reveal its variant); the scanner records per-variant ownership as files land under their variant root
- [x] Manga series get the full author/book treatment: the series page has series-scoped **Search wanted** (`POST /series/{id}/search`), **Organize…** (`?seriesId=` on rename), **Scan files**, and **Refresh**; each volume expands to the same controls an individual book has (monitor toggle, **Auto grab**, **Search releases**, **Remove from library** with opt-in delete-files); and a per-series **Missing** section lists volumes neither monitored nor owned, each with a one-click **Monitor** to add it back. Removing a volume forgets its file records so it's no longer owned and drops into Missing (files stay on disk unless delete-files is checked; the next scan re-finds them). **Comics later got the same treatment** — expandable issue rows with per-item controls, the library/Missing split, and `library:"comic"` on `PUT /book/{id}/library` — everything except the manga-only colorized/monochrome variants

### Phase 5.5 — Pre-1.0 hardening
Everything is built; this phase proves it. Nothing here adds features — it
turns "works on the dev box" into "trustable release".

- [ ] **Live-verify the mock-tested integrations**: real Prowlarr sync, real torrents through qBittorrent, and real NZBs through SABnzbd are now verified end to end — search → grab → download → import → organized file — against a **TorBox/Real-Debrid bridge**, which shook out several download-client fixes (side-resolved NZB `addfile` and magnet/`.torrent` grabs so a NAT'd/cloud client works, slow-bridge add confirmation, completed files reached over a mounted share, format-optional scoring for manga/comic/magazine/audiobook). **Remote Path Mapping** is now a first-class feature (Settings → Download Clients: remote prefix → local prefix, longest match wins, applied to every client-reported path — no more mounting at the exact same path). Still open: a real **ComicVine** key (comics run on Hardcover today) and broader burn-in across all five libraries
- [ ] **Real-world burn-in**: run LibriNode as the actual daily server for a few weeks — real library sizes, messy release names, provider rate limits, long-seeding torrents, the daily refresh loop. The recent refresh/edition/pack bugs were all burn-in-class finds; assume more are waiting
- [ ] **Upgrade-path testing**: take databases from older commits, run every migration chain against them, and confirm nothing breaks or loses data — post-1.0, migration bugs are data-loss bugs. *(First drill done: a data dir created by the Phase-4 build migrates cleanly under the current binary — migrations apply, API healthy.)* Still open: automate a migration test against a seeded old-schema fixture
- [ ] **Failure-mode polish**: Hardcover down, token expired, indexer 429ing, download client vanishing mid-grab — each should degrade into a readable health banner and recover on its own, never a silent no-op or log-only error
- [ ] **Backup/restore drill**: restore a real backup onto a clean machine and confirm the library comes back whole — database, config, covers cache, and the staged-restore flow itself *(staged-restore flow drilled on a copy of the real data dir: backup → stage → restart swap → data intact with `.pre-restore` copies kept; the clean-machine restore remains)*
- [ ] **Security once-over**: API-key and session handling, path traversal on delete/organize (nothing may escape a root folder), and the log viewer never leaking tokens *(first pass done: sessions bound to their account — user removal/password change/disable-login revoke them immediately; API-key check is constant-time; the image proxy can't reflect non-http(s) URLs; backup names, zip restore entries, and delete paths audited traversal-clean. Still open: a token-leak sweep of log output)*
- [ ] **Performance sanity check**: a few-thousand-book library — grid rendering, scan and refresh-all duration, SQLite behavior under the background loops
- [ ] **Docs stranger-test**: a fresh person (or fresh machine) follows the quickstart and Docker compose from scratch; fix every step that needed insider knowledge
- [ ] **Version/release hygiene**: ~~real version stamping in builds~~ *(done: release builds stamp the tag via ldflags, and unstamped dev builds now self-identify as `dev-<sha> (<date>)` from the embedded VCS info)*, ~~a CHANGELOG~~ *(added)*, and a v0.9 release-candidate tag to shake out the release CI before the real one *(still open)*
- [ ] **Distribution**: published Docker images (GHCR) and a code-signed Windows installer that passes Smart App Control — signing is the logistics-heavy one, so it goes last

### Post-1.0 ideas
- [ ] Fuzzy / ISBN- and embedded-metadata-based file matching (scanning is exact-normalized-match today)
- [ ] `ComicInfo.xml` for CBR archives (needs a RAR writer)
- [ ] Multi-user / permissions
- [ ] Additional metadata providers (Open Library, Google Books) as fallbacks
- [ ] **Native indexer framework — sources Prowlarr can't reach, working *beside* Prowlarr, not instead of it.** Prowlarr only speaks Newznab/Torznab, so sites with no such API (HTML-scrape trackers, direct-download shadow libraries) are structurally out of its reach. Add a `type: native` indexer kind alongside the synced `newznab`/`torznab` rows: each names a built-in Go implementation, and all three kinds feed the *same* `SearchAll` merge, release scoring, and grab pipeline — a native source is a peer indexer in the same list, not a bolt-on. Native rows are LibriNode-managed only and stay off the Readarr-facing surface, so Prowlarr never sees, syncs, or collides with them; they fill exactly the gap it leaves. Two first targets, each exercising a different seam:
  - **AudioBook Bay** (audiobooks). No API — scrape the public listings and **assemble the magnet ourselves** from the on-page info hash + tracker list (precisely the step that breaks Prowlarr→Jackett today). Yields a normal `torrent` release, so it rides the existing qBittorrent path untouched. Design constraints: Cloudflare/anti-bot means a real UA, gentle rate-limiting, and cached caps — the per-indexer exponential backoff already covers repeat failures; audiobook category only.
  - **Anna's Archive** (ebooks, some audio). Keyless search, but downloads need a paid **membership API key** (`fast_download.json?md5=&key=`, stored as the indexer's API key; keyless = search-only, grab returns a clear "needs membership" message). Downloads are direct HTTP files, not torrent/usenet — so add one new **general `direct` protocol** (a third protocol beside torrent/usenet): a LibriNode-side fetcher that pulls the URL straight into the download dir, then Completed Download Handling imports it like any other grab (mirrors the existing debrid-bridge self-resolve). It's built source-agnostic on purpose — **any direct-link source rides it, including Library Genesis / open-book mirrors** (Anna's own list resolves to several download hosts, and Libgen has many rotating mirrors, so the fetcher takes a **mirror list and fails over** between hosts, tolerating slow/waitlisted free-tier links the way the bridge tolerates slow adds). Anna's is just the first client that speaks it.

  New code is small and composes existing parts: a `Searcher` interface (Torznab client + native impls behind it), the site clients, and one reusable `direct` download client with mirror failover — scoring, CDH import, backoff, and the bridge pattern are all reused. These are shadow libraries: **off by default, user-configured, user-responsible**, rate-limited, and documented as such (the same dual-use posture as Prowlarr's own AudioBook Bay definition).
- [ ] Magazine metadata, two tiers (magazines are fully provider-less today): **Wikidata** as the default series enricher — description, publisher, ISSN, and publication frequency (no key; structured properties, not Wikipedia infobox scraping), with frequency feeding expected-issue prediction for the Calendar and wanted logic — plus **Internet Archive** as an optional per-issue provider for archived/vintage titles (its collections carry real per-issue records with dates and covers, so it can populate an issue list the way Hardcover does for comics). Neither source has per-issue data for *current* magazines — those stay grab/scan-materialized — and commercial newsstand catalogs (Zinio/PressReader) have the right data but no public APIs
- [ ] Localization

### Maybe ideas
- [ ] Direct integrations: Calibre, Kavita, Komga, Audiobookshelf notify-on-import
- [ ] Import lists (Hardcover want-to-read shelf → auto-monitor)
- [ ] External notifications (Discord, webhook, email) on grab/import/upgrade/failure

---

## Status

🚧 **Pre-1.0 — Phases 0–5 built, hardening remains.** All five media types work end-to-end: ebooks and audiobooks flow author-first from Hardcover into separate per-format libraries with explicit membership; manga flows series-first from AniList or Hardcover and comics from Hardcover or ComicVine (both selectable) — series adds pull metadata only, with volumes/issues monitored selectively from a per-series Missing section or in bulk via the series toggle ("monitor future volumes" included), and manga ownable in colorized and monochrome variants at once; magazines are provider-less periodicals added by name, with issues recognized by date (organize-only for now — magazine searching/downloading is disabled). One acquisition pipeline serves the four acquiring libraries: per-type indexer categories with failure backoff, release parsing that understands formats, narrators, volume numbers, and issue dates, quality profiles with upgrade handling, a failed-release blocklist, qBittorrent/SABnzbd grabbing with seed-goal cleanup, and imports that land in reader-friendly layouts (Audiobookshelf folders with `metadata.opf` for audio, Kavita/Komga layouts with `ComicInfo.xml` for comics, OPF sidecars for Calibre) — with rename/organize covering every type. The UI is Plex-style (sidebar libraries, filterable poster grids, detail pages, grouped settings) with per-library Wanted pages, per-author Missing sections with author-scoped actions, a release calendar, health-check banners, multi-user login accounts with a first-run setup wizard, delete-from-disk options, backups with staged restore, and a log viewer; packaging (Docker, systemd, Windows scripts, release CI) and a docs site are in the repo. Hardcover, AniList, **Prowlarr** (application sync, both usenet and torrent indexers), and the **download clients** (qBittorrent and SABnzbd, both usenet and torrents, verified through a TorBox/Real-Debrid bridge from search to organized file) are verified against live services; ComicVine is mock-tested and awaits live confirmation. **1.0 waits on [Phase 5.5 — Pre-1.0 hardening](#phase-55--pre-10-hardening)**: live-verifying the mock-tested integrations, real-world burn-in, upgrade-path and restore drills, failure-mode/security/performance passes, and signed installers/published images — external notifications and the rest of the [Post-1.0 ideas](#post-10-ideas) stay parked for now.

## License

[GPL-3.0](LICENSE)
