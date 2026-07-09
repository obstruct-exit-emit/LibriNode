# Configuration

## config.yaml

Created on first run in the data directory, with a generated API key:

```yaml
host: 0.0.0.0
port: 7845
api_key: <generated>
log_level: info        # debug, info, warn, error
auth:                  # present once a login account is set
  username: you
  password_hash: pbkdf2-sha256$...
metadata:
  active: hardcover
  providers:
    hardcover: { token: "..." }
    comicvine: { token: "..." }
naming:
  ebook_folder: "{Author Name}"
  ebook_file: "{Series Title} {Series Position} - {Book Title}"
  # audiobook_*, manga_*, comic_*, magazine_* — all editable in the UI
```

Environment variables override the file: `LIBRINODE_HOST`, `LIBRINODE_PORT`,
`LIBRINODE_API_KEY`, `LIBRINODE_LOG_LEVEL`, `LIBRINODE_HARDCOVER_TOKEN`.
The data directory itself is chosen with `--data <dir>`.

## Naming templates

Tokens: `{Author Name}`, `{Author SortName}`, `{Book Title}`,
`{Series Title}`, `{Series Position}`, `{Release Year}`. Tokens without a
value drop out cleanly; emptied fields revert to defaults (a partial save
can never wipe another type's templates).

## Authentication

Set a username/password under **Settings → General → Security** to replace
the API-key prompt with a login page (30-day in-memory sessions — restarts
sign everyone out). Passwords are stored only as PBKDF2-SHA256 hashes. The
API key keeps working for Prowlarr and scripts, and can be regenerated from
the same page. For HTTPS, run behind a TLS-terminating reverse proxy (Caddy
or nginx examples in the README) and never expose the raw port.

## Health checks

Every 15 minutes (and on demand from the System page) LibriNode verifies
root folders are reachable, enabled indexers answer, download clients are
up, and the metadata token is valid — plus warnings when nothing is
configured at all. Issues appear as a banner on every page.

## Logs

`<data>/logs/librinode.log`, size-rotated (5 MB, 3 old files kept). The
System page tails it with a text filter; `log_level: debug` for more.

## Backups

**System → Backups**: a backup is a zip of a consistent database snapshot
plus `config.yaml`, stored under `<data>/backups`. Restore stages the files
and applies them on the next restart, keeping the replaced ones as
`*.pre-restore`. Download the zips somewhere safe.

## Image cache

Two kinds of images are cached under `<data>/covers`, both disposable and
safe to delete (they rebuild on demand):

- **Extracted comic covers** (`covers/book-<id>`): a manga/comic volume's
  cover, pulled from the owned archive's first page and re-extracted when the
  source file changes. Clear it from **Settings → Metadata → Clear extracted
  covers**.
- **Provider art** (`covers/remote/…`): author portraits and series/book
  covers from Hardcover/AniList/ComicVine, downloaded on add/refresh so the
  UI serves them locally and they survive the provider's link rot.

**Settings → Metadata** has buttons to clear each of these, plus
**Descriptions** (stored in the database — cleared descriptions return on the
next metadata refresh) and **Clear all**, which wipes every rebuildable cache
at once.
