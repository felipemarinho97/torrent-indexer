package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

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
func (h *MeilisearchHandler) SearchTorrentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 10 // Default limit
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			http.Error(w, "Invalid limit parameter", http.StatusBadRequest)
			return
		}
	}

	results, err := h.Module.SearchTorrent(query, limit)
	if err != nil {
		http.Error(w, "Failed to search torrents", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
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
