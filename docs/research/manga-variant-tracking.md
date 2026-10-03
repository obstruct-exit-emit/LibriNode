# Research: per-variant wanted tracking for manga

Status: **design research, not yet built.** This is the "Future 💡 → Per-variant
wanted tracking" item from [ROADMAP.md](../../ROADMAP.md), split out here because
it's a genuinely multi-part design (schema + search + scoring + import) rather
than a roadmap one-liner. Companion to the variant-*detection* work that's
already shipped (`internal/scanner/variant.go`, `internal/comicinfo`,
`organize.PlaceFile`'s `pickRoot`) — this is what to build on top of it.

## The gap, precisely

Confirmed in code, not guessed: `wantedWhere()` (`internal/library/home.go`)
for manga is *does any file exist for this book+mediaType* — fully
variant-blind. Same blindness in `SearchSeriesPacks`' missing-count
(`v.HasFile`) and `importPackExtras`' owned-check. There is **no stored
concept of "which variant(s) do I want"** anywhere in the schema —
`organize.TargetVariant()` isn't a preference, it's just "whichever root
folder happens to be configured first," used only as a placement guess.

Meanwhile `upgradeCheck`/`ownedFiles` (`internal/importer/importer.go`) **are
already correctly variant-scoped** — duplicate-prevention and upgrades
compare same-variant files only. So the missing piece is entirely at two
layers: *what counts as "still wanted"*, and *which variant should this
particular search go hunting for*.

## Design: a per-series target-variant setting

**Where it lives:** `library.Series`, not `release.Preferences`. Preferences
is global per-media-type scoring policy; this is a per-record fact — matching
the existing precedent of `Series.ProviderOverride`, a field that overrides a
default only where the admin has deliberately said so.

**Why per-series, not global or implicit-from-root-config:** considered "both
roots configured ⇒ want every volume in both variants" and rejected it. Most
manga has **no official colorized edition at all**; a blanket "want both"
policy would have LibriNode perpetually searching for color editions that
were never produced, for nearly every series, and never converging.
Colorized reprints (Death Note, Solo Leveling, One Piece Digital Colored) are
the **exception**, not the rule — this has to be explicit opt-in per series,
defaulting to `""` (today's behavior: any one variant satisfies "owned," zero
behavior change for the overwhelming majority of series).

```sql
-- migration
ALTER TABLE series ADD COLUMN target_variant TEXT NOT NULL DEFAULT '';
-- '' (default, today's behavior) | 'mono' | 'color' | 'both'
```

- `library.Series.TargetVariant string` + store/API plumbing, mirroring
  `ProviderOverride`'s existing shape exactly (`PUT /series/{id}/...`).
- UI: a control on the series page, next to Monitor/ProviderOverride — but
  **only shown when more than one variant root is configured**. Meaningless
  otherwise, and showing it for every manga series when the user has one
  root configured would be pure clutter for no payoff.

**Wanted-query rewrite** (`wantedWhere`, manga branch): join
`series_books`/`series` (reusing the same "primary series" resolution
`primarySeriesCols` already does elsewhere in `home.go`, for consistency with
how the rest of the app picks one authoritative series per book) and branch:

```
target_variant = ''      → NOT EXISTS(any file)                [unchanged]
target_variant = 'mono'  → NOT has_mono_file
target_variant = 'color' → NOT has_color_file
target_variant = 'both'  → NOT (has_mono_file AND has_color_file)
```

Same shape applies to `SearchSeriesPacks`' missing-count and
`importPackExtras`' owned-check, which both currently use `HasFile`/`v.HasFile`
identically to `wantedWhere`.

## Search & scoring: steer toward the missing variant, never hard-reject

Once a volume search knows it specifically needs (say) color because mono is
already owned, `release.ScoreVolume` needs a `wantedVariant string`
parameter. The signal available here is **asymmetric** — exactly like
`scanner.DetectVariant`, a release title can confidently say "colorized," but
nothing reliably says "monochrome" (it's the unmarked default; scanlation/
digital-release groups don't tag the normal case). So:

- A release title carrying the colorized keyword (reuse the exact regex from
  `scanner.colorizedKeywords` — factor it out so both `scanner.DetectVariant`
  and `release.ScoreVolume` call the same matcher, so the keyword list never
  drifts between the two call sites) gets a **soft score bonus** when
  `wantedVariant == "color"`, and a **soft penalty** when
  `wantedVariant == "mono"` — hunting for mono, a release that announces
  itself as colorized is almost certainly the wrong one.
- An unflagged release (the common case, by far) gets **no adjustment either
  way** — neutral, fully eligible. This must stay an additive `Score +=/-=`,
  never a hard `c.reject(...)`: a hard reject on missing signal would make
  "want mono, already have color" nearly unsatisfiable by search, since mono
  releases essentially never self-identify.

## Import time: a solid match that can't confirm its variant

This is the harder question, because it's not a ranking problem anymore —
it's what gets **written to the database as fact**. Scenario: `target_variant`
says `both`, mono is owned, color is still wanted. A release downloads with a
great title/volume match, but the actual archive has no `ComicInfo.xml` and
no colorized filename hint — `scanner.DetectVariant` returns `""`, same as it
does for the ordinary case today.

**Failure mode if we guess wrong:** once a `book_files` row exists with (say)
`variant='mono'`, `wantedWhere` sees `has_mono_file=true` and stops
searching — the real color edition becomes *permanently* invisible as
still-wanted, with nothing visibly wrong on the page. Quiet data corruption
of the collection's completeness — worse than almost any other failure mode
here, because nothing ever surfaces it.

**Failure mode if we never guess:** mono essentially never self-announces, so
refusing to act on ambiguity would mean every "want both" series missing its
mono copy can *never* be closed by automation — only by a human manually
resolving it, indefinitely. That defeats the point of building this at all.

### Prior art I went and checked

- **`bestNarrator()` in `internal/library/files.go`** (this codebase, already
  shipped): when two audiobook editions are close enough in duration that the
  match is genuinely ambiguous, it returns `""` — "name no one rather than
  guess wrong." A deliberate, already-proven house philosophy for exactly
  this *shape* of problem, not something invented for this doc.
- **[Kavita issue #3803](https://github.com/Kareadita/Kavita/issues/3803)**
  ("One Piece" vs "One Piece (Color)" merged into one series, pages
  interleaved): confirms this exact class of problem is real and hits real
  users on the *reading* side too — and as of this research, it's an
  open, unresolved bug report with no maintainer response. Kavita doesn't
  have a solved answer to point to here.
- **ComicInfo.xml's `Format` field** (Kavita's own wiki): a recognized-keyword
  field, but for *packaging* type — `Special`, `Omnibus`, `TPB`, `Annual`,
  `One Shot`, etc. — not color. Confirms `BlackAndWhite` really is the
  purpose-built field for this question, not a repurposing of something
  adjacent; `Format` is a different, unrelated axis (and a plausible, separate
  future LibriNode feature — distinguishing floppy issues from omnibus
  collections — but out of scope here).
- **Radarr/TRaSH Guides' Custom Formats scoring philosophy**
  ([trash-guides.info](https://trash-guides.info/Radarr/radarr-setup-quality-profiles/)):
  the most mature, widely-deployed scoring system in the whole *arr
  ecosystem. Its stated principle, verbatim: **"undetectable attributes
  aren't penalized."** Custom Formats are additive/subtractive score
  deltas, not hard filters, specifically because a release's failure to
  announce an attribute is not evidence against it. This is independent,
  authoritative confirmation of the exact soft-scoring design above — and it
  matches the additive-scoring architecture `release.Score` already uses
  throughout LibriNode (`RetailBonus`, format scores, etc.), so this isn't a
  new pattern for the codebase, just a new axis for an existing one.
- **Lidarr's open feature request #344** ("Support for different versions of
  albums" — deluxe vs. standard, same underlying work): still unresolved as
  of this research. The community's own proposed fix is a heuristic — guess
  from the downloaded file *count* — matching the shape of "infer from
  context when the title can't tell you," not a clean solved answer. This is
  a genuinely hard, still-open problem across the ecosystem, not a LibriNode-
  specific gap.
- **Readarr's answer to the same general shape of problem** (one logical
  book, two real manifestations — ebook and audiobook): *"you will need
  multiple instances."* It doesn't attempt to track both in one install.
  Worth noting as context: LibriNode already does better than this on the
  adjacent problem (ebook+audiobook as two format-libraries of one book,
  and now manga mono+color as two variant-files of one book) — this item
  extends that same already-ahead design, rather than bolting on something
  foreign to it.

### Recommendation

A middle path, in the spirit of `bestNarrator` but adapted since "decline
silently" isn't viable here the way it is for narrator-naming (a blank
narrator name costs nothing; an unresolved "still wanted" flag costs an
unsatisfiable search forever):

1. If detection is inconclusive **and** the book already owns exactly one of
   the two wanted variants, infer the new file is the *other* (missing)
   one — the only inference that's actually cheap here: a release duplicating
   the edition already confirmed owned should mostly have been deprioritized
   by the search-time soft-penalty above already, so surviving to import is
   itself weak evidence it's the new one. (Mirrors the shape of Lidarr's own
   community-proposed file-count heuristic for its unsolved deluxe/standard
   problem — inference from context, not from the title.)
2. **Mark that inference, don't bury it** — a notice in Activity/the import
   result: *"Imported as presumed color (couldn't confirm from the file) —
   verify, or correct from the book page's ⋯ menu."* This is "don't silently
   guess wrong" honored without refusing to act — visible and correctable,
   not asserted as fact.
3. If the book owns **neither** variant yet (the plain single-root case —
   today's overwhelming majority), there's no "other" to infer toward — fall
   back to the configured root exactly as today. No change, no new risk, for
   the common case.

Point 3 matters most in practice: everything above is scoped to
`target_variant != ''` specifically. For every series that doesn't opt in —
nearly all of them — behavior is **completely unchanged**.

### Open question for the user, not decided here

Step 2's "infer + flag" versus a stricter "never auto-assign when ambiguous,
leave it in Unmatched for manual resolution" (closer to `bestNarrator`'s
*literal* behavior) is a real risk-tolerance trade-off: more automation with a
correctable flag, vs. more certainty with more manual work. Leaning toward
the flagged inference, since "Unmatched forever" seems unlikely to actually
get resolved in practice — but it's the user's collection's correctness on
the line, so this should be confirmed with them before building, not decided
unilaterally.

## Alternative considered: let the metadata provider solve it

Before settling on file-level detection as the primary mechanism, I checked
whether Hardcover's own catalog already distinguishes official colorized
reprints — if "One Piece" and "One Piece: Digital Colored Comics" are
tracked as genuinely separate catalog entries, a user could just add the
colorized one as its own series pointed at a color root, sidestepping file
detection for anything officially published. Probed live against Hardcover's
real GraphQL API (not speculation):

- **Catalog series ARE sometimes split for real editions** — "One Piece 3-in-1
  Omnibus" has its own `series_id` (66415) distinct from base "One Piece"
  (4624); "Death Note: Black Edition" is its own series (5637, `books_count:
  8`, `compilation: true`) distinct from base "Death Note" (7310). *(Note:
  "Black Edition" turned out to be a 2-in-1 omnibus repackaging, not a
  recolor — the wrong example to reach for here; corrected mid-research.)*
- **But not for colorization specifically.** Searching Hardcover directly for
  `"One Piece Digital Colored Comics Vol 1"` doesn't surface that edition at
  all in the top results — the official VIZ colorized line doesn't appear to
  be tracked as its own entry for this series.
- **Introspected the `editions` GraphQL type directly** — the full field
  list has no `color`/`black_and_white`/anything adjacent. The closest
  candidate, free-text `edition_information`, was checked against all ~47
  real editions of One Piece Vol. 1 Hardcover has on file: publisher imprints,
  "20th Anniversary Limited Edition," regional-language notes — not one
  mentions color.

**Conclusion: no shortcut here.** This isn't a provider-metadata gap LibriNode
can route around — confirmed empirically, not assumed. File-level detection
(`ComicInfo.xml` + filename, already shipped) has to stay the primary
mechanism; the provider only occasionally helps for the *different* axis of
omnibus/repackaging editions, which is a separate, smaller feature if ever
worth doing (see `Format` in the ComicInfo.xml section above).

## Touch list

| Area | File | Change |
|---|---|---|
| Schema | new migration | `series.target_variant` |
| Model | `internal/library/library.go` | `Series.TargetVariant` |
| Store | `internal/library/*.go` | read/write, mirrors `ProviderOverride` |
| API | `internal/api/library.go` + router | `PUT /series/{id}/variant`-style endpoint |
| Wanted | `internal/library/home.go` `wantedWhere` | manga branch, series JOIN |
| Pack search | `internal/autosearch/autosearch.go` `SearchSeriesPacks` | `HasFile` → per-variant check |
| Shared keyword | `internal/scanner/variant.go` | export `colorizedKeywords` (or a small shared matcher) so `release` reuses it without drift |
| Scoring | `internal/release/score.go` `ScoreVolume` | `wantedVariant` param, soft +/- |
| Search query | `internal/autosearch/autosearch.go` | resolve `wantedVariant` before scoring |
| Import inference | `internal/importer/importer.go` (the block added for variant detection) | "infer from what's already owned" step + flagged notice |
| UI | series page | the target-variant control, root-count-gated |
