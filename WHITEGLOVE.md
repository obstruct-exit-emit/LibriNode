# White-glove backlog

UX polish + power-feature tracker for LibriNode: making the app powerful,
user-friendly, good-looking, and consistent. Grounded in a code audit of
`web/src` (2026-07). **Magazine acquisition is disabled** (organize-only
library) — magazine polish is limited to the organizational surfaces.

Priorities: **P1** most visible / worst gap · **P2** systemic consistency ·
**P3** per-surface pass · **Power** capability · **Foundation** cross-cutting.

## Done (this pass)
- [x] Settings visual polish — sections, collapsible advanced, consistent save state
- [x] Multi-user Security card — user list, change password, make default, remove, add, disable
- [x] Visual folder browser for root folders (Settings + setup wizard)
- [x] Release browser — sort/filter, protocol/score/format pills, per-row grab state, seeders/leechers/size/age always shown
- [x] Live download badges — book page, series volume rows, Wanted cards (shared cached queue poll)
- [x] Multi-file audiobooks — disc subfolders, collision-safe flatten, per-book folder unit
- [x] Cross-format book links · clickable Home tiles · first-run setup wizard
- [x] Existing-file import — ALL five libraries: confident suggestions with a
      0–100% confidence rating, one-click Import + bulk "Import all matched",
      duplicate resolution (Replace/Delete, variant-aware for manga),
      one-click add of missing author/series/magazine from the row; magazine
      imports materialize the issue; scanner keeps matches sticky across
      organizes
- [x] Magazines switched to organize-only (acquisition disabled everywhere;
      engine kept for later)
- [x] **P1: Search-and-add flow** — shared AddResultsGrid poster cards with
      cover art, title/subtitle, clamped blurbs (series), per-card
      adding→added state; wired into prose AddPanel and series add
- [x] **P1: All 13 native `confirm()` dialogs killed** — app-wide styled
      confirm dialog (title, danger styling, Escape/backdrop cancel) via the
      new `useUi()` layer
- [x] **P1: `confirm()`-as-monitor-prompt** — cross-format add is a real
      inline three-way choice (Add + monitor / Just add / cancel)
- [x] **P2: Toast layer** — stacking, dismissible toasts (`web/src/ui.tsx`);
      every view's errors surface as toasts (the connection error keeps its
      card for its recovery UI); add flows toast successes. Some contextual
      inline notices (form validation, scan summaries) intentionally remain.
- [x] **P2: URL routing / deep-linking** — hash router in App.tsx: every
      page has a URL, refresh keeps the page, back/forward work, views are
      bookmarkable
- [x] **P2: Edit-in-place** — indexers, download clients, and quality
      profiles load into their form via an edit button (Save changes /
      Cancel); no more delete-and-re-add to change a URL or format list
- [x] **P2: Skeleton loading states** — shimmering poster-grid, detail-head,
      and list skeletons replace every bare "Loading…" text
- [x] **Power: Indexer/client priority UI** — the 1–50 priority (lower wins)
      is editable under Advanced on both forms
- [x] **Power: Global search** — sidebar search box across every library
      (authors, prose books, series/magazines) with grouped poster results
- [x] **Foundation: shared formatting utils** — one formatBytes (the
      "936278 KiB" bug is dead) + formatDate + relativeTime in
      `web/src/format.ts`; history/blocklist/backups now show ages
- [x] **P3: Quality Profiles editor** — formats are ordered chips (‹ ›
      reorder, ✕ remove, suggestion-backed add per media type) instead of a
      raw comma field
- [x] **P3: Indexers & Download Clients cards** — saved rows are two-line
      items with protocol/priority/disabled pills and the URL underneath,
      instead of everything squeezed into one line of tiny buttons
- [x] **P3: Calendar** — every item is clickable (prose → book page,
      volumes/issues → series page; backend now sends authorId/seriesId,
      covered by TestCalendarNavIDs), with relative when-badges (today /
      tomorrow / in Nd / Nd ago) on day headers
- [x] **P3: System page** — status card first as an uppercase-labeled tile
      grid, ERROR/WARN log lines colored, backups show ages
- [x] **P3: Auth entry screens** — centered branded cards with welcome
      copy and stacked full-width form fields
- [x] **P3: Author & Series detail** — ownership meter (owned/total
      progress bar) in both headers; file lists and sizes had already been
      unified in the formatting pass

## P3 — per-surface passes not yet done
- [ ] **Deeper detail-page work** (beyond the completed header/meter pass):
      Missing rows and file lists could still carry richer actions — batch
      monitor from Missing, per-file actions inline.

## Power — the "powerful" half
- [ ] **Bulk actions.** Everything is one-at-a-time. Multi-select on library
      grids / Missing / Wanted for bulk monitor / grab / remove.
- [ ] **Series pack grab.** Known content gap — manga/comic torrents are
      whole-series packs that get rejected. A deliberate series-level pack grab
      (reusing the pack importer) would close it.
- [ ] **Configurable timings.** Wanted-search (6h), metadata refresh (24h),
      health check (15m), import (1m), stale-grab grace (30m) are hardcoded in
      `main.go`/`importer.go` (the code even flags "not yet configurable").
      Expose the useful ones (search interval especially) in an advanced
      settings section.

## Foundation — cross-cutting
- [ ] **Accessibility.** Improved at the edges (aria-labels on new
      components, role=dialog/status, Escape handling on the confirm modal)
      but still no systematic pass: icon-only buttons rely on `title`, book
      covers use empty `alt`, no focus trapping. Add labels, focus
      management, and keyboard paths.
- [ ] **Responsive / mobile.** Only two media queries (sidebar collapse at
      800px, detail-head stack at 600px). Poster grids, Settings forms, the
      release browser controls, and Activity rows have no responsive handling;
      the mobile sidebar is icon-only with hidden group labels.

## Investigate before scoping
- [ ] **Empty / first-use states** — friendlier zero-state per library beyond
      "+ Add" text.
- [ ] **Remote Path Mapping UI** — still relies on mounting at an exact path; a
      real Settings feature (map client prefix → local path) is still open.
- [ ] **Activity History** — capped at 200 rows (backend `LIMIT 200`) with no
      filter, search, or paging; it's a collapsible dropdown now but gets
      unwieldy on a busy instance.
- [ ] **Light theme / UI preferences** — dark-only today (`color-scheme:
      dark`, no light vars). README defers a theme/language/dates prefs page
      to post-1.0; listed here so it isn't forgotten.
- [ ] **Session ↔ user binding** — sessions are anonymous tokens, so removing a
      user doesn't end their open session until the next restart. Fine for a
      trusted instance; revisit if accounts ever gate different access.

---
All P1, P2, and P3 passes are done, plus global search, priority controls,
and the shared formatting utils. What remains is the Power list
(configurable timings, bulk actions, series pack grab) and the closing
foundation sweeps (accessibility, responsive). Series pack grab is the one
real content-gap feature left.
