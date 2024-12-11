package meilisearch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/felipemarinho97/torrent-indexer/schema"
)

// SearchIndexer integrates with Meilisearch to index and search torrent items.
type SearchIndexer struct {
	Client    *http.Client
	BaseURL   string
	APIKey    string
	IndexName string
}

// NewSearchIndexer creates a new instance of SearchIndexer.
func NewSearchIndexer(baseURL, apiKey, indexName string) *SearchIndexer {
	return &SearchIndexer{
		Client:    &http.Client{Timeout: 10 * time.Second},
		BaseURL:   baseURL,
		APIKey:    apiKey,
		IndexName: indexName,
	}
}

// IndexTorrent indexes a single torrent item in Meilisearch.
func (t *SearchIndexer) IndexTorrent(torrent schema.IndexedTorrent) error {
	url := fmt.Sprintf("%s/indexes/%s/documents", t.BaseURL, t.IndexName)

	torrentWithKey := struct {
		Hash string `json:"id"`
		schema.IndexedTorrent
	}{
		Hash:           torrent.InfoHash,
		IndexedTorrent: torrent,
	}

	jsonData, err := json.Marshal(torrentWithKey)
	if err != nil {
		return fmt.Errorf("failed to marshal torrent data: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.APIKey))
	}

	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

func (t *SearchIndexer) IndexTorrents(torrents []schema.IndexedTorrent) error {
	url := fmt.Sprintf("%s/indexes/%s/documents", t.BaseURL, t.IndexName)

	torrentsWithKey := make([]struct {
		Hash string `json:"id"`
		schema.IndexedTorrent
	}, 0, len(torrents))
	for _, torrent := range torrents {
		torrentWithKey := struct {
			Hash string `json:"id"`
			schema.IndexedTorrent
		}{
			Hash:           torrent.InfoHash,
			IndexedTorrent: torrent,
		}
		torrentsWithKey = append(torrentsWithKey, torrentWithKey)
	}

	jsonData, err := json.Marshal(torrentsWithKey)
	if err != nil {
		return fmt.Errorf("failed to marshal torrent data: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.APIKey))
	}

	resp, err := t.Client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// SearchTorrent searches indexed torrents in Meilisearch based on the query.
func (t *SearchIndexer) SearchTorrent(query string, limit int) ([]schema.IndexedTorrent, error) {
	url := fmt.Sprintf("%s/indexes/%s/search", t.BaseURL, t.IndexName)
	requestBody := map[string]string{
		"q": query,
	}
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search query: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", t.APIKey))
	}

	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search failed: %s", body)
	}

	var result struct {
		Hits []schema.IndexedTorrent `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}

	return result.Hits, nil
}
