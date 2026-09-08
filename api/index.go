package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/consts"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/requester"
	"github.com/felipemarinho97/torrent-indexer/schema"
	meilisearch "github.com/felipemarinho97/torrent-indexer/search"
)

type Indexer struct {
	config            IndexersConfig
	redis             *cache.Redis
	metrics           *monitoring.Metrics
	requester         *requester.Requster
	search            *meilisearch.SearchIndexer
	magnetMetadataAPI *magnet.MetadataClient
	postProcessors    []PostProcessorFunc
}

type IndexerMeta struct {
	Label       string // Label is used for Prometheus metrics and logging. Must be alphanumeric optionally with underscores.
	URL         string // URL is the base URL of the indexer, e.g. "https://example.com/"
	SearchURL   string // SearchURL is the base URL for search queries, e.g. "?s="
	PagePattern string // PagePattern for pagination, e.g. "page/%s"
}

type Response struct {
	Results      []schema.IndexedTorrent `json:"results"`
	Count        int                     `json:"count"`
	IndexedCount int                     `json:"indexed_count,omitempty"`
}

type PostProcessorFunc func(*Indexer, *http.Request, []schema.IndexedTorrent) []schema.IndexedTorrent

var GlobalPostProcessors = []PostProcessorFunc{
	AddSimilarityCheck,     // Jaccard similarity
	FullfilMissingMetadata, // Fill missing size or title metadata
	ParseTitleMetadata,     // Parse S/E, Resolution, Quality from titles
	CleanupTitleWebsites,   // Remove website names from titles
	FallbackPostTitle,      // Fallback to original title if empty
	AppendAudioTags,        // Add (brazilian, eng, etc.) audio tags to titles
	ApplySorting,           // Sort results based on sortBy and sortDirection params
	StripExtendedMetadata,  // Strip metadata if ENABLE_EXTENDED_METADATA is not true
	SendToSearchIndexer,    // Send indexed torrents to Meilisearch
	FilterBy,               // Filter results based on query params (audio, etc.)
	ApplyLimit,             // Limit number of results based on query param
}

type IndexersConfig struct {
	FallbackTitleEnabled bool
	DisabledIndexers     map[string]bool
}

func NewIndexers(
	config IndexersConfig,
	redis *cache.Redis,
	metrics *monitoring.Metrics,
	req *requester.Requster,
	si *meilisearch.SearchIndexer,
	mc *magnet.MetadataClient,
) *Indexer {
	return &Indexer{
		config:            config,
		redis:             redis,
		metrics:           metrics,
		requester:         req,
		search:            si,
		magnetMetadataAPI: mc,
		postProcessors:    GlobalPostProcessors,
	}
}

func (i *Indexer) GetAllHandlersMap() map[string]http.HandlerFunc {
	allHandlers := map[string]http.HandlerFunc{
		"bludv":              i.HandlerBluDVIndexer,
		"comando_torrents":   i.HandlerComandoIndexer,
		"rede_torrent":       i.HandlerRedeTorrentIndexer,
		"starck-filmes":      i.HandlerStarckFilmesIndexer,
		"torrent-dos-filmes": i.HandlerTorrentDosFilmesIndexer,
		"vaca_torrent":       i.HandlerVacaTorrentIndexer,
	}

	filteredHandlers := make(map[string]http.HandlerFunc)
	for name, handler := range allHandlers {
		if !i.config.DisabledIndexers[name] {
			filteredHandlers[name] = handler
		}
	}
	return filteredHandlers
}

func (i *Indexer) GetIndexerNames() []string {
	var names []string
	for name := range i.GetAllHandlersMap() {
		names = append(names, name)
	}
	return names
}

func (i *Indexer) HandlerIndex(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC850)

	commonQueryParams := map[string]string{
		"q":              "search query",
		"page":           "page number",
		"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
		"limit":          "maximum number of results to return",
		"sortBy":         "sort by field (title, original_title, year, date, seed_count, leech_count, size, similarity)",
		"sortDirection":  "sort direction (asc or desc, default: desc)",
		"audio":          "filter by audio languages (comma separated, e.g. por,eng,brazilian)",
		"year":           "filter by year (e.g. 2020)",
		"imdb":           "filter by imdb ID (e.g. tt1234567) - this ONLY FILTERTS results, for searching by IMDB ID use the \"q\" parameter",
	}

	// Define structs for ordered JSON output
	type EndpointDetail struct {
		Method      string                 `json:"method"`
		Description string                 `json:"description"`
		QueryParams map[string]string      `json:"query_params,omitempty"`
		Body        map[string]interface{} `json:"body,omitempty"`
	}

	type Endpoints struct {
		IndexerGeneric []EndpointDetail `json:"/indexers/{indexer_name}"`
		Manual         []EndpointDetail `json:"/indexers/manual"`
		Search         []EndpointDetail `json:"/search"`
		UI             []EndpointDetail `json:"/ui/"`
	}

	type RootResponse struct {
		Time         string      `json:"time"`
		Build        interface{} `json:"build"`
		IndexerNames []string    `json:"indexer_names"`
		Endpoints    Endpoints   `json:"endpoints"`
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(RootResponse{
		Time:  currentTime,
		Build:        consts.GetBuildInfo(),
		IndexerNames: i.GetIndexerNames(),
		Endpoints: Endpoints{
			IndexerGeneric: []EndpointDetail{
				{
					Method:      "GET",
					Description: "Indexer endpoint for the specified indexer. Supports multiple queries (e.g. ?q=q1&q=q2)",
					QueryParams: commonQueryParams,
				},
				{
					Method:      "GET",
					Description: "Search across multiple specific indexers concurrently. Supports multiple queries.",
					QueryParams: map[string]string{
						"indexers": "comma-separated list of indexers (e.g. bludv,vaca_torrent)",
						"q":        "search query (supports multiple)",
					},
				},
				{
					Method:      "GET",
					Description: "Search across ALL indexers concurrently. Supports multiple queries.",
					QueryParams: commonQueryParams,
				},
			},
			Manual: []EndpointDetail{
				{
					Method:      "POST",
					Description: "Add a manual torrent entry to the indexer for 12 hours",
					Body: map[string]interface{}{
						"magnetLink": "magnet link",
					},
				},
				{
					Method:      "GET",
					Description: "Get all manual torrents",
				},
			},
			Search: []EndpointDetail{
				{
					Method:      "GET/POST",
					Description: "Search for cached torrents across all indexers (Meilisearch). Supports multiple queries via GET ?q=a&q=b or POST {'queries':['a','b']}",
					QueryParams: map[string]string{
						"q":     "search query (can pass multiple for multi-search)",
						"limit": "maximum number of results to return (default: 10)",
					},
					Body: map[string]interface{}{
						"queries": "array of string queries",
						"limit":   "maximum results per query",
					},
				},
				{
					Method:      "GET",
					Description: "Get statistics about the Meilisearch index",
				},
			},
			UI: []EndpointDetail{
				{
					Method:      "GET",
					Description: "Show the unified search UI (only work if Meilisearch is enabled)",
				},
			},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (i *Indexer) getIndexerHandlers(names []string) []http.HandlerFunc {
	allHandlers := i.GetAllHandlersMap()

	if len(names) == 0 {
		return nil
	}

	var handlers []http.HandlerFunc
	for _, name := range names {
		if name == "all" {
			handlers = make([]http.HandlerFunc, 0, len(allHandlers))
			for _, h := range allHandlers {
				handlers = append(handlers, h)
			}
			return handlers
		}
		if h, ok := allHandlers[name]; ok {
			handlers = append(handlers, h)
		}
	}
	return handlers
}

func (i *Indexer) HandlerMultiIndexers(w http.ResponseWriter, r *http.Request) {
	var names []string
	if r.URL.Path == "/indexers/all" {
		names = []string{"all"}
	} else {
		namesParam := r.URL.Query().Get("indexers")
		if namesParam != "" {
			importStrings := strings.Split(namesParam, ",")
			for _, name := range importStrings {
				names = append(names, strings.TrimSpace(name))
			}
		}
	}

	handlers := i.getIndexerHandlers(names)
	if len(handlers) == 0 {
		http.Error(w, "No valid indexers specified", http.StatusBadRequest)
		return
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	allResults := []schema.IndexedTorrent{}
	totalIndexed := 0

	for _, h := range handlers {
		wg.Add(1)
		go func(handlerFunc http.HandlerFunc) {
			defer wg.Done()
			rec := httptest.NewRecorder()
			handlerFunc(rec, r)

			if rec.Code == http.StatusOK {
				var res Response
				if err := json.Unmarshal(rec.Body.Bytes(), &res); err == nil {
					mu.Lock()
					allResults = append(allResults, res.Results...)
					totalIndexed += res.IndexedCount
					mu.Unlock()
				}
			}
		}(h)
	}

	wg.Wait()

	response := Response{
		Results:      allResults,
		Count:        len(allResults),
		IndexedCount: totalIndexed,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// WithMultiQuery wraps an http.HandlerFunc to support multiple 'q' query parameters.
// If multiple 'q' parameters are present, it executes the handler concurrently for each query
// and merges the results into a single response.
func WithMultiQuery(handlerFunc http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queries := r.URL.Query()["q"]
		if len(queries) <= 1 {
			// Standard execution if 0 or 1 query
			handlerFunc(w, r)
			return
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		allResults := []schema.IndexedTorrent{}
		totalIndexed := 0
		status := http.StatusOK

		for _, q := range queries {
			wg.Add(1)
			go func(query string) {
				defer wg.Done()

				// Create a new request with just this one 'q'
				newURL := *r.URL
				newQuery := newURL.Query()
				newQuery.Set("q", query)
				newURL.RawQuery = newQuery.Encode()

				newReq := r.Clone(r.Context())
				newReq.URL = &newURL

				rec := httptest.NewRecorder()
				handlerFunc(rec, newReq)

				if rec.Code == http.StatusOK {
					var res Response
					if err := json.Unmarshal(rec.Body.Bytes(), &res); err == nil {
						mu.Lock()
						allResults = append(allResults, res.Results...)
						totalIndexed += res.IndexedCount
						mu.Unlock()
					}
				} else {
					mu.Lock()
					status = rec.Code
					mu.Unlock()
				}
			}(q)
		}

		wg.Wait()

		response := Response{
			Results:      allResults,
			Count:        len(allResults),
			IndexedCount: totalIndexed,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(response)
	}
}
