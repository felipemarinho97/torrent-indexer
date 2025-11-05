package handler

import (
	"encoding/json"
	"net/http"
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
	Results []schema.IndexedTorrent `json:"results"`
	Count   int                     `json:"count"`
}

type PostProcessorFunc func(*Indexer, *http.Request, []schema.IndexedTorrent) []schema.IndexedTorrent

var GlobalPostProcessors = []PostProcessorFunc{
	AddSimilarityCheck,     // Jaccard similarity
	FullfilMissingMetadata, // Fill missing size or title metadata
	CleanupTitleWebsites,   // Remove website names from titles
	FallbackPostTitle,      // Fallback to original title if empty
	AppendAudioTags,        // Add (brazilian, eng, etc.) audio tags to titles
	SendToSearchIndexer,    // Send indexed torrents to Meilisearch
}

type IndexersConfig struct {
	FallbackTitleEnabled bool
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

func HandlerIndex(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC850)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"time":  currentTime,
		"build": consts.GetBuildInfo(),
		"endpoints": map[string]interface{}{
			"/indexers/comando_torrents": []map[string]interface{}{
				{
					"method":      "GET",
					"description": "Indexer for comando torrents",
					"query_params": map[string]string{
						"q":              "search query",
						"page":           "page number",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					},
				},
			},
			"/indexers/bludv": []map[string]interface{}{
				{
					"method":      "GET",
					"description": "Indexer for bludv",
					"query_params": map[string]string{
						"q":              "search query",
						"page":           "page number",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					}},
			},
			"/indexers/torrent-dos-filmes": []map[string]interface{}{
				{
					"method":      "GET",
					"page":        "page number",
					"description": "Indexer for Torrent dos Filmes",
					"query_params": map[string]string{
						"q":              "search query",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					},
				},
			},
			"/indexers/comandohds": []map[string]interface{}{
				{
					"method":      "GET",
					"page":        "page number",
					"description": "Indexer for Comando HDs",
					"query_params": map[string]string{
						"q":              "search query",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					},
				},
			},
			"/indexers/starck-filmes": []map[string]interface{}{
				{
					"method":      "GET",
					"page":        "page number",
					"description": "Indexer for Starck Filmes",
					"query_params": map[string]string{
						"q":              "search query",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					},
				},
			},
			"/indexers/rede_torrent": []map[string]interface{}{
				{
					"method":      "GET",
					"description": "Indexer for rede torrent",
					"query_params": map[string]string{
						"q":              "search query",
						"page":           "page number",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					},
				},
			},
			"/indexers/vaca_torrent": []map[string]interface{}{
				{
					"method":      "GET",
					"description": "Indexer for Vaca Torrent",
					"query_params": map[string]string{
						"q":              "search query",
						"page":           "page number",
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					},
				},
			},
			"/indexers/manual": []map[string]interface{}{
				{
					"method":      "POST",
					"description": "Add a manual torrent entry to the indexer for 12 hours",
					"body": map[string]interface{}{
						"magnetLink": "magnet link",
					}},
				{
					"method":      "GET",
					"description": "Get all manual torrents",
				},
			},
			"/search": []map[string]interface{}{
				{
					"method":      "GET",
					"description": "Search for cached torrents across all indexers",
					"query_params": map[string]string{
						"q": "search query",
					},
				},
			},
			"/ui/": []map[string]interface{}{
				{
					"method":      "GET",
					"description": "Show the unified search UI (only work if Meilisearch is enabled)",
				},
			},
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
