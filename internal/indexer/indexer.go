// Package indexer implements Quillarr's indexer framework: Newznab (usenet)
// and Torznab (torrent) clients behind one API, indexer configuration
// storage, and aggregated release search. Release scoring and automatic
// grabbing build on top of this in later Phase 2 slices.
package indexer

// Indexer types. Torznab is Newznab's API shape served by torrent indexers
// (Jackett/Prowlarr style) with torrent-specific attributes on results.
const (
	TypeNewznab = "newznab"
	TypeTorznab = "torznab"
)

// Protocols derived from the indexer type.
const (
	ProtocolUsenet  = "usenet"
	ProtocolTorrent = "torrent"
)

// Indexer is one configured indexer endpoint.
type Indexer struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	BaseURL    string `json:"baseUrl"`
	APIKey     string `json:"apiKey"`
	Categories string `json:"categories"` // comma-separated Newznab category ids
	Enabled    bool   `json:"enabled"`
	Priority   int    `json:"priority"` // 1-50, lower wins ties
	AddedAt    string `json:"addedAt"`
}

// Protocol reports how releases from this indexer are downloaded.
func (i *Indexer) Protocol() string {
	if i.Type == TypeTorznab {
		return ProtocolTorrent
	}
	return ProtocolUsenet
}

// Release is one search result from an indexer — a candidate file for a
// wanted book. Scoring and mapping to library books happen in later slices.
type Release struct {
	IndexerID   int64  `json:"indexerId"`
	Indexer     string `json:"indexer"`
	Protocol    string `json:"protocol"`
	Title       string `json:"title"`
	GUID        string `json:"guid"`
	InfoURL     string `json:"infoUrl,omitempty"`
	DownloadURL string `json:"downloadUrl"`
	Size        int64  `json:"size"`
	PublishDate string `json:"publishDate,omitempty"`
	Categories  []int  `json:"categories,omitempty"`
	// Torrent-only; -1 means unknown/not applicable (usenet).
	Seeders int `json:"seeders"`
	Peers   int `json:"peers"`
}
