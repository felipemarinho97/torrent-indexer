package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/requester"
	"github.com/felipemarinho97/torrent-indexer/schema"
)

const (
	magnet720  = "magnet:?xt=urn:btih:51ca3ba90f04bf50394e9eb7e448ceea214d5ad6&dn=Matrix+720p"
	magnet1080 = "magnet:?xt=urn:btih:8cc4fdc2c777d1464d9043397309f45859b9297b&dn=Matrix+1080p"
)

func mustParseHTML(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}
	return doc
}

func downloadRow(name, magnetLink string) string {
	return fmt.Sprintf(`<div class="download-row"><div class="download-name">%s</div><a href="%s">baixar</a></div>`, name, magnetLink)
}

func specCard(label, value string) string {
	return fmt.Sprintf(`<div class="spec-text"><small>%s</small><strong>%s</strong></div>`, label, value)
}

func sizeCard(size string) string { return specCard("Tamanho do Arquivo", size) }

func Test_extractMagnetsRedeTorrent(t *testing.T) {
	type args struct {
		html string
	}
	tests := []struct {
		name      string
		args      args
		wantLinks []string
		wantSizes map[string]string
	}{
		{
			// the page prints one of the two sizes a second time in its spec
			// card, so a document-wide size list holds three entries for two
			// magnets — and deduplicating it leaves the sizes in an order that
			// pairs 796 MB with the 1080p release and 2.03 GB with the 720p one
			name: "reads each release's size from its own download row",
			args: args{html: `<html><body>` +
				downloadRow("1ª OPÇÃO DOWNLOAD TORRENT DUBLADO 720P / 796 MB", magnet720) +
				downloadRow("2ª OPÇÃO DOWNLOAD TORRENT DUAL ÁUDIO 1080P / 2.03 GB", magnet1080) +
				sizeCard("796 MB") +
				`</body></html>`},
			wantLinks: []string{magnet720, magnet1080},
			wantSizes: map[string]string{magnet720: "796 MB", magnet1080: "2.03 GB"},
		},
		{
			name: "falls back to the spec card when the only release has no size",
			args: args{html: `<html><body>` +
				downloadRow("DOWNLOAD TORRENT DUAL ÁUDIO MKV BLURAY 1080P", magnet1080) +
				sizeCard("1.81 GB") +
				`</body></html>`},
			wantLinks: []string{magnet1080},
			wantSizes: map[string]string{magnet1080: "1.81 GB"},
		},
		{
			// with several releases the spec card cannot be attributed to any of
			// them, so no size is better than the wrong size
			name: "ignores the spec card when the page offers several releases",
			args: args{html: `<html><body>` +
				downloadRow("DOWNLOAD TORRENT DUAL ÁUDIO MKV 1080P", magnet1080) +
				downloadRow("DOWNLOAD TORRENT DUBLADO MKV 720P", magnet720) +
				sizeCard("3.23 GB") +
				`</body></html>`},
			wantLinks: []string{magnet1080, magnet720},
			wantSizes: map[string]string{},
		},
		{
			name: "collects magnets that sit outside a download row",
			args: args{html: `<html><body>` +
				downloadRow("DOWNLOAD TORRENT DUBLADO 720P / 796 MB", magnet720) +
				fmt.Sprintf(`<p><a href="%s">espelho</a></p>`, magnet1080) +
				`</body></html>`},
			wantLinks: []string{magnet720, magnet1080},
			wantSizes: map[string]string{magnet720: "796 MB"},
		},
		{
			name:      "returns nothing for a page without magnets",
			args:      args{html: `<html><body><div class="download-row"><div class="download-name">LINKBOX</div></div>` + sizeCard("1.26 GB") + `</body></html>`},
			wantLinks: nil,
			wantSizes: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLinks, gotSizes := extractMagnetsRedeTorrent(mustParseHTML(t, tt.args.html))
			if !reflect.DeepEqual(gotLinks, tt.wantLinks) {
				t.Errorf("extractMagnetsRedeTorrent() links = %v, want %v", gotLinks, tt.wantLinks)
			}
			if !reflect.DeepEqual(gotSizes, tt.wantSizes) {
				t.Errorf("extractMagnetsRedeTorrent() sizes = %v, want %v", gotSizes, tt.wantSizes)
			}
		})
	}
}

func Test_extractAudioRedeTorrent(t *testing.T) {
	type args struct {
		html string
	}
	tests := []struct {
		name string
		args args
		want []schema.Audio
	}{
		{
			name: "reads both languages from the Idioma card",
			args: args{html: "<html><body>" + specCard("Idioma / Áudio", "Inglês / Português") + "</body></html>"},
			want: []schema.Audio{schema.AudioEnglish, schema.AudioPortuguese},
		},
		{
			// "Qualidade de Áudio" sits right next to it and holds a score, so
			// matching on "Áudio" alone would parse "10 / 10" as languages
			name: "is not fooled by the neighbouring audio quality card",
			args: args{html: "<html><body>" +
				specCard("Qualidade de Áudio", "10 / 10") +
				specCard("Idioma / Áudio", "Português") +
				"</body></html>"},
			want: []schema.Audio{schema.AudioPortuguese},
		},
		{
			name: "returns nothing when the card is absent",
			args: args{html: "<html><body>" + sizeCard("1.81 GB") + "</body></html>"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAudioRedeTorrent(mustParseHTML(t, tt.args.html)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractAudioRedeTorrent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_extractMovieJSONLD(t *testing.T) {
	movieLD := `<script type="application/ld+json">{"@context":"https://schema.org","@type":"Movie",
		"name":"Jackie Brown","copyrightYear":"1997","sameAs":"https://www.imdb.com/title/tt0119396/"}</script>`

	type args struct {
		html string
	}
	tests := []struct {
		name      string
		args      args
		wantTitle string
		wantYear  string
		wantIMDB  string
	}{
		{
			name:      "reads title, year and imdb from the Movie block",
			args:      args{html: "<html><head>" + movieLD + "</head><body></body></html>"},
			wantTitle: "Jackie Brown",
			wantYear:  "1997",
			wantIMDB:  "https://www.imdb.com/title/tt0119396/",
		},
		{
			name: "skips blocks of another type and malformed json",
			args: args{html: `<html><head>` +
				`<script type="application/ld+json">{"@type":"BreadcrumbList","name":"Filmes"}</script>` +
				`<script type="application/ld+json">{"@type":"Movie",</script>` +
				movieLD + `</head><body></body></html>`},
			wantTitle: "Jackie Brown",
			wantYear:  "1997",
			wantIMDB:  "https://www.imdb.com/title/tt0119396/",
		},
		{
			name: "leaves imdb empty when sameAs points elsewhere",
			args: args{html: `<html><head><script type="application/ld+json">` +
				`{"@type":"Movie","name":"Duna","copyrightYear":"2021","sameAs":"https://pt.wikipedia.org/wiki/Duna"}` +
				`</script></head><body></body></html>`},
			wantTitle: "Duna",
			wantYear:  "2021",
			wantIMDB:  "",
		},
		{
			name: "returns empty strings when the page has no JSON-LD",
			args: args{html: "<html><body><h1>Sem dados</h1></body></html>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTitle, gotYear, gotIMDB := extractMovieJSONLD(mustParseHTML(t, tt.args.html))
			if gotTitle != tt.wantTitle || gotYear != tt.wantYear || gotIMDB != tt.wantIMDB {
				t.Errorf("extractMovieJSONLD() = (%q, %q, %q), want (%q, %q, %q)",
					gotTitle, gotYear, gotIMDB, tt.wantTitle, tt.wantYear, tt.wantIMDB)
			}
		})
	}
}

func Test_formatBytesToSize(t *testing.T) {
	type args struct {
		byteStr string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "converts bytes to GB", args: args{byteStr: "2147483648"}, want: "2.00 GB"},
		{name: "converts bytes to MB", args: args{byteStr: "1048576"}, want: "1.00 MB"},
		{name: "converts bytes to KB", args: args{byteStr: "1024"}, want: "1.00 KB"},
		{name: "returns empty for an empty string", args: args{byteStr: ""}, want: ""},
		{name: "returns empty for a non-number", args: args{byteStr: "1.2 GB"}, want: ""},
		{name: "returns empty for zero", args: args{byteStr: "0"}, want: ""},
		{name: "returns empty for a negative length", args: args{byteStr: "-1024"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatBytesToSize(tt.args.byteStr); got != tt.want {
				t.Errorf("formatBytesToSize() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mapCache is a requester.Cache that keeps everything in memory.
type mapCache struct{ entries map[string][]byte }

func newMapCache() *mapCache { return &mapCache{entries: map[string][]byte{}} }

func (m *mapCache) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := m.entries[key]
	if !ok {
		return nil, fmt.Errorf("miss")
	}
	return value, nil
}

func (m *mapCache) SetWithExpiration(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.entries[key] = value
	return nil
}

func (m *mapCache) Del(_ context.Context, key string) error {
	delete(m.entries, key)
	return nil
}

// redeTorrentSite mimics the search handshake of redestorrents.com: the
// homepage hands out a session cookie and a token bound to it, and index.php
// redirects to "/" unless it is called back with both.
type redeTorrentSite struct {
	tokens     map[string]string // session id -> token
	homeHits   int
	searchHits int
	omitToken  bool
	staleToken bool
}

func (s *redeTorrentSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/index.php" {
		s.searchHits++
		cookie, err := r.Cookie("PHPSESSID")
		if err != nil || s.tokens[cookie.Value] != r.URL.Query().Get("token") {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		fmt.Fprintf(w, `<html><body><a class="cover-link" href="https://site/%s/">ok</a></body></html>`,
			r.URL.Query().Get("busca"))
		return
	}

	s.homeHits++
	session := fmt.Sprintf("session-%d", s.homeHits)
	token := fmt.Sprintf("%032x", s.homeHits)
	if s.staleToken {
		s.tokens[session] = "a-different-token"
	} else {
		s.tokens[session] = token
	}
	http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: session, Path: "/"})
	field := fmt.Sprintf(`<input type="hidden" name="token" value="%s">`, token)
	if s.omitToken {
		field = ""
	}
	fmt.Fprintf(w, `<html><body><form action="/index.php">%s</form>
		<a class="cover-link" href="https://site/ultimo-lancamento/">último lançamento</a></body></html>`, field)
}

func newSearchTestServer(t *testing.T, site *redeTorrentSite) (*requester.Requster, *mapCache, string) {
	t.Helper()
	site.tokens = map[string]string{}
	server := httptest.NewServer(site)
	t.Cleanup(server.Close)

	cache := newMapCache()
	// an unreachable address keeps FlareSolverr out of these tests
	req := requester.NewRequester(requester.NewFlareSolverr("", 1000, 1), cache, 5*time.Second)
	return req, cache, server.URL + "/"
}

func Test_searchRedeTorrent(t *testing.T) {
	t.Run("returns the results the handshake unlocks", func(t *testing.T) {
		site := &redeTorrentSite{}
		req, _, baseURL := newSearchTestServer(t, site)

		doc, err := searchRedeTorrent(context.Background(), req, baseURL, "jackie brown")
		if err != nil {
			t.Fatalf("searchRedeTorrent() error = %v", err)
		}
		// the fixture echoes the decoded query, so this also proves the term
		// reached the site instead of being swallowed by the redirect
		if got, want := doc.Find("a.cover-link").AttrOr("href", ""), "https://site/jackie brown/"; got != want {
			t.Errorf("result link = %q, want %q", got, want)
		}
	})

	t.Run("reports a rejected handshake instead of the homepage it lands on", func(t *testing.T) {
		// the site redirects to "/", which still renders a full page of latest
		// releases — returning those as search results is the bug this guards
		site := &redeTorrentSite{staleToken: true}
		req, _, baseURL := newSearchTestServer(t, site)

		doc, err := searchRedeTorrent(context.Background(), req, baseURL, "jackie brown")
		if err == nil {
			t.Fatalf("expected an error, got a document with %d links", doc.Find("a.cover-link").Length())
		}
		if !strings.Contains(err.Error(), "rejected") {
			t.Errorf("error = %v, want it to mention the rejected handshake", err)
		}
	})

	t.Run("fails when the homepage carries no token", func(t *testing.T) {
		site := &redeTorrentSite{omitToken: true}
		req, _, baseURL := newSearchTestServer(t, site)

		if _, err := searchRedeTorrent(context.Background(), req, baseURL, "jackie brown"); err == nil {
			t.Error("expected an error when the search form has no token")
		}
	})

	t.Run("serves a repeated search from the cache", func(t *testing.T) {
		site := &redeTorrentSite{}
		req, cache, baseURL := newSearchTestServer(t, site)

		for i := 0; i < 3; i++ {
			if _, err := searchRedeTorrent(context.Background(), req, baseURL, "jackie brown"); err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
		}
		if site.homeHits != 1 || site.searchHits != 1 {
			t.Errorf("hit the site %d times for the token and %d times for the search, want 1 and 1",
				site.homeHits, site.searchHits)
		}
		// the token is single-use, so the cache key must not carry it
		key := "shortLivedCache:" + baseURL + "index.php?busca=jackie+brown"
		if _, ok := cache.entries[key]; !ok {
			t.Errorf("cache key %q not found, got %v", key, reflect.ValueOf(cache.entries).MapKeys())
		}
	})
}
