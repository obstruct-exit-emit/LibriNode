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
- **Failed downloads** are blocklisted (never grabbed again; search falls to
  the next candidate) and cleaned out of the client. The blocklist is
  managed from the Activity page.
