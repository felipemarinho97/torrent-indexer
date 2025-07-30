package requester

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

const (
	cacheKey = "shortLivedCache"
)

var challangeRegex = regexp.MustCompile(`(?i)(just a moment|cf-chl-bypass|under attack)`)

type Requster struct {
	fs                        *FlareSolverr
	c                         *cache.Redis
	httpClient                *http.Client
	shortLivedCacheExpiration time.Duration
}

func NewRequester(fs *FlareSolverr, c *cache.Redis) *Requster {
	return &Requster{fs: fs, httpClient: &http.Client{}, c: c, shortLivedCacheExpiration: 30 * time.Minute}
}

func (i *Requster) SetShortLivedCacheExpiration(expiration time.Duration) {
	i.shortLivedCacheExpiration = expiration
}

func (i *Requster) GetDocument(ctx context.Context, url string) (io.ReadCloser, error) {
	var body io.ReadCloser

	// try request from short-lived cache
	key := fmt.Sprintf("%s:%s", cacheKey, url)
	bodyByte, err := i.c.Get(ctx, key)
	if err == nil {
		fmt.Printf("returning from short-lived cache: %s\n", url)
		body = io.NopCloser(bytes.NewReader(bodyByte))
		return body, nil
	}

	// try request with plain client
	resp, err := i.httpClient.Get(url)
	if err != nil {
		// try request with flare solverr
		body, err = i.fs.Get(url, 3)
		if err != nil {
			return nil, fmt.Errorf("failed to do request for url %s: %w", url, err)
		}
	} else {
		defer resp.Body.Close()
		body = resp.Body
	}

	bodyByte, err = io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if hasChallange(bodyByte) {
		// try request with flare solverr
		body, err = i.fs.Get(url, 3)
		if err != nil {
			return nil, fmt.Errorf("failed to do request for url %s: %w", url, err)
		}
		bodyByte, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		fmt.Printf("request served from flaresolverr: %s\n", url)
	} else {
		fmt.Printf("request served from plain client: %s\n", url)
	}

	// save response to cache if it's not a challange, body is not empty and is valid HTML
	if !hasChallange(bodyByte) && len(bodyByte) > 0 && utils.IsValidHTML(string(bodyByte)) {
		err = i.c.SetWithExpiration(ctx, key, bodyByte, i.shortLivedCacheExpiration)
		if err != nil {
			fmt.Printf("failed to save response to cache: %v\n", err)
		}
		fmt.Printf("saved to cache: %s\n", url)
	} else {
		return nil, fmt.Errorf("response is a challange")
	}

	return io.NopCloser(bytes.NewReader(bodyByte)), nil
}

func (i *Requster) ExpireDocument(ctx context.Context, url string) error {
	key := fmt.Sprintf("%s:%s", cacheKey, url)
	return i.c.Del(ctx, key)
}

// hasChallange checks if the body contains a challange by regex matching
func hasChallange(body []byte) bool {
	return challangeRegex.Match(body)
}
