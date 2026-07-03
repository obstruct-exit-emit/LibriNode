package api

// Prowlarr application sync: Prowlarr only pushes indexers to *arr apps it
// recognizes, so LibriNode speaks Readarr's v1 API dialect on the indexer
// endpoints — add LibriNode to Prowlarr as a "Readarr" application and it
// manages indexers here automatically. The same endpoints keep accepting
// LibriNode's native JSON; the payloads are distinguished by the arr-style
// "implementation" marker.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/librinode/librinode/internal/indexer"
)

// arrField is the name/value pair *arr resources use for provider settings.
type arrField struct {
	Name  string `json:"name"`
	Value any    `json:"value,omitempty"`
}

// arrIndexerResource is the Readarr v1 indexer shape (the subset Prowlarr
// reads and writes).
type arrIndexerResource struct {
	ID                      int64      `json:"id,omitempty"`
	Name                    string     `json:"name"`
	Implementation          string     `json:"implementation"`
	ImplementationName      string     `json:"implementationName,omitempty"`
	ConfigContract          string     `json:"configContract"`
	Protocol                string     `json:"protocol,omitempty"`
	SupportsRss             bool       `json:"supportsRss"`
	SupportsSearch          bool       `json:"supportsSearch"`
	EnableRss               bool       `json:"enableRss"`
	EnableAutomaticSearch   bool       `json:"enableAutomaticSearch"`
	EnableInteractiveSearch bool       `json:"enableInteractiveSearch"`
	Priority                int        `json:"priority"`
	Tags                    []int      `json:"tags"`
	Fields                  []arrField `json:"fields"`
}

func (r *arrIndexerResource) field(name string) any {
	for _, f := range r.Fields {
		if strings.EqualFold(f.Name, name) {
			return f.Value
		}
	}
	return nil
}

func (r *arrIndexerResource) stringField(name string) string {
	if v, ok := r.field(name).(string); ok {
		return v
	}
	return ""
}

// toIndexer maps an arr resource onto LibriNode's indexer model.
func (r *arrIndexerResource) toIndexer() (*indexer.Indexer, error) {
	ind := &indexer.Indexer{
		ID:       r.ID,
		Name:     strings.TrimSpace(r.Name),
		Priority: r.Priority,
		Enabled:  r.EnableRss || r.EnableAutomaticSearch || r.EnableInteractiveSearch,
	}
	switch strings.ToLower(r.Implementation) {
	case indexer.TypeNewznab:
		ind.Type = indexer.TypeNewznab
	case indexer.TypeTorznab:
		ind.Type = indexer.TypeTorznab
	default:
		return nil, fmt.Errorf("unsupported implementation %q", r.Implementation)
	}

	base := strings.TrimRight(strings.TrimSpace(r.stringField("baseUrl")), "/")
	if base == "" {
		return nil, fmt.Errorf("baseUrl field is required")
	}
	if apiPath := strings.TrimSpace(r.stringField("apiPath")); apiPath != "" && apiPath != "/api" {
		base += apiPath
	}
	ind.BaseURL = base
	ind.APIKey = r.stringField("apiKey")

	if cats, ok := r.field("categories").([]any); ok {
		parts := make([]string, 0, len(cats))
		for _, c := range cats {
			if n, ok := c.(float64); ok {
				parts = append(parts, strconv.Itoa(int(n)))
			}
		}
		ind.Categories = strings.Join(parts, ",")
	}
	if ind.Categories == "" {
		ind.Categories = "7000,7020"
	}
	if ind.Priority <= 0 || ind.Priority > 50 {
		ind.Priority = 25
	}
	return ind, nil
}

// toArrResource maps a stored indexer into the Readarr v1 shape.
func toArrResource(ind *indexer.Indexer) arrIndexerResource {
	impl := "Newznab"
	if ind.Type == indexer.TypeTorznab {
		impl = "Torznab"
	}
	categories := []int{}
	for _, part := range strings.Split(ind.Categories, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			categories = append(categories, n)
		}
	}
	fields := []arrField{
		{Name: "baseUrl", Value: ind.BaseURL},
		{Name: "apiPath", Value: "/api"},
		{Name: "apiKey", Value: ind.APIKey},
		{Name: "categories", Value: categories},
		{Name: "additionalParameters", Value: ""},
	}
	if ind.Type == indexer.TypeTorznab {
		fields = append(fields, arrField{Name: "minimumSeeders", Value: 1})
	}
	return arrIndexerResource{
		ID:                      ind.ID,
		Name:                    ind.Name,
		Implementation:          impl,
		ImplementationName:      impl,
		ConfigContract:          impl + "Settings",
		Protocol:                ind.Protocol(),
		SupportsRss:             true,
		SupportsSearch:          true,
		EnableRss:               ind.Enabled,
		EnableAutomaticSearch:   ind.Enabled,
		EnableInteractiveSearch: ind.Enabled,
		Priority:                ind.Priority,
		Tags:                    []int{},
		Fields:                  fields,
	}
}

// mergedIndexerResource serves both consumers from one endpoint: LibriNode's
// UI reads the flat native keys, Prowlarr reads the arr keys.
func mergedIndexerResource(ind *indexer.Indexer) map[string]any {
	merged := map[string]any{}

	native, _ := json.Marshal(ind)
	_ = json.Unmarshal(native, &merged)

	arr, _ := json.Marshal(toArrResource(ind))
	arrMap := map[string]any{}
	_ = json.Unmarshal(arr, &arrMap)
	for k, v := range arrMap {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return merged
}

// handleIndexerSchema lists the indexer implementations Prowlarr may push.
func (s *server) handleIndexerSchema(w http.ResponseWriter, r *http.Request) {
	schema := []arrIndexerResource{
		{
			Name: "", Implementation: "Newznab", ImplementationName: "Newznab",
			ConfigContract: "NewznabSettings", Protocol: indexer.ProtocolUsenet,
			SupportsRss: true, SupportsSearch: true, Tags: []int{},
			Fields: []arrField{
				{Name: "baseUrl"}, {Name: "apiPath", Value: "/api"}, {Name: "apiKey"},
				{Name: "categories", Value: []int{7000, 7020}}, {Name: "additionalParameters"},
			},
		},
		{
			Name: "", Implementation: "Torznab", ImplementationName: "Torznab",
			ConfigContract: "TorznabSettings", Protocol: indexer.ProtocolTorrent,
			SupportsRss: true, SupportsSearch: true, Tags: []int{},
			Fields: []arrField{
				{Name: "baseUrl"}, {Name: "apiPath", Value: "/api"}, {Name: "apiKey"},
				{Name: "categories", Value: []int{7000, 7020}}, {Name: "additionalParameters"},
				{Name: "minimumSeeders", Value: 1},
			},
		},
	}
	writeJSON(w, http.StatusOK, schema)
}

// handleListTags exists because *arr clients resolve tags during sync;
// LibriNode has no tag system yet.
func (s *server) handleListTags(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{})
}
