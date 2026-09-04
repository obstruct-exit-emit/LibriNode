-- Per-author "mirror" flag. When on, the author's prose books are kept in
-- lockstep across the ebook and audiobook libraries: membership and monitoring
-- move together, so owning or wanting a book in one format pulls in the other
-- (the missing format becomes a wanted item the search scheduler grabs). Off by
-- default; existing authors keep independent per-format state until turned on.
ALTER TABLE authors ADD COLUMN mirror INTEGER NOT NULL DEFAULT 0;
