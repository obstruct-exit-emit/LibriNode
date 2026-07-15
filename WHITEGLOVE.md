# White-glove backlog

UX polish + power-feature tracker for LibriNode: making the app powerful,
user-friendly, good-looking, and consistent. Grounded in a code audit of
`web/src` (2026-07). **Magazines are intentionally out of scope** for now.

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

## P1 — most visible, worst polish gap
- [ ] **Search-and-add flow.** The primary "add content" surface is plain text
      rows (`name · N books` + Add button) with no cover art, descriptions, or
      grid — the least polished screen in the app. Give it the poster-grid look
      with covers/year/blurb. `BooksLibraryView` AddPanel + series add.
- [ ] **Kill the 12 native `confirm()` dialogs.** Unstyled browser popups for
      remove-client/indexer/root-folder/user, disable-login, clear-caches,
      restore/delete-backup. A styled confirm (`RemovePanel`) already exists —
      unify on one. `ActivityView`, `SettingsView`, `SystemView`.
- [ ] **Fix the `confirm()`-as-monitor-prompt.** `BookDetailView:188` uses a
      yes/no dialog to make a three-way choice ("OK = monitor, Cancel = just
      add"). Replace with a real small dialog with labeled options.

## P2 — systemic consistency
- [ ] **Unified toast/notification layer.** Errors are one big top-of-page red
      card (a new one replaces the old); success notices are inline per
      component in ~4 formats. Add dismissible, stacking toasts used everywhere.
      `App.tsx` error card + scattered `notice ok/bad`.
- [ ] **Skeleton loading states.** Replace plain "Loading…" / "Loading book…"
      text with poster-grid and detail-header skeletons.

## P3 — per-surface passes not yet done
- [ ] **Author & Series detail pages** — got download badges but no full pass;
      action rows, Missing rows, and file lists are utilitarian.
- [ ] **Quality Profiles editor** — formats are a raw comma-separated text
      field; make a format-chips / drag-to-order editor (prettier + powerful).
- [ ] **Indexers & Download Clients cards** — saved-item rows are tiny text
      buttons (test/enable/remove); match the rest of Settings.
- [ ] **System page** (backups / logs / health) — never polished; basic backup
      rows and log viewer.
- [ ] **Calendar view** — plain agenda list, untouched.
- [ ] **Auth entry screens** (`LoginForm`, `ApiKeyForm` in `App.tsx`) — the
      first thing a non-wizard user sees; no polish pass.

## Power — the "powerful" half
- [ ] **Bulk actions.** Everything is one-at-a-time. Multi-select on library
      grids / Missing / Wanted for bulk monitor / grab / remove.
- [ ] **Global search.** Add-search is per-library only; a top-bar search across
      all libraries would be a big win.
- [ ] **Series pack grab.** Known content gap — manga/comic torrents are
      whole-series packs that get rejected. A deliberate series-level pack grab
      (reusing the pack importer) would close it.

## Foundation — cross-cutting
- [ ] **Shared formatting utils.** Three different byte formatters
      (`formatSize` in BookDetailView, `fmtSize` in ReleaseBrowser without a KiB
      tier, and a raw `(size/1024) KiB` inline in `SeriesDetailView` — the
      "936278 KiB" huge-number bug still shows there). One `formatBytes` +
      `formatDate` + `relativeTime` util, used everywhere.
- [ ] **Accessibility.** Near-zero today: no `role=`, icon-only buttons (✕,
      remove, toggles) rely on `title` not `aria-label`, book covers use empty
      `alt`, no focus trapping on inline dialogs, ~one keyboard handler in the
      whole app. Add labels, focus management, and keyboard paths.
- [ ] **Responsive / mobile.** Only two media queries (sidebar collapse at
      800px, detail-head stack at 600px). Poster grids, Settings forms, the
      release browser controls, and Activity rows have no responsive handling;
      the mobile sidebar is icon-only with hidden group labels.

## Investigate before scoping
- [ ] **Empty / first-use states** — friendlier zero-state per library beyond
      "+ Add" text.
- [ ] **Remote Path Mapping UI** — still relies on mounting at an exact path; a
      real Settings feature (map client prefix → local path) is still open.

---
Suggested order: P1 search-and-add + the `confirm()` cleanup, then P2 toasts
(makes everything else feel finished), then the shared formatting util (cheap,
fixes a real inconsistency), then work down P3 / Power.
