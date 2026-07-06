package api

import (
	"io/fs"
	"net/http"
	"runtime"
	"strings"
	"time"
)

var startTime = time.Now()

func (s *server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"appName": "LibriNode",
		// Prowlarr's Readarr application sync parses "version" as a dotted
		// .NET Version and enforces a minimum, so this reports a
		// Readarr-compatible number; LibriNode's real version is appVersion.
		"version":    "0.4.18.2805",
		"appVersion": s.version,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"uptime":     time.Since(startTime).Round(time.Second).String(),
		"dataDir":    s.cfg.DataDir(),
		"startTime":  startTime.UTC().Format(time.RFC3339),
	})
}

// handleIndex serves the embedded web UI: real files directly, anything else
// falls back to index.html so client-side routes work. Without an embedded
// build (backend-only compile) it serves a plain status page.
func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Unknown API routes must 404 as JSON, never fall back to the SPA.
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "unknown API route")
		return
	}
	if s.webFS != nil {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" && path != "index.html" {
			if _, err := fs.Stat(s.webFS, path); err == nil {
				http.ServeFileFS(w, r, s.webFS, path)
				return
			}
		}
		http.ServeFileFS(w, r, s.webFS, "index.html")
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!doctype html>
<title>LibriNode</title>
<style>body{font-family:system-ui;display:grid;place-items:center;min-height:90vh;background:#14141b;color:#e8e6e3}main{text-align:center}h1{font-size:2.5rem}p{color:#9a97a3}code{background:#22222c;padding:.2em .5em;border-radius:4px}</style>
<main>
  <h1>&#128396;&#65039; LibriNode</h1>
  <p>The written-media automation server is running.</p>
  <p>This build has no web UI embedded &mdash; run <code>npm run build</code> in <code>web/</code> and rebuild the binary. The API is fully available: try <code>GET /api/v1/system/status</code> with your API key.</p>
</main>`))
}
