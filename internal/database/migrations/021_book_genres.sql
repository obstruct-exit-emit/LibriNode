-- Genres for a prose book — captured from the book provider (Hardcover's
-- work-level genre tags), so LibriNode can display them and embed a Genre tag
-- when writing to audiobook files. A work-level property, shared across a
-- book's ebook/audiobook formats. Stored newline-joined; empty when unknown.
ALTER TABLE books ADD COLUMN genres TEXT NOT NULL DEFAULT '';
