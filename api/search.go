package handler

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	edlib "github.com/hbollon/go-edlib"
	"github.com/felipemarinho97/torrent-indexer/schema"
	meilisearch "github.com/felipemarinho97/torrent-indexer/search"
)

// MeilisearchHandler handles HTTP requests for Meilisearch integration.
type MeilisearchHandler struct {
	Module *meilisearch.SearchIndexer
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Service   string                 `json:"service"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp string                 `json:"timestamp"`
}

// StatsResponse represents the stats endpoint response
type StatsResponse struct {
	Status            string           `json:"status"`
	NumberOfDocuments int64            `json:"numberOfDocuments"`
	IsIndexing        bool             `json:"isIndexing"`
	FieldDistribution map[string]int64 `json:"fieldDistribution"`
	Service           string           `json:"service"`
}

// NewMeilisearchHandler creates a new instance of MeilisearchHandler.
func NewMeilisearchHandler(module *meilisearch.SearchIndexer) *MeilisearchHandler {
	return &MeilisearchHandler{Module: module}
}

// SearchTorrentHandler handles the searching of torrent items.
// Supports multiple queries via GET ?q=a&q=b (each limited to perQueryLimit).
func (h *MeilisearchHandler) SearchTorrentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	queries := r.URL.Query()["q"]
	if len(queries) == 0 || (len(queries) == 1 && queries[0] == "") {
		queries = []string{time.Now().Format("2006-01-02")} // needed for prowlar/jackett empty queries
	}

	limitStr := r.URL.Query().Get("limit")
	perQueryLimit := 100 // Default limit per query
	if limitStr != "" {
		var err error
		perQueryLimit, err = strconv.Atoi(limitStr)
		if err != nil || perQueryLimit <= 0 {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
	}

	// Cap at 100 per query to prevent abuse
	if perQueryLimit > 100 {
		perQueryLimit = 100
	}

	// Search each query with the per-query limit, then merge + deduplicate
	seen := make(map[string]bool)
	var allResults []schema.IndexedTorrent

	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}

		results, err := h.Module.SearchTorrent(query, perQueryLimit)
		if err != nil {
			continue
		}

		// Calculate similarity scores against this specific query
		qLower := strings.ToLower(query)
		for i := range results {
			tLower := strings.ToLower(results[i].Title)
			sim := edlib.JaccardSimilarity(tLower, qLower, 2)
			// Keep the highest similarity if we see the same torrent from multiple queries
			if results[i].Similarity < sim {
				results[i].Similarity = sim
			}
		}

		// Deduplicate by info_hash
		for _, r := range results {
			key := r.InfoHash
			if key == "" {
				key = r.MagnetLink
			}
			if key != "" && seen[key] {
				continue
			}
			if key != "" {
				seen[key] = true
			}
			allResults = append(allResults, r)
		}
	}

	// Sort all merged results by highest similarity first
	slices.SortFunc(allResults, func(a, b schema.IndexedTorrent) int {
		return int((b.Similarity - a.Similarity) * 1000000)
	})

	// Format response to match indexers structure
	response := map[string]interface{}{
		"results": allResults,
		"count":   len(allResults),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HealthHandler provides a health check endpoint for Meilisearch.
func (h *MeilisearchHandler) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Check if Meilisearch is healthy
	isHealthy := h.Module.IsHealthy()

	response := HealthResponse{
		Service:   "meilisearch",
		Timestamp: getCurrentTimestamp(),
	}

	if isHealthy {
		// Try to get additional stats for more detailed health info
		stats, err := h.Module.GetStats()
		if err == nil {
			response.Status = "healthy"
			response.Details = map[string]interface{}{
				"documents": stats.NumberOfDocuments,
				"indexing":  stats.IsIndexing,
			}
			w.WriteHeader(http.StatusOK)
		} else {
			// Service is up but can't get stats
			response.Status = "degraded"
			response.Details = map[string]interface{}{
				"error": "Could not retrieve stats",
			}
			w.WriteHeader(http.StatusOK)
		}
	} else {
		// Service is down
		response.Status = "unhealthy"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// StatsHandler provides detailed statistics about the Meilisearch index.
func (h *MeilisearchHandler) StatsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Get detailed stats from Meilisearch
	stats, err := h.Module.GetStats()
	if err != nil {
		// Check if it's a connectivity issue
		if !h.Module.IsHealthy() {
			http.Error(w, "Meilisearch service is unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Failed to retrieve statistics", http.StatusInternalServerError)
		return
	}

	response := StatsResponse{
		Status:            "healthy",
		Service:           "meilisearch",
		NumberOfDocuments: stats.NumberOfDocuments,
		IsIndexing:        stats.IsIndexing,
		FieldDistribution: stats.FieldDistribution,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// getCurrentTimestamp returns the current timestamp in RFC3339 format
func getCurrentTimestamp() string {
	return time.Now().Format(time.RFC3339)
}
