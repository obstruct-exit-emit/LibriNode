-- Other, distinct series a search for this series' own title surfaces at the
-- provider (e.g. "Dragon Ball Super" alongside "Dragon Ball") — fetched at
-- add/refresh time and used to stop a release that actually names one of
-- them from being accepted as a tag-decorated release of this series.
-- Newline-joined, same convention as books.genres.

ALTER TABLE series ADD COLUMN sibling_titles TEXT NOT NULL DEFAULT '';
