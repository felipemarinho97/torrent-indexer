package magnet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/logging"
)

type MetadataRequest struct {
	MagnetURI string `json:"magnet_uri"`
}

type TorrentFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	Offset int64  `json:"offset"`
}

type MetadataResponse struct {
	InfoHash    string        `json:"info_hash"`
	Name        string        `json:"name"`
	Size        int64         `json:"size"`
	Files       []TorrentFile `json:"files"`
	CreatedBy   string        `json:"created_by"`
	CreatedAt   time.Time     `json:"created_at"`
	Comment     string        `json:"comment"`
	Trackers    []string      `json:"trackers"`
	DownloadURL string        `json:"download_url"`
}

type MetadataClient struct {
	baseURL    string
	httpClient *http.Client
	c          *cache.Redis
}

func NewClient(baseURL string, timeout time.Duration, c *cache.Redis) *MetadataClient {
	return &MetadataClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:      100,
				IdleConnTimeout:   30 * time.Second,
				ForceAttemptHTTP2: true,
			},
		},
		c: c,
	}
}

func (c *MetadataClient) IsEnabled() bool {
	return c != nil && c.baseURL != ""
}

func (c *MetadataClient) FetchMetadata(ctx context.Context, magnetURI string) (*MetadataResponse, error) {
	if !c.IsEnabled() {
		return nil, fmt.Errorf("magnet metadata API is not enabled")
	}
	// Check cache first
	m, err := ParseMagnetUri(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse magnet URI: %w", err)
	}
	cacheKey := fmt.Sprintf("metadata:%s", m.InfoHash)
	cachedData, err := c.c.Get(ctx, cacheKey)
	if err == nil && cachedData != nil {
		var cachedMetadata MetadataResponse
		if err := json.Unmarshal(cachedData, &cachedMetadata); err == nil {
			return &cachedMetadata, nil
		}
	}

	reqBody := MetadataRequest{MagnetURI: magnetURI}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/metadata", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	logging.Debug().Str("info_hash", fmt.Sprint(m.InfoHash)).Msg("Fetching metadata from MAGNET_METADATA_API")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send POST request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API responded with status: %s", resp.Status)
	}

	var metadata MetadataResponse
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Cache the metadata response
	cacheData, err := json.Marshal(metadata)
	if err == nil {
		_ = c.c.SetWithExpiration(ctx, cacheKey, cacheData, 7*24*time.Hour)
	}

	return &metadata, nil
}
