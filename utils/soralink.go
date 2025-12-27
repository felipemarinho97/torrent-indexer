package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/logging"
)

// SoraLinkFetcher handles fetching links from SoraLink-protected pages
type SoraLinkFetcher struct {
	client  *http.Client
	baseURL string
	cache   *cache.Redis
}

// SoraLinkResult contains the extracted link or error
type SoraLinkResult struct {
	Link  string
	Error error
}

// NewSoraLinkFetcher creates a new SoraLink fetcher with a persistent cookie jar
func NewSoraLinkFetcher(baseURL string, cache *cache.Redis) (*SoraLinkFetcher, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &SoraLinkFetcher{
		client:  client,
		baseURL: baseURL,
		cache:   cache,
	}, nil
}

// FetchLink extracts the protected link from a SoraLink-protected page
func (s *SoraLinkFetcher) FetchLink(ctx context.Context, queryID string) (string, error) {
	key := fmt.Sprintf("soralink:%s", queryID)
	// try to get from cache
	cachedLink, err := s.cache.Get(ctx, key)
	if err == nil {
		logging.Debug().Str("queryID", queryID[:30]+"...").Msg("Returning SoraLink from cache")
		return string(cachedLink), nil
	}

	logging.Debug().Str("baseURL", s.baseURL).Str("queryID", queryID[:30]+"...").Msg("Fetching SoraLink page")

	pageURL := fmt.Sprintf("%s?id=%s", s.baseURL, queryID)
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create GET request: %w", err)
	}

	req.Header.Set("User-Agent", SpoofedUserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	bodyText := string(bodyBytes)

	// Step 2: Extract token using regex
	tokenRegex := regexp.MustCompile(`"token":"(.*?)"`)
	tokenMatch := tokenRegex.FindStringSubmatch(bodyText)
	if len(tokenMatch) < 2 {
		return "", fmt.Errorf("failed to extract token from page")
	}

	// Unescape JSON slashes (\/ -> /)
	token := strings.ReplaceAll(tokenMatch[1], `\/`, `/`)
	logging.Debug().Str("token", token[:30]+"...").Msg("Extracted token")

	// Step 3: Extract action code (soralink_z)
	actionRegex := regexp.MustCompile(`"soralink_z":"(.*?)"`)
	actionMatch := actionRegex.FindStringSubmatch(bodyText)

	action := "" // Default fallback
	if len(actionMatch) >= 2 {
		action = actionMatch[1]
	} else {
		logging.Warn().Msg("Could not find dynamic action code, using default")
	}
	logging.Debug().Str("action", action).Msg("Extracted action code")

	// Step 4: POST request to ajax endpoint
	ajaxURL := s.baseURL + "/wp-admin/admin-ajax.php"
	logging.Debug().Str("ajaxURL", ajaxURL).Msg("Sending POST request")

	formData := url.Values{}
	formData.Set("token", token)
	formData.Set("action", action)

	postReq, err := http.NewRequestWithContext(ctx, "POST", ajaxURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create POST request: %w", err)
	}

	postReq.Header.Set("User-Agent", SpoofedUserAgent)
	postReq.Header.Set("Accept", "*/*")
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	postResp, err := s.client.Do(postReq)
	if err != nil {
		return "", fmt.Errorf("failed to send POST request: %w", err)
	}
	defer postResp.Body.Close()

	// Get the magnet link from the Location header
	location := postResp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no Location header found in response")
	}

	// Validate it's a magnet link
	if !strings.HasPrefix(location, "magnet:") {
		return "", fmt.Errorf("location header is not a magnet link: %s", location)
	}

	logging.Debug().Str("magnet", location[:50]+"...").Msg("Extracted magnet link from Location header")

	// cache the result
	_ = s.cache.Set(ctx, key, []byte(location))

	return location, nil
}
