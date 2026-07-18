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

## Done (power + foundation wave)
- [x] **Power: Series pack grab** — release parsing understands volume
      ranges ("v01-v41", "#1-60") and completeness words; ScoreSeriesPack
      ranks full range > partial > bare series-title, rejects single
      volumes; `GET /release/packs?seriesId=` + "🎁 Search packs" on
      manga/comic series pages (shared ReleaseBrowser in pack mode); the
      grab binds to the first missing volume and the existing pack importer
      fills the rest on completion. The one real content-gap feature.
- [x] **Power: Configurable timings** — wanted search / metadata refresh /
      health checks / import poll are config.TimingSettings (blank =
      default, clamped ranges), edited under Settings → General → Advanced:
      background timings; applied at startup.
- [x] **Power: Bulk actions** (scoped to where one-at-a-time hurt) — both
      Missing sections get per-row checkboxes, "+ Monitor selected (N)",
      and "+ Monitor all (N)" per series group / whole section, with
      settled-batch error reporting.
- [x] **Foundation: responsive pass** — a 700px breakpoint covers poster
      grids, card heads, row-action wrapping, full-width settings fields,
      bottom-sheet toasts, and modal margins; long paths/titles wrap
      instead of overflowing.
- [x] **Foundation: accessibility (edge pass)** — aria-current on nav,
      aria-labels on icon-only and checkbox controls, meaningful alt on
      standalone detail art (grid covers stay decorative alt=""),
      role=dialog/status + Escape on the UI layer.
- [x] **Activity History** — server-side paging with total (LIMIT-200 cap
      gone), debounced title filter, "Show more" progressive loading.
- [x] **Empty / first-use states** — every library's zero state is a
      friendly onboarding block: icon, per-type guidance, and direct
      + Add / Scan files actions.

## Still open (honestly)
- [ ] **Accessibility, the systematic pass** — focus trapping in dialogs,
      full keyboard paths through grids/rows, and a screen-reader walk of
      the main flows. The edge pass above is real but not that.
- [ ] **Mobile sidebar labels** — group labels are still hidden at the
      collapse breakpoint; usable, not lovely.
- [ ] **Per-file actions inline on detail pages** (delete/re-organize one
      file from the file list without leaving the page).
- [ ] **Remote Path Mapping UI** — still relies on mounting at an exact
      path; a real Settings feature (map client prefix → local path) is
      open (also tracked in the README's hardening list).
- [ ] **Light theme / UI preferences** — dark-only today; README defers a
      theme/language/dates prefs page to post-1.0.
- [ ] **Session ↔ user binding** — sessions are anonymous tokens, so
      removing a user doesn't end their open session until the next
      restart. Fine for a trusted instance; revisit if accounts ever gate
      different access.

---
The backlog's P1/P2/P3 passes, the Power list, and the foundation sweeps
are done. What's left is deliberately parked: the deep accessibility pass,
remote path mapping, and the post-1.0 preference page.
