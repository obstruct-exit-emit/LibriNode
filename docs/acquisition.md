# Acquisition

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

## Release scoring & quality profiles

Search results are parsed (author/title/year, formats, retail, language,
narrator/bitrate/abridged for audio, volume numbers, issue dates) and scored
against the **default quality profile** for the media type (**Settings →
Libraries**): ordered format preferences, language, size bounds, retail
bonus. Candidates that can't be the book you asked for are rejected outright.

With **upgrades allowed**, owning a lesser format keeps the book wanted
until the profile's cutoff; upgrade grabs must be strictly better, and the
import replaces the old file.

## Download clients

qBittorrent (torrents) and SABnzbd (usenet), under **Settings → Download
Clients** — category-scoped so LibriNode only ever touches its own
downloads.

- **Automatic search** sweeps all wanted items every 6 hours; **Search
  wanted** and per-item **Auto grab** run on demand; **Search releases**
  lists scored candidates for hand-picking.
- **Completed Download Handling** (every minute): finished downloads are
  imported into the naming-template layout, grab history updated. Files are
  *copied*, so torrents keep seeding; usenet history is cleaned up.
- **Multi-book packs**: when a grabbed release turns out to be a bundle
  ("complete series"), the grabbed book's file is identified by volume
  number (manga/comics) or title (ebooks) — never by size — and the pack's
  other files fill your **monitored** books only. Unmonitored books are
  never auto-imported by default; an opt-in **Import whole packs** toggle
  (Settings → Download Clients → Import options) imports every book the
  pack matches instead — files land and ebooks join their library, but
  nothing gets monitored automatically. Either way, a book that already
  owns the format is only replaced when the pack's copy is a genuine
  quality upgrade.
- **Seed goals**: configure ratio/time limits in qBittorrent. When it
  finishes and pauses a torrent (goal reached), LibriNode removes the
  torrent *and its data* — but only for downloads it grabbed and imported.
- **Failed downloads** are blocklisted (never grabbed again; search falls to
  the next candidate) and cleaned out of the client. The blocklist is
  managed from the Activity page.
