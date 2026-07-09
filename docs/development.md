# Development

Go 1.25+ backend, React 19 + Vite frontend, SQLite (pure Go, no cgo).

```sh
go run ./cmd/librinode     # starts on http://localhost:7845
go test ./...
go build ./cmd/librinode   # embeds web/dist if present
```

Frontend (Node 22+):

```sh
cd web
npm install
npm run dev      # Vite dev server, proxies /api to :7845
npm run build    # production build into web/dist
```

> **Windows note:** with Smart App Control enabled, Windows blocks locally
> compiled (unsigned) binaries. Develop inside WSL or disable SAC — official
> releases will be code-signed.

## Layout

```
cmd/librinode/        entrypoint, background loops, restore staging
internal/api/         REST handlers, router, auth, backups
internal/library/     domain model + SQLite store (authors/books/series)
internal/metadata/    provider registry; hardcover/, anilist/, comicvine/
internal/indexer/     Newznab/Torznab clients, search fan-out, backoff
internal/release/     release parsing + scoring
internal/download/    qBittorrent/SABnzbd clients, grabs, blocklist
internal/autosearch/  wanted-list sweeps, per-book search
internal/importer/    Completed Download Handling, seed-goal cleanup
internal/scanner/     library file scanning + matching
internal/organize/    naming-template rename engine (all media types)
internal/naming/      template token rendering
internal/opf/         OPF sidecar rendering
internal/comicinfo/   ComicInfo.xml for CBZ archives
internal/comiccover/  cover extraction from CBZ/CBR archives
internal/imagecache/  provider-image download cache
internal/health/      background health checks
internal/logging/     rotating log file
internal/config/      config.yaml + env overrides
internal/database/    SQLite open + embedded migrations
web/                  React SPA (embedded via go:embed)
docs/                 this documentation (mkdocs)
packaging/            Docker entrypoint, systemd unit, Windows scripts
```

Releases are cut by tagging `v*` — CI builds version-stamped binaries and a
Docker image (see `.github/workflows/release.yml`).

Docs preview: `pip install mkdocs-material && mkdocs serve`.
