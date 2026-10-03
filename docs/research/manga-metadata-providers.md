# Research: is there a better metadata provider for manga?

Status: **research complete, one actionable conclusion.** Every claim below
is backed by a live probe against the real API (not documentation summaries
taken on faith, not reputation) — commands and raw responses were run
against each provider's production endpoint while researching this.

## What "better" has to mean

`metadata.SeriesProvider.GetSeries` needs `Issues []Issue` — **real per-volume
records**: number, title, description, cover, release date. That's the bar
every candidate is checked against. A provider that only gives a series-level
volume *count* forces LibriNode to synthesize `Vol. 1..N` with no per-volume
date or cover — which is exactly AniList's current, confirmed-in-code
limitation (`internal/metadata/anilist/anilist.go`: *"AniList has no
per-volume records; synthesize Vol. 1..N"*).

## The one real finding: LibriNode already has a better option — Hardcover

Checked Hardcover's manga-series path
(`internal/metadata/hardcover/series.go`) against the same interface: each
volume comes back as a **real `book` entity** — its own title, description,
release date, cover image — plus per-edition language/country data used to
pick the best printing. Not synthesized. This is the *same* machinery
Hardcover already uses for prose books, applied to manga volumes.

Verified this isn't just true in theory — live-searched 12 real series across
three tiers:

| Tier | Titles checked | Result |
|---|---|---|
| Current/popular | Kaiju No. 8, Dandadan, Oshi no Ko, Sakamoto Days, Witch Hat Atelier | All found, real per-volume entries, real covers |
| Classic | Death Note, One Piece, Mushoku Tensei | Found (Mushoku Tensei's top hit was the light novel series, not the manga adaptation — a disambiguation wrinkle, not a coverage gap; both are separately cataloged and `SearchSeries` returns both for the user to pick) |
| Niche / cult | Dorohedoro, Goodnight Punpun, Girls' Last Tour, Dai Dark, Blame!, Fire Punch | All found |

**Recommendation: make Hardcover the manga default (or just point your own
install at it) instead of AniList.** This needs **zero new code** — it's
already a fully-supported `SeriesProvider`, switchable in place per the
existing "switchable with in-place re-sourcing" behavior. This is the only
change here that's purely a config decision, not a build.

**The honest caveat:** Hardcover's strength comes from it fundamentally being
a *book*-cataloging site — it's strong wherever an official print/digital
edition exists. A title that's *never* been officially licensed/published
anywhere (pure web-only or scanlation-only content) is the one case where
AniList, fed directly by the anime/manga fan community rather than book-trade
data, would plausibly still have an entry Hardcover doesn't. For a library
tool whose owned files are overwhelmingly going to be officially-published
material (clean archives, consistent volume numbering — the kind of thing
that actually gets properly scanned/released), this is a narrow edge case,
not a reason to hold off.

## What I checked and ruled out, with evidence

### MangaUpdates — wrong data model, not a coverage problem

The community's most-cited name for manga tracking. It has a real, public,
keyless API (`api.mangaupdates.com`) — but live-probed a real series
(`Kaiju No. 8`, id `19101650311`) and its "volume" data is a **free-text
status string**: `"status": "16 Volumes (Complete)"`. No structured
per-volume array; `latest_chapter` is the only numeric field, and it's
chapter-level. MangaUpdates was built *for scanlation-group release
tracking* (who translated what, when), not as a volume bibliography.
Integrating it would mean regex-parsing a free-text field to synthesize
volumes — the same fragility AniList already has, with a *less* reliable
source string (not every series' status field is cleanly formatted).
**Not an improvement over what's already there.**

### MyAnimeList / Jikan — reliability, not data shape, is disqualifying

MAL has no clean public API of its own; Jikan is the de facto standard
wrapper, but it's an **unofficial scraper** of MAL's website. Documented
rate limits (60 req/min) and a known failure mode: *"you can receive a 429
... if MyAnimeList is rate-limiting the Jikan servers."* Tried to live-probe
it twice, on two different well-known titles (`Kaiju No. 8`, then `Berserk`),
independently, several seconds apart: both times Jikan itself reported
`"Jikan failed to connect to MyAnimeList. MyAnimeList may be down/unavailable
or refuses to connect."` That's not a one-off — it's the exact failure mode
the community already documents, reproduced live. For a background service
that needs to reliably hit a provider on every add/refresh, unattended, this
is disqualifying on its own — independent of whatever MAL's actual manga
data shape looks like (which, for the record, is also integer-only
`volumes`/`chapters`, same ceiling as AniList, per MAL's well-established API
behavior).

### Kitsu — reliable, but no better, and worse where it matters most

Kitsu is reachable and has an open, keyless, official API (`kitsu.io/api/edge`) —
no reliability red flag like Jikan. But checked it head-to-head against
AniList on the exact same titles:

| Title | Kitsu `volumeCount` | AniList `volumes` |
|---|---|---|
| One Piece | `None` | `None` |
| Berserk | `0` | `None` |
| Chainsaw Man | `11` | `24` |
| Death Note | `12` | `12` |

Two things fall out of this:

1. **Even where Kitsu has a number, it's the same integer-only ceiling as
   AniList** — no real per-volume records there either. No structural
   improvement.
2. **For the two most iconic, almost-certainly-owned series in the test set
   (One Piece, Berserk), both providers come back empty.** This turned out
   to be a genuine, shared limitation of community-maintained manga
   databases generally, not a Kitsu-specific gap: **volume counts are
   reliably tracked once a series finishes, and routinely missing or stale
   while a series is still actively releasing** — which describes most of
   the manga anyone is actively collecting and wanting Missing/Wanted
   tracking for. Switching from AniList to Kitsu would not fix this; neither
   would any other count-only provider.
3. The Chainsaw Man mismatch (11 vs 24) is its own warning sign: the two
   providers don't even agree with each other on how to count an ongoing,
   multi-part series — a reminder that "switch providers" risks subtly
   different series/volume boundaries, not just better or worse completeness.

**Not an improvement.**

### ComicVine — wrong tool, not evaluated further

Already integrated, but deliberately scoped to **comics**, not manga
(`ComicSeriesFactory` vs `SeriesFactory` in `hardcover/series.go`; AniList is
the dedicated manga path). ComicVine's own community and catalog are
Western-comics-focused by design — reusing it for manga would be pointing
the right kind of tool at the wrong catalog, not a data-quality question
worth a live probe.

## Bottom line

One real, immediately actionable change: **Hardcover over AniList for
manga**, for anyone who wants real per-volume dates/covers instead of
synthesized `Vol. N` placeholders — a config change, not a build. Everything
else checked (MangaUpdates, MyAnimeList/Jikan, Kitsu) has a concrete,
now-verified reason it would *not* be a genuine upgrade, not just a weaker
one. AniList stays worth keeping around as the fallback for the narrow
unlicensed/web-only case Hardcover can't reach — but shouldn't be the
default anymore now that this has actually been checked.

The still-open, structurally different problem — ongoing series' volume
counts being incomplete at *every* provider checked — is a separate thing
no provider swap fixes. (It does, incidentally, make a decent case for why
scanning/importing actual owned files — which is what the variant-detection
work already does — matters more than leaning on upstream metadata for an
ongoing series' true volume count.)
