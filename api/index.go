package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/requester"
	"github.com/felipemarinho97/torrent-indexer/schema"
	meilisearch "github.com/felipemarinho97/torrent-indexer/search"
)

type Indexer struct {
	redis     *cache.Redis
	metrics   *monitoring.Metrics
	requester *requester.Requster
	search    *meilisearch.SearchIndexer
}

type IndexerMeta struct {
	URL       string
	SearchURL string
}

type Response struct {
	Results []schema.IndexedTorrent `json:"results"`
	Count   int                     `json:"count"`
}

func NewIndexers(redis *cache.Redis, metrics *monitoring.Metrics, req *requester.Requster, si *meilisearch.SearchIndexer) *Indexer {
	return &Indexer{
		redis:     redis,
		metrics:   metrics,
		requester: req,
		search:    si,
	}
}

func HandlerIndex(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC850)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err := json.NewEncoder(w).Encode(map[string]interface{}{
		"time": currentTime,
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
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
