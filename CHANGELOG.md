# Changelog

Notable changes to LibriNode. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions follow semver.
Tagging began with `v0.9.0-rc.1` and `v0.9.0-rc.2` (release candidates that
shook out the release CI); `v0.9.0` will be the first stable tag.

## [Unreleased]

Everything to date — Phases 0–5 (feature-complete) plus the pre-1.0 hardening
in progress. Highlights from the hardening period, newest first:

### Added
- **A "Refresh all metadata" button** under Settings → Metadata → Cache
  maintenance re-fetches every author and series from your provider in one
  action — descriptions and covers come back, and (with the reconcile) entries
  the provider no longer lists are removed. Previously a full re-sync could only
  be triggered per-library from each library page or waited for the daily sweep;
  the metadata section, where you go to clear/rebuild metadata, now offers it too
  (`POST /library/refresh` accepts `{"mediaType":"all"}`).
- **Box sets & collections are hidden from metadata search by default**, with an
  opt-in under Settings → Metadata ("Show box sets & collections"). Hardcover
  lists omnibus/box-set editions alongside the individual books — six of them for
  a "dune" search — so results stay to single books unless you want the sets.
- Organize now **scans first** (scoped to its level — a library page scans
  only that library's roots, an author/series page its own format) so the
  move plan always reflects what's actually on disk, and library-level
  organize gained a **cleanup**: files that don't belong in the library
  (download junk like `.nfo`/`.torrent`, or another type's media dumped in
  the root) are previewed with a delete checkbox, and applying prunes every
  empty folder. Matched files, unmatched media, `.opf` sidecars, artwork,
  and `ComicInfo.xml` are always kept; deletions are re-validated
  server-side against the library's own roots.
- **Library Genesis** ebook source (native indexer, `direct` protocol).
  Searches libgen.li and downloads by MD5 through its `ads.php` → `get.php`
  mirror — the direct fetcher follows the landing page to the real file — so
  downloads work **without any account**. Each result carries its author, year,
  language, and file format from the results table, so an interactive or
  automatic search keeps only the book you asked for in the language and format
  your quality profile wants (a wrong-language edition, wrong-author book, or an
  unwanted format is filtered out; the release scorer's author check is now
  order-independent so "Last, First" listings match). Off by default,
  user-added, user-responsible. (Anna's Archive was evaluated and dropped — it
  renders search behind a JS/anti-bot wall needing a browser/bypasser LibriNode
  doesn't ship, and Libgen is the catalog it aggregates.)
- **Light theme** with a sidebar theme control: Auto (follows your OS, live),
  Light, or Dark — a per-browser preference applied before first paint, so
  there's no flash. The whole UI runs on one CSS-variable contract; the light
  palette is paper-toned.
- **Per-file actions** on book and volume detail pages: every file row now has
  **organize** (preview → confirm the naming-template moves for that book) and
  **delete** (removes the file from disk after a confirmation) — no more
  round-trip through library-wide organize or the API.
- Mobile: sidebar group labels now render as row headers at the narrow
  breakpoint instead of disappearing.
- A general **`direct` download protocol** — a third release protocol beside
  torrent and usenet: a built-in download client (type `direct`, its "host" a
  local download folder) where **LibriNode streams the file itself** —
  mirror-list failover, following an open-mirror landing page (or a membership
  API's JSON answer) one hop to the real file, live progress in the queue, and
  Completed Download Handling importing the result like any other grab. Library
  Genesis rides it; any direct-link source can.
- Native indexer framework + **AudioBook Bay**. A new `type: native` indexer
  kind sits beside Newznab/Torznab: a built-in Go source, selected as the
  indexer's type (no URL), feeding the same search/scoring/grab pipeline as the
  API clients. Native sources are LibriNode-managed only and hidden from
  Prowlarr, so it never treats them as indexers it owns. The first source is
  AudioBook Bay: it has no API, so it scrapes the listings and **assembles the
  magnet from the on-page info hash + tracker list**, yielding an ordinary
  torrent that rides the existing qBittorrent path. These are dual-use
  shadow-library sources — **nothing is bundled or enabled by default**; a user
  adds one deliberately and is responsible for its use. (A general `direct`
  HTTP-download protocol and Library Genesis followed.)
- ISBN / embedded-metadata / fuzzy file matching for library scans. On top of
  the existing exact author/title matching, a file now also matches by **ISBN or
  ASIN** — parsed from the filename or read from an epub's embedded OPF metadata
  — against a known edition, so a correctly-identified but oddly-named file is
  placed outright (ISBN-10 and ISBN-13 fold to one checksum-validated form). And
  when nothing matches by title, a **fuzzy** pass (character-bigram similarity)
  pre-fills the Unmatched card's import picker with the closest book — offered
  for one-click confirmation, never auto-imported. A file with neither a usable
  identifier nor a fuzzy hit behaves exactly as before.
- Automated clean-machine backup/restore drill: a backup taken on a populated
  data dir is staged into a brand-new empty one and swapped in through the real
  startup path, asserting the library comes back whole (author/book rows intact,
  config verbatim) and that a fresh machine — with no live files to protect —
  is left with no `.restore`/`.pre-restore` leftovers. Closes the clean-machine
  half of the restore drill that the staged-restore test didn't cover.
- Automated upgrade-path (migration) testing: a database seeded at an older
  pre-rebuild schema is driven through the full remaining migration chain,
  asserting that the table rebuilds keep every row and the membership/variant
  backfills compute the values an upgrading user expects — migration bugs are
  data-loss bugs, so they now fail a test instead of a real library. A
  companion test confirms a fresh database applies every migration and that
  re-opening it is a clean no-op.
- Metadata fallback providers: **Open Library** and **Google Books** ship as
  keyless book providers (a Google Books API key is optional, only to lift
  rate limits). Configure them, in order, under Settings → Metadata →
  Fallbacks: the active provider answers everything it can, and a fallback is
  consulted only when the active one finds nothing for a search or an id
  lookup — the "as fallbacks" contract, not a merge. A record found through a
  fallback is stored under that fallback's name, so its later metadata refresh
  routes back to the same source rather than the primary that never had it
  (a new `metadataSource` field carries the origin through the add). Either
  new provider can also be selected as the primary book provider. Implemented
  as a `metadata.FallbackProvider` chain wrapping the registered providers, so
  further sources are still one registration away.
- Roles / permissions: every login account is now an **admin** or a
  **member**. Members get everyday use — browsing, monitoring, search, grab,
  scan, organize, and their own password — but not the server's own
  configuration (Settings, Indexers, Download Clients, Quality Profiles,
  backups, logs, root folders) or other accounts. Every configuration and
  account-management route is gated behind an admin check on the backend (the
  UI only hides what the API already refuses); the API key stays
  admin-equivalent for automation. Set the role when adding a user or
  promote/demote later. The default user is always an admin, so an instance
  can never be left with no one who can administer it, and changing a role
  revokes that account's other sessions immediately. Accounts created before
  roles existed migrate to admin, so nothing changes until you choose to
  restrict someone.
- Library-wide "Refresh metadata" on every library page (except organize-only
  magazines) — the bulk twin of the per-author/per-series Refresh buttons,
  honoring per-record provider overrides; runs in the background, one at a
  time, and reports how many records it covers.
- Remote path mappings (Settings → Download Clients): map a download
  client's reported path prefix to the local path where this server sees
  the same files — longest prefix wins, boundary-aware, case-insensitive,
  separator-converting. Ends the "mount the share at the exact same path"
  requirement for remote/containerized clients.
- Security hardening: login sessions are bound to their account (user
  removal and password changes revoke them immediately; disabling login
  revokes all), the API-key check is constant-time, and the image proxy
  refuses to reflect non-http(s) URLs.
- Series pack grab: manga/comic series pages get "🎁 Search packs" — release
  parsing understands volume ranges ("v01-v41", "#1-60") and completeness
  words, pack candidates are ranked (full range > partial > bare series
  title; single volumes rejected back to the per-volume flow), and a grabbed
  pack is imported by the existing pack importer, filing every matching
  volume. Closes the "series torrents are whole-series packs" content gap.
- Configurable background timings (Settings → General → Advanced): wanted
  search, metadata refresh, health checks, and import polling — blank means
  default, values are clamped to sane ranges, applied at startup.
- Bulk monitor from Missing on author and series pages: per-row checkboxes
  with "+ Monitor selected", plus "+ Monitor all" per series group or whole
  section.
- Activity History paging: server-side total with progressive "Show more"
  (the 200-row cap is gone) and a debounced title filter.
- Friendly first-use states: an empty library now shows per-type guidance
  with direct + Add / Scan actions instead of one line of text.
- Responsive pass (≤700px): wrapping card heads and action rows, full-width
  settings fields, adaptive poster grids, bottom-sheet toasts; plus an
  accessibility edge pass (aria-current nav, labeled icon-only controls,
  meaningful alt text on detail art).
- White-glove P3 surface passes: quality profile formats are ordered chips
  (reorder/remove/add with per-type suggestions); indexer and download
  client rows show protocol/priority/disabled pills with the URL beneath;
  calendar items are clickable (through new authorId/seriesId on the
  calendar API) with relative when-badges on day headers; the System page
  leads with a status tile grid and colors ERROR/WARN log lines; sign-in
  and API-key screens are centered branded cards; author and series pages
  show an owned/total progress meter.
- White-glove UX wave: an app-wide toast layer (stacking, dismissible)
  replaces the top-of-page error card for in-app errors; every native
  browser confirm() popup (13) is now a styled confirm dialog; search-and-add
  renders provider results as poster cards with cover art and per-card add
  state; URL routing (hash-based) makes every page bookmarkable with working
  back/forward and refresh; indexers, download clients, and quality profiles
  are editable in place (including the 1–50 priority, finally exposed);
  global search in the sidebar spans every library; skeleton loading states
  replace "Loading…" text; Activity history, the blocklist, and backups show
  relative ages via shared date/size formatting utils.
- Existing-file import across all five libraries: unmatched files get a
  best-guess suggestion with a 0–100% confidence rating, one-click Import and
  bulk "Import all matched", duplicate resolution (both files shown, Replace
  or Delete — variant-aware for manga), and one-click adding of a missing
  author (provider search), manga/comic series (provider search), or magazine
  (by name). Magazine imports materialize the issue on the spot; adopted
  prose books are enrolled and monitored.
- First-run setup wizard: a fresh instance is claimed by its first visitor —
  create the account, then guided steps for library folders (with a visual
  folder browser), the Hardcover token, an indexer, and a download client.
  No API-key paste required.
- Multi-user accounts: the Security card lists users with add, change
  password, make-default, and remove (the default account is protected).
- Visual folder picker for root folders (Settings and the wizard): browse the
  server's filesystem instead of typing paths blind.
- Release browser: interactive search results in one organized component —
  approved/all and protocol filters, sorting (score/size/seeders/age),
  always-visible size/seeders/leechers/age per row with the active sort
  highlighted, rejection reasons inline with a "grab anyway" override, and
  per-row grab feedback.
- Live download progress: Activity queue rows carry progress bars and per-line
  remove; blocklist and history collapse into dropdowns; a book page's badge
  turns "downloading N%" while its grab is active and reverts on its own;
  series volume rows and Wanted cards show the same live state.
  Queue responses are served from a shared 15-second snapshot so open pages
  never stampede the download clients.
- Richer default file naming across all libraries: per-book ebook folders,
  author/title/year in file names, zero-padded volume/issue numbers.
- Home page tiles link to their book or series page; the book page's
  cross-format badge links to the same book in the other library; multi-file
  audiobooks list their tracks on the book page.
- Junk/spam defense: releases naming an executable are rejected before grab;
  completed downloads with wrong content (or an executable payload) are
  blocklisted, deleted from the client *and* disk, and immediately re-searched;
  completed-but-never-readable paths are abandoned after a grace period.
- Import handling options (Settings → Download Clients), all on by default:
  import whole packs, remove completed downloads from the client, delete the
  downloaded files after import.
- Debrid-bridge support (Real-Debrid/TorBox style): LibriNode resolves
  releases on its own side — NZB fetched and uploaded via SABnzbd `addfile`,
  torrents passed as magnets or uploaded `.torrent` bytes — and tolerates
  slow bridge adds by confirming against the client's list.
- Multi-disc audiobooks: disc subfolders (CD1/Disc 02/Part 3) survive import
  and scan as one book unit; other nesting is flattened collision-safely.

### Fixed
- **An author's "missing" list drops foreign editions, box sets, and
  anthologies.** Hardcover catalogs every translation as its own book under the
  author, lists multi-author anthologies and magazine issues the author has one
  story in, and files graphic-novel adaptations under them too — so a
  bibliography was mostly not the author's own books (Frank Herbert ran to ~100
  entries, Andy Weir ~50). A book is now kept only when it looks like the
  author's own work: it has an edition in your metadata language (or enough
  readers to be a real work Hardcover just hasn't language-tagged), it isn't a
  known foreign edition, it isn't a box set (unless "Show box sets & collections"
  is on), and it credits a normal number of authors rather than the dozens an
  anthology does. Frank Herbert now lists his actual novels and stories instead
  of a dozen translated Dunes, *Nebula Winners Fifteen*, and *The Wesleyan
  Anthology of Science Fiction*. A metadata refresh also **reconciles** now — a
  bibliography entry the provider stops returning is removed, so the cleanup
  reaches libraries already full of the old junk; only books you never added to a
  library and own no file for are ever removed.
- **Hardcover search and author pages carry far less junk.** Hardcover lists many
  near-duplicate and ghost records for one work — a film study and two authorless
  records all titled "Dune" next to Frank Herbert's, plus reissues and
  translations promoted to their own book. Search now collapses true duplicates
  (same title + author) to the most-read record and drops same-title stragglers a
  dominant work dwarfs (under 1% of its readers) along with never-read ghost
  records; an author's bibliography drops repeated titles, keeping the canonical,
  most-read one. Genuinely distinct same-title works, each with real readers, both
  stay.
- **Open Library and Google Books work as selectable metadata sources** now, not
  just names in the list. Open Library's provider test searched for a stopword
  ("the"), which Open Library rejects with HTTP 422 — so testing or activating it
  failed with a Bad Gateway; it now queries a real word and validates cleanly
  (and identifies itself for Open Library's higher rate limit). Google Books'
  keyless access shares one global anonymous daily quota that is frequently spent
  already: a 429 with no key configured now says exactly that and points you to
  add a free API key, instead of a bare "HTTP 429". (Open Library needs no key.)
- **AudioBook Bay** stopped intermittently returning empty results or bouncing to
  the homepage on a long-running server: it opens a **fresh connection per
  request** instead of reusing the app-lifetime keep-alive pool, which the site
  throttles once one connection has served enough requests. A browser, curl, and
  a just-started process always worked against the same site and IP — only the
  server's reused pooled connection failed.
- **Library Genesis** searches keep the right book again: the author is read
  from the results table's own column (libgen.li stopped wrapping authors in
  links, so releases carried no author and the scorer rejected every one for
  "not mentioning the author"). And a downloaded ebook is saved by its **real
  type** — the direct client identifies the file from its content, so a book
  served from a `get.php` mirror URL is written as `.epub`, not the unusable
  `.php` the URL implied.
- The built-in **direct downloader** now validates what it receives: a mirror
  that answers with an error/landing page instead of the file (even one
  mislabeled as a download) is rejected instead of saved as a bogus "book", and
  a direct download is removed from the download folder once imported.
- A release **grabbed from the web UI now auto-imports**. The app ran two
  separate download services — one for the background importer, one for the API —
  and the built-in direct client's queue is in-memory, so a UI grab finished in a
  queue the importer never watched. The whole app now shares one download
  service, so a UI grab and the auto-importer act on the same queue. (Remote path
  mappings now apply to API-triggered imports too.)
- **AudioBook Bay** grabs resolve to a real magnet again: release URLs keep their
  trailing slash — the slash-less form returns an unfollowable 301, so no magnet
  was ever assembled. A search bounced to the homepage (ABB rate-limiting the IP,
  common on a shared/VPN exit IP) is now retried a few times with backoff,
  browser-style, and requests carry full browser navigation headers, before the
  rate-limit error is surfaced.
- Series links now reconcile on every metadata refresh: a book's series
  membership is set to exactly what the provider currently reports, so a stale
  or wrong link (e.g. a standalone the provider once mislabeled as part of a
  series) is dropped instead of sticking forever and corrupting the organized
  path via `{Series Title}`. Previously links were only ever added.
- **Scan** is now scoped like organize already was: scanning from a specific
  library (or author/series) page walks only that library's roots, not every
  root on the server.
- **AudioBook Bay** stopped hammering the site into IP bans: a search now makes
  a single listing request and defers each result's per-page magnet assembly to
  grab time (for the one release grabbed), riding a warmed-up browser-like
  session; a search bounced to the homepage surfaces as rate-limiting.
- An unmatched-file row whose path was long no longer collapses into a
  one-character-per-line vertical strip — the path takes its own line with the
  actions below.
- Performance: a scan used to write every file as its own autocommit SQLite
  transaction, so a library scan of a few thousand files took 30+ seconds
  (a no-op rescan barely faster). Ebook/audiobook/manga/comic scans now
  batch a whole root's writes into one transaction — 31.8s → 2.5s on a
  synthetic ~11,000-book/2,293-file library, 13.0s → 2.9s on a rescan.
  Magazines keep the old per-file behavior (materializing a new issue needs
  a second connection the batch would starve, given the database's
  single-connection cap) — the smallest-volume scan path in practice.
- `GET /book` unscoped was shipping every book of every media type to the
  browser just to populate the Ebooks/Audiobooks page's manual-match
  fallback list (5.7 MB of unused JSON at library scale). `GET
  /book?library=ebook|audiobook` filters server-side now.
- Log/token-leak sweep: Newznab/Torznab, SABnzbd, and ComicVine all carry
  their API key directly in the request URL's query string, so a connection
  failure (or an indexer error page echoing the query back) used to leak
  the raw key into the health banner, search-error notices, and the log
  file the in-app log viewer exposes. A new `internal/redact` package
  strips known credential-shaped query params — and scrubs their literal
  values out of response bodies too — from every error at the point it's
  raised, wired into the indexer client, both download clients, and
  ComicVine. Hardcover (header-based auth) and AniList (keyless) were never
  exposed to this. Regression tests assert the exact secret string never
  survives into the resulting error.
- Failure-mode polish across the four scenarios that used to degrade into a
  silent log line: a metadata provider that's down or unreachable now reads
  as a self-healing warning in the health banner, distinct from a genuinely
  rejected token/key (a new `ErrUnreachable` sentinel, wired through
  Hardcover/AniList/ComicVine); manga/comic series providers are health-
  checked too, scoped to libraries actually in use; a background refresh
  sweep aborts after three consecutive unreachable results instead of timing
  out on every remaining author/series one at a time; an indexer already
  resting in backoff is no longer re-probed by the health check (which would
  add load to something already known to be 429ing) and reports its
  resting-until time instead; and the importer's orphan sweep — which
  resolves a grab whose download vanished from its client — is now per-
  client, so one download client being briefly unreachable can't freeze
  orphan resolution for grabs sitting in a different, healthy client.
- The scan no longer silently attaches a file to a book that belongs only to
  the OTHER format library (the "added an ebook, it showed up in Audiobooks"
  linkage): the file lands in Unmatched with a confident suggestion, and the
  one-click import is the consent that enrolls the second format. Books in
  no format library yet still match freely on scan.
- A library page's Organize… now moves only that library's files (it used to
  organize every library at once); the rename API gained a mediaType scope.
- Manga/comic unmatched files without an auto-matched series can now be
  matched by hand: pick any series in the library, then one of its volumes.
- Verified with new regression tests that adding an author or book into one
  format library never enrolls the other (a title appearing in both means
  you own both formats — ownership enrolls by design).
- Scanner matches survive organizing: template-named files re-match their
  books on every scan (template-aware keys), manual matches stick, and a
  re-found file no longer duplicates its record.
- An import whose organized target already exists on disk but is unrecorded
  now adopts the file instead of skipping forever with the grab stuck.
- Format-less release names (manga/comic/magazine/audiobook convention) are
  scored instead of rejected; series/magazine title matching is whole-word so
  short titles no longer false-match longer ones.
- Failed and junk downloads have their data deleted directly on disk as well,
  covering clients that ignore the delete-files flag.

### Changed
- **Library visibility is membership, not monitoring.** A prose title shows up
  in its Ebooks/Audiobooks library exactly when it's a member of that library;
  unmonitoring a book stops it from being auto-grabbed but no longer hides it
  from the grid. The owned-vs-total progress meter that implied "monitored =
  should own" is gone.
- Native sources still under real-world burn-in (AudioBook Bay, Library Genesis)
  are now flagged **WIP** in the indexer UI so their off-by-default, use-at-your-
  own-risk status is visible where you enable them.
- The metadata refresh sweep defaults to every **30 days** (was 24 hours) —
  metadata rarely changes and a monthly re-sync is kinder to providers.
  Per-item and manual refreshes are unaffected; tune it under Settings →
  General → Background timings (6–2160 hours).
- Magazines are organize-only for now: searching and downloading are disabled
  everywhere (wanted sweep, series search, release search, grab), while
  add-by-name, scanning, issue materialization, import, and organizing all
  keep working. The magazine search engine stays in the tree for later.
- Unstamped (dev) builds now report `dev-<sha> (<date>)` from the embedded
  build info instead of a stale placeholder; releases keep stamping real
  versions via ldflags.

Earlier work (Phases 0–5) is chronicled in the README's roadmap section:
libraries for ebooks/audiobooks/manga/comics/magazines, Hardcover/AniList/
ComicVine metadata, Prowlarr sync, quality profiles and upgrades, Completed
Download Handling with multi-book pack imports, per-library UI, health checks,
authentication, backups, packaging, and the docs site.
