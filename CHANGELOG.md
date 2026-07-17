# Changelog

Notable changes to LibriNode. Format loosely follows
[Keep a Changelog](https://keepachangelog.com/); versions follow semver once
tagged releases begin (v0.9.0-rc is planned as the first).

## [Unreleased]

Everything to date — Phases 0–5 (feature-complete) plus the pre-1.0 hardening
in progress. Highlights from the hardening period, newest first:

### Added
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
