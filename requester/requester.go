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
	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/utils"
	"github.com/fereidani/httpdecompressor"
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
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DisableCompression:  false,
			MaxIdleConns:        100,              // Increase connection pool
			MaxIdleConnsPerHost: 10,               // More connections per host
			IdleConnTimeout:     90 * time.Second, // Keep connections alive longer
			DisableKeepAlives:   false,            // Enable keep-alive
			ForceAttemptHTTP2:   true,             // Use HTTP/2 when possible
		},
	}

	return &Requster{fs: fs, httpClient: httpClient, c: c, shortLivedCacheExpiration: 30 * time.Minute}
}

func (i *Requster) SetShortLivedCacheExpiration(expiration time.Duration) {
	i.shortLivedCacheExpiration = expiration
}

// spoofBrowserHeaders adds browser-like headers to spoof a real browser.
// If referer is empty, it defaults to "https://google.com/"
func spoofBrowserHeaders(req *http.Request, referer string) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", httpdecompressor.ACCEPT_ENCODING)

	// Use provided referer or default to Google
	if referer != "" {
		req.Header.Set("Referer", referer)
	} else {
		req.Header.Set("Referer", "https://google.com/")
	}

	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")
}

func (i *Requster) GetDocument(ctx context.Context, url string, referer ...string) (io.ReadCloser, error) {
	var body io.ReadCloser

	// Extract referer if provided
	ref := ""
	if len(referer) > 0 {
		ref = referer[0]
	}

	// try request from short-lived cache
	key := fmt.Sprintf("%s:%s", cacheKey, url)
	bodyByte, err := i.c.Get(ctx, key)
	if err == nil {
		logging.Debug().Str("url", url).Msg("Returning from short-lived cache")
		body = io.NopCloser(bytes.NewReader(bodyByte))
		return body, nil
	}

	// try request with plain client
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for url %s: %w", url, err)
	}

	// Add browser-like headers to spoof a real browser
	spoofBrowserHeaders(req, ref)

	resp, err := i.httpClient.Do(req)
	if err != nil {
		// try request with flare solverr
		body, err = i.fs.Get(url, 3)
		if err != nil {
			return nil, fmt.Errorf("failed to do request for url %s: %w", url, err)
		}
	} else {
		defer resp.Body.Close()

		// Decompress response using httpdecompressor
		body, err = httpdecompressor.Reader(resp)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress response: %w", err)
		}
		defer body.Close()

		encoding := resp.Header.Get("Content-Encoding")
		if encoding != "" {
			logging.Debug().Str("encoding", encoding).Msg("Decompressing response")
		}
	}

	// Pre-allocate buffer based on Content-Length if available
	var buf bytes.Buffer
	if resp != nil && resp.ContentLength > 0 {
		buf.Grow(int(resp.ContentLength))
	} else {
		buf.Grow(32 * 1024) // Default 32KB pre-allocation
	}

	// Use io.Copy instead of io.ReadAll for better performance
	_, err = io.Copy(&buf, body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	bodyByte = buf.Bytes()
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
		logging.Debug().Str("url", url).Msg("Request served from flaresolverr")
	} else {
		logging.Debug().Str("url", url).Msg("Request served from plain client")
	}

	// save response to cache if it's not a challange, body is not empty and is valid HTML
	if !hasChallange(bodyByte) && len(bodyByte) > 0 && utils.IsValidHTML(string(bodyByte)) {
		err = i.c.SetWithExpiration(ctx, key, bodyByte, i.shortLivedCacheExpiration)
		if err != nil {
			logging.Error().Err(err).Str("url", url).Msg("Failed to save response to cache")
		}
		logging.Debug().Str("url", url).Msg("Saved to cache")
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
