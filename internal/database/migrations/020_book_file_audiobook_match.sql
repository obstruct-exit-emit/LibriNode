-- Records what a duration probe + edition match found for an owned audiobook:
-- its total runtime, and the narrator of the audiobook edition whose runtime it
-- matches. Lets LibriNode name the narrator of a file even when the narrator is
-- nowhere in the filename or tags. Zero/empty until matched; scans never clear
-- them (upsertBookFile leaves these columns alone).
ALTER TABLE book_files ADD COLUMN runtime_minutes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE book_files ADD COLUMN narrator TEXT NOT NULL DEFAULT '';
