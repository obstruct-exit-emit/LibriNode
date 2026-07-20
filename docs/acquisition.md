# Acquisition

Acquisition serves **ebooks, audiobooks, manga, and comics**. Magazines are
organize-only for now — the wanted sweep skips them and the release/grab
endpoints reject them; see the Libraries page. (The magazine categories
setting below is kept for when acquisition returns.)

## Indexers

Two ways in:

- **Manually** under **Settings → Indexers**: any Newznab (usenet) or
  Torznab (torrent) endpoint, including per-indexer feed URLs from Prowlarr
  or Jackett. Test buttons on the form and on every saved indexer.
- **Prowlarr sync**: in Prowlarr, add an application of type **Readarr**
  with LibriNode's URL and API key. Prowlarr pushes its indexers into
  LibriNode and keeps them in sync (LibriNode emulates the Readarr v1 API).

Each indexer carries per-type category lists: books `7000,7020`, audio
`3030`, comics/manga `7030`, magazines `7010` — adjust per indexer if yours
differ.

An indexer that keeps failing **rests with exponential backoff** (5 minutes
doubling up to 6 hours) instead of being retried every sweep; one success
clears it.

### Native indexers

Some sites speak no Newznab/Torznab API, so Prowlarr structurally can't reach
them. A **native** indexer is a built-in source, selected as the indexer's
*type* under **Settings → Indexers** (no URL to paste) — it feeds the same
search, scoring, and grab pipeline as everything else. Native indexers are
LibriNode-managed only and are hidden from Prowlarr, so it never treats them as
indexers it owns.

> ⚠️ **The native sources are a work in progress.** Scraping these sites
> reliably still needs work — expect failed searches and grabs. The Settings
> form flags each as WIP; enable them to experiment, not to depend on.

The built-in sources today:

- **AudioBook Bay** (audiobooks) scrapes the public listings and assembles a
  magnet from a release page's info hash and tracker list, producing an ordinary
  torrent that goes through qBittorrent like any other. AudioBook Bay temp-bans
  IPs that crawl, so a **search makes a single listing request** and the
  per-release detail fetch is deferred — the magnet is assembled only when you
  grab a result. Requests use a warmed-up, browser-like session, and a search
  bounced to the homepage is reported as rate-limited rather than retried.
- **Library Genesis** (ebooks) is the built-in ebook source: it searches
  libgen.li and downloads through its `ads.php` → `get.php` mirror by each
  file's MD5, using the `direct` protocol (below). No account needed. Its
  domain rotates, so set the **Site URL** to a live mirror if the default
  stops answering. Each result carries its **author, year, language, and file
  format** from the results table, so an interactive or automatic search keeps
  only the book you asked for in the language and format your quality profile
  wants — a Spanish edition, a wrong-author book, or an `fb2` when your profile
  lists only epub/mobi/azw3/pdf are all filtered out. (Libgen has many `fb2`
  scans; add `fb2` to your ebook quality profile if you want them.)

Each rotating-domain source takes an optional **Site URL** override plus an
optional **fallback site URL** (comma-separated); searches try them in order.

### The `direct` protocol

Ebook shadow-library sources hand out plain HTTP links, not torrents or NZBs,
so LibriNode has its own **direct** download client — add it under **Settings →
Download Clients** with a local download folder as its "host". It streams the
file itself, **failing over across a `|`-separated mirror list**, following a
membership-API JSON answer or an open-mirror landing page one hop to the real
file, and Completed Download Handling imports the result like any other grab.
It's source-agnostic: any direct-link source can ride it.

**Library Genesis** (ebooks) rides a third release protocol: **direct** —
plain HTTP file downloads, handled by a built-in **Direct fetcher** download
client (Settings → Download Clients: pick *Direct fetcher*, point it at a local
download folder). LibriNode streams the file itself — no external program —
follows the mirror's landing page (`ads.php` → `get.php`) to the real file,
shows progress in the Activity queue, and Completed Download Handling imports
the result like any other grab. A release can carry a `|`-separated **mirror
list**: hosts are tried in order and a dead one fails over to the next. Files
are keyed by **MD5**, the key the open mirror network serves by — so downloads
work **without any account**.

These are dual-use shadow-library sources: **nothing is bundled or enabled by
default** — you add one deliberately, and its use is your responsibility. Being
scraped, a native source is inherently more fragile than an API indexer (a site
redesign can break it) and is rate-limited to stay polite.

## Release scoring & quality profiles

Search results are parsed (author/title/year, formats, retail, language,
narrator/bitrate/abridged for audio, volume numbers, issue dates) and scored
against the **default quality profile** for the media type (**Settings →
Quality Profiles**): ordered format preferences, language, size bounds, retail
bonus. Candidates that can't be the book you asked for are rejected outright.

Manga, comic, magazine, and audiobook release names often omit the file
format — a scan is just `Vol. 01 (Digital)`, an audiobook carries the bitrate
or narrator instead of `m4b`/`mp3`. Those are accepted (a named format still
ranks higher) and the real format is read from the downloaded files at import;
ebooks still require a recognized format in the name.

With **upgrades allowed**, owning a lesser format keeps the book wanted
until the profile's cutoff; upgrade grabs must be strictly better, and the
import replaces the old file.

## Download clients

qBittorrent (torrents) and SABnzbd (usenet), under **Settings → Download
Clients** — category-scoped so LibriNode only ever touches its own
downloads.

LibriNode resolves each release on **its own side** before handing it off:
it fetches the NZB and uploads the file (SABnzbd `addfile`), and follows a
torrent to its magnet or downloads the `.torrent` and uploads the bytes. So a
download client behind NAT — or a SABnzbd/qBittorrent-compatible **debrid
bridge** (Real-Debrid, TorBox) whose cloud side can't reach your LAN
indexers — still works. Adds to a slow debrid bridge are confirmed against the
client's list so a slow response never loses the grab.

- **Automatic search** sweeps all wanted items every 6 hours; **Search
  wanted** and per-item **Auto grab** run on demand; **Search releases**
  lists scored candidates for hand-picking.
- **Completed Download Handling** (every minute): finished downloads are
  imported into the naming-template layout and grab history updated. Three
  **Import handling** options (Settings → Download Clients), **all on by
  default**, govern the rest: *import whole packs*, *remove the completed
  download from the client*, and *delete the downloaded files*. Turn the last
  two off to leave torrents seeding and the originals in place — usenet
  history is cleared either way, since LibriNode only ever copies from it.
- **Multi-book packs**: when a grabbed release turns out to be a bundle
  ("complete series"), the grabbed book's file is identified by volume
  number (manga/comics) or title (ebooks) — never by size, so a v01–v12
  pack can't file volume 12 as the one you grabbed. With **Import whole
  packs** on (the default), the pack's other files fill every book they
  match — imported ebooks join their format library, though nothing is
  monitored automatically. Turn it off to fill only the grabbed book plus
  other **monitored** books. Either way, a book that already owns the format
  is only replaced when the pack's copy is a genuine quality upgrade.
- **Seed goals**: with *remove completed downloads* off, a torrent keeps
  seeding to the ratio/time limit set in qBittorrent; when it finishes and
  pauses (goal reached), LibriNode removes the torrent *and its data* — but
  only for downloads it grabbed and imported.
- **Failed and junk downloads** are blocklisted (never grabbed again; a
  replacement search starts immediately) and deleted — out of the client and
  off disk. This covers client-side failures, spam whose content isn't the
  book (an `.exe` instead of a media file), and completed downloads whose
  files never become readable. The blocklist is managed from the Activity
  page.
