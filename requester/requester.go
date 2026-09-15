package requester

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"time"

	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/utils"
	"github.com/fereidani/httpdecompressor"
)

const (
	cacheKey = "shortLivedCache"
)

var challangeRegex = regexp.MustCompile(`(?i)(just a moment|cf-chl-bypass|under attack)`)

// Cache is the part of the cache the requester uses, so that callers can pass a
// *cache.Redis in production and a fake in tests.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	SetWithExpiration(ctx context.Context, key string, value []byte, expiration time.Duration) error
	Del(ctx context.Context, key string) error
}

type Requster struct {
	fs                        *FlareSolverr
	c                         Cache
	httpClient                *http.Client
	shortLivedCacheExpiration time.Duration
}

func NewRequester(fs *FlareSolverr, c Cache, timeout time.Duration) *Requster {
	httpClient := &http.Client{
		Timeout: timeout,
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
		body, err = i.fs.Get(ctx, url, 3)
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
		body, err = i.fs.Get(ctx, url, 3)
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

// ErrChallenge is returned when a response still looks like an anti-bot
// challenge after every available strategy was tried.
var ErrChallenge = errors.New("response is a challenge")

// Session groups requests that have to share cookies, for sites whose search
// needs a handshake: a session cookie plus a token scraped from an earlier
// page. GetDocument cannot serve those — it keeps no cookie jar, and the
// one-shot token in the URL would make its cache store nothing but misses.
//
// A Session stands for a single visitor and is not safe for concurrent use.
type Session struct {
	r         *Requster
	client    *http.Client
	jar       http.CookieJar
	userAgent string
	lastURL   string
}

// NewSession opens a cookie-sharing session on top of the requester's transport
// and timeout.
func (i *Requster) NewSession() (*Session, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	return &Session{
		r:   i,
		jar: jar,
		client: &http.Client{
			Transport: i.httpClient.Transport,
			Timeout:   i.httpClient.Timeout,
			Jar:       jar,
		},
	}, nil
}

// LastURL is the URL the last Get ended on, after redirects. A handshake that
// the site rejects lands on its homepage instead of the search page, which is
// otherwise indistinguishable from a page of results.
func (s *Session) LastURL() string {
	return s.lastURL
}

// Cached returns a previously stored body for key, if any. Callers pass a key
// of their own because handshake URLs carry a token that is never seen twice.
func (s *Session) Cached(ctx context.Context, key string) ([]byte, bool) {
	body, err := s.r.c.Get(ctx, fmt.Sprintf("%s:%s", cacheKey, key))
	if err != nil {
		return nil, false
	}
	return body, true
}

// Get fetches url reusing the session's cookies, falling back to FlareSolverr
// the way GetDocument does. A non-empty key also stores the body in the
// short-lived cache under that key.
func (s *Session) Get(ctx context.Context, rawURL, referer, key string) ([]byte, error) {
	if key != "" {
		if body, ok := s.Cached(ctx, key); ok {
			logging.Debug().Str("url", rawURL).Msg("Returning from short-lived cache")
			s.lastURL = rawURL
			return body, nil
		}
	}

	body, err := s.get(ctx, rawURL, referer)
	if err != nil {
		return nil, err
	}

	if key != "" {
		s.Store(ctx, key, body)
	}

	return body, nil
}

// Store puts body in the short-lived cache under key, for callers that have to
// validate a response before it is worth keeping.
func (s *Session) Store(ctx context.Context, key string, body []byte) {
	if !utils.IsValidHTML(string(body)) {
		return
	}
	err := s.r.c.SetWithExpiration(ctx, fmt.Sprintf("%s:%s", cacheKey, key), body, s.r.shortLivedCacheExpiration)
	if err != nil {
		logging.Error().Err(err).Str("key", key).Msg("Failed to save response to cache")
	}
}

func (s *Session) get(ctx context.Context, rawURL, referer string) ([]byte, error) {
	body, err := s.plainGet(ctx, rawURL, referer)
	if err == nil && !hasChallange(body) {
		logging.Debug().Str("url", rawURL).Msg("Request served from plain client")
		return body, nil
	}
	if err != nil {
		logging.Debug().Str("url", rawURL).Err(err).Msg("Session request failed, falling back to flaresolverr")
	}

	if s.r.fs == nil || !s.r.fs.Initiated() {
		if err != nil {
			return nil, err
		}
		return nil, ErrChallenge
	}

	return s.flareGet(ctx, rawURL)
}

func (s *Session) plainGet(ctx context.Context, rawURL, referer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for url %s: %w", rawURL, err)
	}

	spoofBrowserHeaders(req, referer)
	if s.userAgent != "" {
		// stay on the user agent the solver established for these cookies
		req.Header.Set("User-Agent", s.userAgent)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request for url %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	s.lastURL = resp.Request.URL.String()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("unexpected status %s for url %s", resp.Status, rawURL)
	}

	reader, err := httpdecompressor.Reader(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress response: %w", err)
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (s *Session) flareGet(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	solution, err := s.r.fs.GetSolution(ctx, rawURL, s.jar.Cookies(parsed), 3)
	if err != nil {
		return nil, err
	}

	// keep the jar and user agent aligned with the solver, so the next plain
	// request in this session is still recognized as the same visitor
	if len(solution.Cookies) > 0 {
		s.jar.SetCookies(parsed, solution.Cookies)
	}
	if solution.UserAgent != "" {
		s.userAgent = solution.UserAgent
	}
	if hasChallange(solution.Body) {
		return nil, ErrChallenge
	}
	if len(solution.Body) == 0 {
		return nil, fmt.Errorf("flaresolverr returned an empty body for url %s", rawURL)
	}

	// the solver reports where it ended up, which is how a silent redirect away
	// from the requested page becomes visible to the caller
	s.lastURL = solution.URL
	if s.lastURL == "" {
		s.lastURL = rawURL
	}
	logging.Debug().Str("url", rawURL).Str("final_url", s.lastURL).Msg("Request served from flaresolverr")

	return solution.Body, nil
}
