package utils

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const wantMagnet = "magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c&tr=udp%3A%2F%2Ftracker.example.com%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.sample.com%3A83%2Fannounce"

var errCacheMiss = errors.New("cache miss")

// fakeCache is an in-memory adwareCache that records what was written to it.
type fakeCache struct {
	entries map[string][]byte
	setTTLs map[string]time.Duration
	setErr  error
}

func newFakeCache() *fakeCache {
	return &fakeCache{
		entries: map[string][]byte{},
		setTTLs: map[string]time.Duration{},
	}
}

func (f *fakeCache) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := f.entries[key]
	if !ok {
		return nil, errCacheMiss
	}
	return value, nil
}

func (f *fakeCache) SetWithExpiration(_ context.Context, key string, value []byte, expiration time.Duration) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.entries[key] = value
	f.setTTLs[key] = expiration
	return nil
}

// adwareID builds the value an adware link carries in its "id" query parameter:
// the reversed base64 of the (HTML-escaped) magnet link.
func adwareID(link string) string {
	return reverseString(base64.StdEncoding.EncodeToString([]byte(link)))
}

// adwareURL builds an adware link on host carrying id in its "id" parameter.
func adwareURL(t *testing.T, host, id string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(host + "/go")
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", host, err)
	}
	if id != "" {
		parsed.RawQuery = url.Values{"id": {id}}.Encode()
	}
	return parsed
}

// newTestResolver serves the given page and returns a resolver pointed at it,
// along with the page URL and a counter of how many requests it received.
func newTestResolver(t *testing.T, cache AdwareCache, status int, page string) (*AdwareResolver, string, *int) {
	t.Helper()

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if got := r.Header.Get("User-Agent"); got != SpoofedUserAgent {
			t.Errorf("User-Agent = %q, want %q", got, SpoofedUserAgent)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(server.Close)

	return NewAdwareResolver(cache, server.URL), server.URL, &hits
}

func TestNewAdwareResolver_Domains(t *testing.T) {
	tests := []struct {
		name    string
		domains string
		want    []string
	}{
		{
			name:    "single domain",
			domains: "https://www.seuvideo.xyz",
			want:    []string{"https://www.seuvideo.xyz"},
		},
		{
			name:    "comma separated domains",
			domains: "https://www.seuvideo.xyz,https://www.systemads.org,https://superadsgo.xyz",
			want:    []string{"https://www.seuvideo.xyz", "https://www.systemads.org", "https://superadsgo.xyz"},
		},
		{
			name:    "surrounding whitespace is trimmed",
			domains: " https://www.seuvideo.xyz ,\thttps://www.systemads.org\n",
			want:    []string{"https://www.seuvideo.xyz", "https://www.systemads.org"},
		},
		{
			name:    "empty entries are dropped",
			domains: "https://www.seuvideo.xyz,,  ,https://superadsgo.xyz,",
			want:    []string{"https://www.seuvideo.xyz", "https://superadsgo.xyz"},
		},
		{
			name:    "empty configuration yields no domains",
			domains: "",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewAdwareResolver(newFakeCache(), tt.domains)
			if !reflect.DeepEqual(resolver.adwareDomains, tt.want) {
				t.Errorf("adwareDomains = %q, want %q", resolver.adwareDomains, tt.want)
			}
		})
	}
}

func TestAdwareResolver_ExtractAdwareLinks(t *testing.T) {
	const page = `<div class="entry-content">
	<a href="magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa">Magnet</a>
	<a href="https://www.seuvideo.xyz/go?id=abc">1080p</a>
	<a href="https://superadsgo.xyz/go?id=def">720p</a>
	<a href="https://www.seuvideo.xyz.evil.com/go?id=ghi">Impostor</a>
	<a href="https://legit.example.com/download">Site</a>
	<a>No href at all</a>
</div>`

	tests := []struct {
		name    string
		domains string
		want    []string
	}{
		{
			name:    "links are collected per configured domain",
			domains: "https://www.seuvideo.xyz,https://superadsgo.xyz",
			want: []string{
				"https://www.seuvideo.xyz/go?id=abc",
				"https://www.seuvideo.xyz.evil.com/go?id=ghi",
				"https://superadsgo.xyz/go?id=def",
			},
		},
		{
			name:    "domains with no match are skipped",
			domains: "https://superadsgo.xyz,https://www.systemads.org",
			want:    []string{"https://superadsgo.xyz/go?id=def"},
		},
		{
			name:    "no configured domains extracts nothing",
			domains: "",
			want:    nil,
		},
		{
			name:    "unrelated domain extracts nothing",
			domains: "https://www.systemads.org",
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(page))
			if err != nil {
				t.Fatalf("failed to parse test page: %v", err)
			}

			resolver := NewAdwareResolver(newFakeCache(), tt.domains)
			got := resolver.ExtractAdwareLinks(doc.Selection)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractAdwareLinks() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAdwareResolver_ResolveEncodedAdware(t *testing.T) {
	htmlEscapedMagnet := strings.ReplaceAll(wantMagnet, "&", "&amp;")

	tests := []struct {
		name string
		// id is the raw value of the "id" query parameter; when empty the
		// parameter is left off the URL entirely.
		id      string
		want    string
		wantErr bool
	}{
		{
			name: "reversed base64 magnet link",
			id:   "jVzYmJjZxYjYwMDZiVjZ2UTMmJGM3EmZ4E2M2cDZ0UGN4UmN5EWOlpDapRnY64mc11Dd49jO0VmbnFWb",
			want: "magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c",
		},
		{
			name: "magnet link with trackers",
			id:   adwareID(wantMagnet),
			want: wantMagnet,
		},
		{
			name: "html entities in the decoded link are unescaped",
			id:   adwareID(htmlEscapedMagnet),
			want: wantMagnet,
		},
		{
			name:    "no id query parameter",
			id:      "",
			wantErr: true,
		},
		{
			name:    "id is not valid base64",
			id:      "invalid_encoded_string",
			wantErr: true,
		},
		{
			name:    "id is base64 but not reversed",
			id:      base64.StdEncoding.EncodeToString([]byte(wantMagnet)),
			wantErr: true,
		},
		{
			name:    "decodes to a non-magnet link",
			id:      adwareID("https://watch.brplayer.example/movie/42"),
			wantErr: true,
		},
		{
			name:    "decodes to a magnet scheme without a query",
			id:      adwareID("magnet:"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewAdwareResolver(newFakeCache(), "https://ads.example.com")

			got, err := resolver.ResolveEncodedAdware(context.Background(), adwareURL(t, "https://ads.example.com", tt.id))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveEncodedAdware() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ResolveEncodedAdware() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdwareResolver_ResolveAdware(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		page    string
		want    string
		wantErr bool
	}{
		{
			name: "magnet assigned to a JS const inside a script tag",
			page: `<script >
const DEST_URL     = "magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c&tr=udp%3A%2F%2Ftracker.example.com%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.sample.com%3A83%2Fannounce";
</script>`,
			want: wantMagnet,
		},
		{
			name: "magnet inside a full adware page",
			page: `<!DOCTYPE html>
<html lang="pt-BR">
	<head>
		<title>Aguarde...</title>
		<script src="https://ads.example.com/loader.js"></script>
	</head>
	<body>
		<div id="countdown">5</div>
		<script >
const DEST_URL     = "magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c&tr=udp%3A%2F%2Ftracker.example.com%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.sample.com%3A83%2Fannounce";
setTimeout(function () { window.location.href = DEST_URL; }, 5000);
		</script>
	</body>
</html>`,
			want: wantMagnet,
		},
		{
			name: "minified html with no whitespace around the magnet",
			page: `<!DOCTYPE html><html><head><title>Aguarde...</title></head><body><div id="countdown">5</div><script>const DEST_URL="magnet:?xt=urn:btih:e9a96e84e4d763a8fa70bf156f5bd30b61f2fc5c&tr=udp%3A%2F%2Ftracker.example.com%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.sample.com%3A83%2Fannounce";setTimeout(function(){window.location.href=DEST_URL},5000);</script></body></html>`,
			want: wantMagnet,
		},
		{
			name: "magnet in an anchor href",
			page: `<a class="btn" href="` + wantMagnet + `">Baixar</a>`,
			want: wantMagnet,
		},
		{
			name: "first magnet wins when the page has several",
			page: `<a href="` + wantMagnet + `">1080p</a>` +
				`<a href="magnet:?xt=urn:btih:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa">720p</a>`,
			want: wantMagnet,
		},
		{
			name: "no magnet link in the page",
			page: `<!DOCTYPE html>
<html>
	<head><title>Aguarde...</title></head>
	<body>
		<div id="countdown">5</div>
		<script >
const DEST_URL     = "https://example.com/download?id=42";
</script>
	</body>
</html>`,
			wantErr: true,
		},
		{
			name:    "minified html with no magnet link",
			page:    `<!DOCTYPE html><html><head><title>Aguarde...</title></head><body><script>const DEST_URL="https://example.com/download?id=42";window.location.href=DEST_URL;</script></body></html>`,
			wantErr: true,
		},
		{
			name:    "empty page",
			page:    "",
			wantErr: true,
		},
		{
			name:    "magnet scheme without an infohash",
			page:    `<a href="magnet:?dn=Some.Movie.2024.1080p">Baixar</a>`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newFakeCache()
			resolver, pageURL, hits := newTestResolver(t, cache, http.StatusOK, tt.page)

			got, err := resolver.ResolveAdware(context.Background(), pageURL)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveAdware() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ResolveAdware() = %v, want %v", got, tt.want)
			}
			if *hits != 1 {
				t.Errorf("adware page requested %d times, want 1", *hits)
			}

			// A resolved magnet is cached for 24h; a failed one is not cached at all.
			key := "adware:" + pageURL
			cached, ok := cache.entries[key]
			if tt.wantErr {
				if ok {
					t.Errorf("cached %q under %q, want nothing cached on failure", cached, key)
				}
				return
			}
			if !ok {
				t.Fatalf("nothing cached under %q, want %q", key, tt.want)
			}
			if string(cached) != tt.want {
				t.Errorf("cached = %v, want %v", string(cached), tt.want)
			}
			if cache.setTTLs[key] != 24*time.Hour {
				t.Errorf("cache TTL = %v, want %v", cache.setTTLs[key], 24*time.Hour)
			}
		})
	}
}

func TestAdwareResolver_ResolveAdwarePrefersTheEncodedLink(t *testing.T) {
	resolver, pageURL, hits := newTestResolver(t, newFakeCache(), http.StatusOK, "<html><body>no magnet here</body></html>")
	href := adwareURL(t, pageURL, adwareID(wantMagnet)).String()

	got, err := resolver.ResolveAdware(context.Background(), href)
	if err != nil {
		t.Fatalf("ResolveAdware() error = %v, want nil", err)
	}
	if got != wantMagnet {
		t.Errorf("ResolveAdware() = %v, want %v", got, wantMagnet)
	}
	if *hits != 0 {
		t.Errorf("adware page requested %d times, want 0 when the link decodes locally", *hits)
	}
}

func TestAdwareResolver_ResolveAdwareFallsBackToTheWebWhenTheIDIsUndecodable(t *testing.T) {
	resolver, pageURL, hits := newTestResolver(t, newFakeCache(), http.StatusOK, `<a href="`+wantMagnet+`">Baixar</a>`)
	href := adwareURL(t, pageURL, "not-base64-at-all").String()

	got, err := resolver.ResolveAdware(context.Background(), href)
	if err != nil {
		t.Fatalf("ResolveAdware() error = %v, want nil", err)
	}
	if got != wantMagnet {
		t.Errorf("ResolveAdware() = %v, want %v", got, wantMagnet)
	}
	if *hits != 1 {
		t.Errorf("adware page requested %d times, want 1", *hits)
	}
}

func TestAdwareResolver_ResolveAdwareServesFromCache(t *testing.T) {
	cache := newFakeCache()
	resolver, pageURL, hits := newTestResolver(t, cache, http.StatusOK, "<html><body>no magnet here</body></html>")
	cache.entries["adware:"+pageURL] = []byte(wantMagnet)

	got, err := resolver.ResolveAdware(context.Background(), pageURL)
	if err != nil {
		t.Fatalf("ResolveAdware() error = %v, want nil", err)
	}
	if got != wantMagnet {
		t.Errorf("ResolveAdware() = %v, want %v", got, wantMagnet)
	}
	if *hits != 0 {
		t.Errorf("adware page requested %d times, want 0 on a cache hit", *hits)
	}
}

func TestAdwareResolver_ResolveAdwareCacheWriteFailure(t *testing.T) {
	cache := newFakeCache()
	cache.setErr = errors.New("redis is down")
	resolver, pageURL, _ := newTestResolver(t, cache, http.StatusOK, `<a href="`+wantMagnet+`">Baixar</a>`)

	// A cache write failure is logged but must not fail the resolution.
	got, err := resolver.ResolveAdware(context.Background(), pageURL)
	if err != nil {
		t.Fatalf("ResolveAdware() error = %v, want nil", err)
	}
	if got != wantMagnet {
		t.Errorf("ResolveAdware() = %v, want %v", got, wantMagnet)
	}
}

func TestAdwareResolver_ResolveAdwareNonOKStatus(t *testing.T) {
	resolver, pageURL, _ := newTestResolver(t, newFakeCache(), http.StatusNotFound, `<a href="`+wantMagnet+`">Baixar</a>`)

	got, err := resolver.ResolveAdware(context.Background(), pageURL)
	if err == nil {
		t.Fatalf("ResolveAdware() error = nil, want an error for a 404 response")
	}
	if got != "" {
		t.Errorf("ResolveAdware() = %v, want empty string", got)
	}
}

func TestAdwareResolver_ResolveAdwareUnparseableURL(t *testing.T) {
	resolver := NewAdwareResolver(newFakeCache(), "https://ads.example.com")

	got, err := resolver.ResolveAdware(context.Background(), "://ads.example.com/go?id=abc")
	if err == nil {
		t.Fatalf("ResolveAdware() error = nil, want an error for an unparseable URL")
	}
	if got != "" {
		t.Errorf("ResolveAdware() = %v, want empty string", got)
	}
}
