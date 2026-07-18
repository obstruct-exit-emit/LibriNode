# Changelog

Notable changes to LibriNode. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions follow semver once
tagged releases begin (v0.9.0-rc is planned as the first).

## [Unreleased]

Everything to date — Phases 0–5 (feature-complete) plus the pre-1.0 hardening
in progress. Highlights from the hardening period, newest first:

### Added
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
