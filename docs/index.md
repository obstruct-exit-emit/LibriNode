# LibriNode

A self-hosted media automation server for **written media** — the
Readarr/LazyLibrarian successor that treats ebooks, audiobooks, manga,
comics, and magazines as first-class citizens.

LibriNode monitors your wanted list, searches your indexers, sends releases
to your download client, then imports, renames, and organizes files into
per-type libraries — automatically.

> 🚧 LibriNode is **pre-1.0**. The whole loop works end-to-end, but expect
> rough edges and breaking changes until 1.0.

## The five libraries

| Type | Metadata | Formats |
|---|---|---|
| Ebooks | Hardcover | epub, mobi, azw3, pdf |
| Audiobooks | Hardcover | m4b, m4a, mp3, flac, opus |
| Manga | AniList (no key) | cbz, cbr, epub |
| Comics | ComicVine (free key) | cbz, cbr, pdf |
| Magazines | none — added by name | pdf, epub, cbz |

Each library is independent: its own root folders, naming templates, and
quality profiles. Plex-style, a library only appears in the UI once you set
it up. For prose books, Ebooks and Audiobooks are **separate libraries** —
a book belongs only to the format libraries you added it to (or own).

## Highlights

- One acquisition pipeline for everything: Newznab/Torznab indexers (or
  Prowlarr sync), release parsing and scoring, quality profiles with
  upgrades, qBittorrent and SABnzbd, Completed Download Handling.
- Reader-friendly layouts: Audiobookshelf folders for audio, Kavita/Komga
  layouts with `ComicInfo.xml` for comics, OPF sidecars for Calibre.
- Poster-grid browsing with detail pages, per-library Wanted pages, a
  release calendar, health checks, optional login, backups, and a log
  viewer.
- Manga extras: colorized/monochrome variants in one library, per-series
  Missing view, and volume covers extracted from the owned CBZ/CBR.
- Local image cache: provider art is downloaded on add/refresh and served
  from LibriNode, surviving provider link rot.

Start with [Installation](installation.md), then the
[Quickstart](quickstart.md).
