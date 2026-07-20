# Installation

LibriNode is a single self-contained binary; the web UI is embedded. Data
(config, database, logs, backups) lives in one directory: `%AppData%\LibriNode`
on Windows, `~/.config/librinode` on Linux, or wherever `--data <dir>` points.

## Docker

Images are published per release to
`ghcr.io/obstruct-exit-emit/librinode`. `:latest` tracks stable releases and
arrives with the first non-rc tag; pre-release builds are pullable by their
exact version tag (e.g. `:0.9.0-rc.3`). Or build the image yourself from a
repo checkout — the example below tags a local build `librinode`.

```sh
docker build -t librinode .   # or: docker pull ghcr.io/obstruct-exit-emit/librinode:0.9.0-rc.3
docker run -d --name librinode -p 7845:7845 \
  -e PUID=1000 -e PGID=1000 \
  -v /path/to/config:/config \
  -v /path/to/media:/media \
  librinode                   # use the ghcr.io/... ref instead if you pulled it
```

See `docker-compose.example.yml` in the repository for a full compose file
with per-type media mounts. Mount your download client's completed folder at
the same path in both containers so imports can find the files.

## Linux (bare metal)

Download the release tarball (or build from source), then:

```sh
sudo useradd --system --home /var/lib/librinode --create-home librinode
sudo cp librinode /usr/local/bin/
sudo cp packaging/systemd/librinode.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now librinode
```

The unit ships with filesystem hardening — add your media folders to
`ReadWritePaths=` so the scanner and organizer can touch them.

## Windows

Unzip the release, then from an elevated PowerShell in that folder:

```powershell
powershell -ExecutionPolicy Bypass -File install-librinode.ps1
```

This registers LibriNode to start at boot (Task Scheduler, runs as SYSTEM,
data in `C:\ProgramData\LibriNode`). `uninstall-librinode.ps1` removes it
without touching your data. A signed installer with a native Windows service
ships with 1.0.

## From source

Requires Go 1.25+ and Node 22+:

```sh
cd web && npm install && npm run build && cd ..
go build ./cmd/librinode
./librinode
```

Open `http://localhost:7845` when it's running.
