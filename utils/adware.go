package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/felipemarinho97/torrent-indexer/logging"
)

// magnetRegex matches a magnet URI up to the first quote, backslash, whitespace
// or angle bracket, so it can be pulled out of markup or from a JS string literal.
var magnetRegex = regexp.MustCompile(`(?i)magnet:\?xt=urn:btih:[^"'\\[:space:]<>]+`)

// AdwareCache is the slice of the cache used by the resolver, kept as an
// interface so it can be faked in tests. *cache.Redis satisfies it.
type AdwareCache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	SetWithExpiration(ctx context.Context, key string, value []byte, expiration time.Duration) error
}

type AdwareResolver struct {
	client        *http.Client
	cache         AdwareCache
	adwareDomains []string
}

func NewAdwareResolver(cache AdwareCache, domains string) *AdwareResolver {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Trim whitespace from each domain and drop the empty ones, so an empty or
	// trailing-comma ADWARE_DOMAINS does not turn into an `a[href^=""]` selector
	// that matches every anchor on the page.
	var splitDomains []string
	for domain := range strings.SplitSeq(domains, ",") {
		if domain = strings.TrimSpace(domain); domain != "" {
			splitDomains = append(splitDomains, domain)
		}
	}

	return &AdwareResolver{
		client:        client,
		cache:         cache,
		adwareDomains: splitDomains,
	}
}

func (r *AdwareResolver) ExtractAdwareLinks(pageDom *goquery.Selection) []string {
	var adwareLinks []string

	for _, domain := range r.adwareDomains {
		pageDom.Find(fmt.Sprintf("a[href^=\"%s\"]", domain)).Each(func(i int, s *goquery.Selection) {
			href, exists := s.Attr("href")
			if exists {
				adwareLinks = append(adwareLinks, href)
			}
		})
	}

	return adwareLinks
}

func (r *AdwareResolver) ResolveAdware(ctx context.Context, href string) (string, error) {
	// extract querysting "id" from url
	parsedUrl, err := url.Parse(href)
	if err != nil {
		logging.Error().Err(err).Str("href", href).Msg("Failed to parse URL")
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	decodedLink, err := r.ResolveEncodedAdware(ctx, parsedUrl)

	if err == nil {
		logging.Debug().Str("href", href).Str("decoded", decodedLink).Msg("Successfully resolved encoded adware link")
		return decodedLink, nil
	}

	logging.Warn().Str("href", href).Msg("Failed to resolve encoded adware link - trying web resolution")

	magnetLink, err := r.ResolveWebAdware(ctx, href)

	if err != nil {
		logging.Error().Err(err).Str("href", href).Msg("Failed to resolve adware link")
		return "", fmt.Errorf("failed to resolve adware link: %w", err)
	}

	if !IsMagnetLink(magnetLink) {
		logging.Error().Str("href", href).Str("decoded", magnetLink).Msg("Decoded link is not a valid magnet link")
		return "", fmt.Errorf("exhausted all resolution methods for adware link %s", href)
	}

	return magnetLink, nil
}

// ResolveEncodedAdware decodes the magnet link the adware page carries in its
// "id" query parameter, which holds the reversed base64 of the (HTML-escaped)
// magnet link. It performs no I/O, so the adware page never has to be fetched.
func (r *AdwareResolver) ResolveEncodedAdware(ctx context.Context, adURL *url.URL) (string, error) {
	encoded := adURL.Query().Get("id")
	if encoded == "" {
		return "", fmt.Errorf("empty string")
	}
	reversed := reverseString(encoded)

	decodedBytes, err := base64.StdEncoding.DecodeString(reversed)
	if err != nil {
		return "", err
	}

	htmlUnescaped := html.UnescapeString(string(decodedBytes))

	if !IsMagnetLink(htmlUnescaped) {
		return "", fmt.Errorf("decoded link is not a valid magnet link: %s", htmlUnescaped)
	}

	return htmlUnescaped, nil
}

// ResolveWebAdware fetches the adware page and extracts the magnet link from its HTML.
// It also caches the result for future requests.
func (r *AdwareResolver) ResolveWebAdware(ctx context.Context, url string) (string, error) {
	key := fmt.Sprintf("adware:%s", url)

	cachedLink, err := r.cache.Get(ctx, key)
	if err == nil {
		logging.Debug().Str("url", url).Msg("Adware link found in cache")
		return string(cachedLink), nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", SpoofedUserAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch adware page: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Step 2: Extract the magnet link from the page source
	magnetLink := magnetRegex.FindString(string(body))
	if magnetLink == "" {
		return "", fmt.Errorf("magnet link not found in adware page")
	}

	logging.Debug().Str("url", url).Str("magnetLink", magnetLink).Msg("Adware link resolved successfully")

	// Step 3: Cache the resolved link for future requests
	logging.Debug().Str("url", url).Str("magnetLink", magnetLink).Msg("Caching adware link")
	err = r.cache.SetWithExpiration(ctx, key, []byte(magnetLink), 24*time.Hour)
	if err != nil {
		logging.Warn().Err(err).Str("url", url).Msg("Failed to cache adware link")
	}

	return magnetLink, nil
}
