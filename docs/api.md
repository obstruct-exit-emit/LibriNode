# API

Everything the UI does goes through the versioned REST API at `/api/v1` —
same endpoints, fully scriptable. Authenticate with the `X-Api-Key` header
(or `?apikey=`), or a login session cookie.

```sh
curl -H "X-Api-Key: <key>" http://localhost:7845/api/v1/system/status
```

| Area | Endpoints |
|---|---|
| System | `GET /system/status`, `GET /ping` (no auth), `GET /health`, `POST /health/check`, `GET /log?lines=N` |
| Auth | `GET /auth/status` + `POST /auth/login` (unauthenticated), `POST /auth/logout`, `PUT /auth/credentials`, `POST /auth/apikey/regenerate` |
| Backups | `GET/POST /backup`, `DELETE /backup/{name}`, `POST /backup/{name}/restore`, `GET /backup/{name}/download` |
| Root folders | `GET/POST /rootfolder` (manga roots take a `"variant"`: `color`\|`mono`, default `mono`), `DELETE /rootfolder/{id}` |
| Search | `GET /search?term=&type=author\|book\|manga\|comic` |
| Authors | `GET/POST /author` (`?library=` scopes; adds take `"library"`), `GET/DELETE /author/{id}` (`?deleteFiles=true`, every library), `PUT /author/{id}/library` (add/remove from ONE format library; `deleteFiles`; auto-deletes the author once in no library), `PUT /author/{id}/monitor`, `POST /author/{id}/refresh` (metadata only — never touches membership/monitoring), `GET /author/{id}/missing?library=` (bibliography gaps), `POST /author/{id}/search?library=` (this author's wanted books only) |
| Books | `GET/POST /book`, `GET/DELETE /book/{id}` (`?deleteFiles=true`), `PUT /book/{id}/library` (membership + monitored + `deleteFiles`; `library:"manga"` adds/removes a volume, `member:false` forgets its file records so it drops to Missing), `PUT /book/{id}/monitor`, `POST /book/{id}/refresh` |
| Series | `GET/POST /series`, `GET/DELETE /series/{id}` (`?deleteFiles=true`), `PUT /series/{id}/monitor`, `POST /series/{id}/refresh`, `POST /series/{id}/search` (this series' wanted volumes only) |
| Libraries | `GET /libraries`, `GET /home`, `GET /wanted?library=X`, `GET /calendar?past=30&days=90` |
| Files | `POST /library/scan`, `GET/POST /library/rename` (preview/apply; `?bookId=`, `?authorId=`/`{"authorId":N}`, or `?seriesId=`/`{"seriesId":N}` scopes, otherwise everything), `GET /bookfile?unmatched=true`, `POST /bookfile/{id}/match`, `DELETE /bookfile/{id}` |
| Indexers | `GET/POST /indexer`, `GET/PUT/DELETE /indexer/{id}`, `POST /indexer/test`, `GET /indexer/schema`, `GET /release?term=` or `?bookId=N&mediaType=` |
| Quality | `GET/POST /qualityprofile`, `PUT/DELETE /qualityprofile/{id}`, `PUT /qualityprofile/{id}/default` |
| Downloads | `GET/POST /downloadclient`, `PUT/DELETE /downloadclient/{id}`, `POST /downloadclient/test`, `POST /release/grab`, `GET /queue`, `GET /history`, `POST /library/import`, `GET /blocklist`, `DELETE /blocklist/{id}` |
| Auto search | `POST /book/{id}/search?mediaType=`, `POST /library/search` |
| Settings | `GET/PUT /settings/metadata`, `POST /settings/metadata/test`, `GET/PUT /settings/naming` |

Notes:

- `POST /author` takes `{"foreignAuthorId": "...", "library": "ebook"}` and
  pulls the full bibliography as metadata — the author joins the library,
  but no book is enrolled or monitored (see Missing, below). `POST /book`
  takes `foreignBookId` the same way and monitors + enrolls that one book.
  `POST /series` takes `{"mediaType": "manga", "foreignSeriesId": ...}`
  or `{"mediaType": "magazine", "title": "..."}`.
- The per-author **Missing** section (bibliography gaps not yet monitored
  or owned in a format library) is `GET /author/{id}/missing?library=`;
  one-click monitor is the existing `PUT /book/{id}/library`.
- The Prowlarr-facing surface emulates Readarr v1 (`/api/v1/indexer` accepts
  Readarr resources; `/system/status` reports a Readarr-compatible
  `version`, LibriNode's own in `appVersion`).
- Without a metadata token, metadata endpoints return 503.
