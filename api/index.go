package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
)

type Indexer struct {
	redis *cache.Redis
}

func NewIndexers(redis *cache.Redis) *Indexer {
	return &Indexer{
		redis: redis,
	}
}

func HandlerIndex(w http.ResponseWriter, r *http.Request) {
	currentTime := time.Now().Format(time.RFC850)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"time": currentTime,
		"endpoints": map[string]interface{}{
			"/indexers/comando_torrents": map[string]interface{}{
				"method":      "GET",
				"description": "Indexer for comando torrents",
				"query_params": map[string]string{
					"q": "search query",
				},
			},
		},
	})
}
