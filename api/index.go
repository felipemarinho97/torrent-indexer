package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/requester"
	"github.com/felipemarinho97/torrent-indexer/schema"
)

type Indexer struct {
	redis     *cache.Redis
	metrics   *monitoring.Metrics
	requester *requester.Requster
}

type IndexerMeta struct {
	URL       string
	SearchURL string
}

type Response struct {
	Results []IndexedTorrent `json:"results"`
	Count   int              `json:"count"`
}

type IndexedTorrent struct {
	Title         string         `json:"title"`
	OriginalTitle string         `json:"original_title"`
	Details       string         `json:"details"`
	Year          string         `json:"year"`
	IMDB          string         `json:"imdb"`
	Audio         []schema.Audio `json:"audio"`
	MagnetLink    string         `json:"magnet_link"`
	Date          time.Time      `json:"date"`
	InfoHash      string         `json:"info_hash"`
	Trackers      []string       `json:"trackers"`
	Size          string         `json:"size"`
	LeechCount    int            `json:"leech_count"`
	SeedCount     int            `json:"seed_count"`
	Similarity    float32        `json:"similarity"`
}

func NewIndexers(redis *cache.Redis, metrics *monitoring.Metrics, req *requester.Requster) *Indexer {
	return &Indexer{
		redis:     redis,
		metrics:   metrics,
		requester: req,
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
						"filter_results": "if results with similarity equals to zero should be filtered (true/false)",
					}},
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
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
