# Libraries

A library only appears in the sidebar once it's set up — a root folder
added, or content of that type already owned. Library pages are poster
grids (authors for prose, series for the rest) with owned/total counts;
clicking a card opens a detail page with artwork, description, actions, and
the books/volumes as rows. Grids over 10 cards get a filter box and render
incrementally.

## Ebooks & Audiobooks (author-first)

Prose books flow from Hardcover. **Ebooks and Audiobooks are separate
libraries with explicit membership**: a book appears in a format library
only if you own that format or deliberately added it there — never
inferred. Each membership has its own monitored flag; a library lists only
books that are monitored or owned in that format.

- Add from the Ebooks page → the book is in the Ebooks library.
- Cross-add from the book detail (**+ Add to Audiobooks/Ebooks**, with a
  monitor prompt); once a book is in both, the detail shows the other
  format's status badge instead.
- Scanning/importing a format's file auto-enrolls the book there.
- Removing from a library (optionally deleting that format's files — an
  opt-in checkbox) leaves the other library untouched.

Audiobook scanning understands `Author/Title.m4b` and multi-file
`Author/Title/*.mp3` layouts; imports land as `Author/Book Title/` folders
with a `metadata.opf` sidecar — Audiobookshelf-ready. Ebooks get a
`<file>.opf` sidecar for Calibre.

## Manga & Comics (series-first)

Search AniList (manga) or ComicVine (comics), add the series, and every
volume/issue appears on its detail page with owned/wanted badges and
per-volume Auto grab. The series monitor toggle doubles as "monitor future
volumes": refreshes (manual or daily) discover new volumes and monitor them
automatically. Imports write `ComicInfo.xml` into CBZ archives and use
Kavita/Komga-friendly `Series/Series Vol. N.cbz` layouts.

Ongoing manga often have no official volume count on AniList yet — they add
with zero volumes and fill in as AniList publishes totals.

## Magazines (provider-less)

Add a magazine **by name**; LibriNode recognizes issues by date or number in
release and file names (`The Economist - 2026-07-04.pdf`, `Issue 452`).
Scanning materializes owned issues; automatic search grabs newly published
issues (capped per pass). Imports land as `Magazine/Magazine - date.pdf`.

## Organizing files

**Organize…** on any library page previews, then applies, moves that bring
files in line with the naming templates (**Settings → Media Management**) —
all five media types, multi-file audiobooks moving as whole folders with
their sidecars. Emptied folders are swept up to (never including) the root.

## Wanted, Home, and Calendar

Every library page has a **Wanted** card (monitored but missing that
format's file, with per-item search). **Home** shows per-library
Recently-added and Wanted rows — types never mix. **Calendar** lists dated
releases across all libraries, upcoming and recent.
