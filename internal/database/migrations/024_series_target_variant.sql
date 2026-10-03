-- Per-series opt-in for which manga/comic variant(s) are wanted. '' (the
-- default) is today's behavior — any one variant satisfies "owned" for a
-- volume. Explicit 'mono'/'color'/'both' makes a volume with only the
-- other variant(s) still count as missing, so a series collecting both the
-- original and a colorized reprint can actually finish searching for both
-- instead of stopping the moment either one is found.

ALTER TABLE series ADD COLUMN target_variant TEXT NOT NULL DEFAULT '';
