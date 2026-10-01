-- Per-book ebook↔audiobook mirror. Like the author-level mirror (authors.mirror)
-- but scoped to a single title: owning or wanting it in one format wants it in
-- the other, without mirroring the author's whole bibliography. A book is
-- mirrored when its own flag OR its author's flag is set.
ALTER TABLE books ADD COLUMN mirror INTEGER NOT NULL DEFAULT 0;
