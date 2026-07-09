# Libraries

A library only appears in the sidebar once it's set up — a root folder
added, or content of that type already owned. Library pages are poster
grids (authors for prose, series for the rest) with owned/total counts.
Grids over 10 cards get a filter box and render incrementally.

## Ebooks & Audiobooks (author-first, three levels deep)

Prose books flow from Hardcover. **Ebooks and Audiobooks are separate
libraries with explicit membership** — for both authors and books:

- An **author** appears in Audiobooks only if added there or you own an
  audiobook of theirs (and vice versa for ebooks); adding/removing an
  author in one format never touches the other.
- A **book** appears in a format library only if you own that format or
  deliberately added it there — never inferred. Each book membership has
  its own monitored flag; a library's Books grid lists only books that are
  monitored or owned in that format.

Browsing: library grid (authors) → **author page** → **book page**.

The author page has a portrait, bio, and author-scoped actions (**Search
wanted**, **Organize…**, **Scan files**, **Refresh metadata**, **Remove
from Ebooks/Audiobooks** — all touch only this author's books), a poster
grid of their monitored-or-owned books, and a **Missing** section below it:
the rest of the bibliography, grouped by series (then standalones by year),
each row expandable to a thumbnail + blurb with a one-click **+ Monitor**
that enrolls the book and starts searching. Adding an author pulls their
bibliography as metadata only — nothing is auto-monitored, so a freshly
added author's whole bibliography starts in Missing; an author with zero
visible books still shows, with an empty grid pointing at Missing, instead
of disappearing.

The book page has cover art, description, the monitor toggle, **Auto
grab**/**Search releases**, remove-from-library (with an opt-in
delete-files checkbox), and cross-add to the other format — once a book is
in both, this switches to a status badge instead of a button.

- Add from the Ebooks page → the author/book joins the Ebooks library.
- Cross-add from the book page (**+ Add to Audiobooks/Ebooks**, with a
  monitor prompt).
- Scanning/importing a format's file auto-enrolls the book (and its
  author) there.
- Refreshing metadata never enrolls, un-enrolls, or re-monitors anything —
  only descriptions/covers/new-book metadata update.

Audiobook scanning understands `Author/Title.m4b` and multi-file
`Author/Title/*.mp3` layouts; imports land as `Author/Book Title/` folders
with a `metadata.opf` sidecar — Audiobookshelf-ready. Ebooks get a
`<file>.opf` sidecar for Calibre.

## Manga & Comics (series-first)

Search the provider, add the series, and every volume/issue appears on its
page with owned/wanted badges. Manga metadata comes from **AniList** (no
key) or **Hardcover** (reuses your Hardcover token) — choose the manga
provider under **Settings → Metadata**; comics use **ComicVine**. Hardcover's
manga series sometimes lack clean volume numbers, so volumes are numbered
sequentially by the provider's order. The series
monitor toggle doubles as "monitor future volumes": refreshes (manual or
daily) discover new volumes and monitor them automatically. Imports write
`ComicInfo.xml` into CBZ archives and use Kavita/Komga-friendly
`Series/Series Vol. N.cbz` layouts.

Ongoing manga often have no official volume count on AniList yet — they add
with zero volumes and fill in as AniList publishes totals.

Manga series get the full author/book treatment. The series page carries
series-scoped **Search wanted**, **Organize…**, **Scan files**, and
**Refresh** (each touches only this series). The volume list stays compact
— title + owned/wanted badge — and every volume expands to a cover, blurb,
and the same controls an individual book has: a monitor toggle, **Auto
grab**, **Search releases**, and **Remove from library** (opt-in
delete-files). By default an owned volume's cover is extracted from the
archive's first page (CBZ or CBR, the latter read via pure-Go rardecode),
falling back to the provider's art; **Settings → Metadata** has a toggle to
always use the provider's cover instead. A per-series **Missing** section
lists volumes you're not
tracking — neither monitored nor owned — each with a one-click **Monitor**;
removing a volume forgets its file records so it drops into Missing, and the
next scan re-finds any files left on disk.

### Colorized & monochrome variants

Manga can be owned in both a colorized and a monochrome edition without
splitting the library. Add a **separate root folder per variant** — a
monochrome/colorized selector appears when the media type is manga
(monochrome is the default, and pre-existing manga roots are treated as
monochrome). Files scanned or imported under a root inherit its variant.

A volume is one metadata row that tracks each variant independently. The
volume list stays compact for long series — each row is the title and a
single owned/wanted badge — and an owned volume expands to show which
variants it owns (`🎨 colorized` / `◻️ monochrome`) and where each file
lives on disk. Grabbing is variant-agnostic (a release doesn't reveal
whether it's color or mono); per-variant ownership is recorded by the
scanner as files land under their variant root.

## Magazines (provider-less)

Add a magazine **by name**; LibriNode recognizes issues by date or number in
release and file names (`The Economist - 2026-07-04.pdf`, `Issue 452`).
Scanning materializes owned issues; automatic search grabs newly published
issues (capped per pass). Imports land as `Magazine/Magazine - date.pdf`.

## Organizing files

**Organize…** previews, then applies, moves that bring files in line with
the naming templates (**Settings → Media Management**) — all five media
types, multi-file audiobooks moving as whole folders with their sidecars.
Emptied folders are swept up to (never including) the root. Available on
every library page (everything) and on the author page (that author's
files only).

## Wanted, Home, and Calendar

Every library page has a **Wanted** card (monitored but missing that
format's file, with per-item search). **Home** shows per-library
Recently-added and Wanted rows — types never mix. **Calendar** lists dated
releases across all libraries, upcoming and recent.
