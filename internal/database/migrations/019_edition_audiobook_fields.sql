-- Audiobook-specific edition metadata. An audiobook edition of a prose book
-- carries a narrator, a total runtime, and whether it's abridged — none of
-- which the print/ebook editions have. Empty/zero for non-audiobook editions.
ALTER TABLE editions ADD COLUMN narrator TEXT NOT NULL DEFAULT '';
ALTER TABLE editions ADD COLUMN runtime_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE editions ADD COLUMN abridged INTEGER NOT NULL DEFAULT 0;
