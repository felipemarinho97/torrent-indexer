package requester

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const challengePage = `<html><head><title>Just a moment...</title></head><body>cf-chl-bypass</body></html>`

// mapCache is a Cache that keeps everything in memory.
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

// site serves a page that hands out a cookie and a page that requires it, and
// can pretend to be behind an anti-bot challenge.
type site struct {
	challengeEntry bool
	entryHits      int
	protectedHits  int
	sawCookie      string
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/protegido":
		s.protectedHits++
		if cookie, err := r.Cookie("PHPSESSID"); err == nil {
			s.sawCookie = cookie.Value
		}
		fmt.Fprint(w, `<html><body>protegido</body></html>`)
	case "/nao-existe":
		http.NotFound(w, r)
	case "/redireciona":
		http.Redirect(w, r, "/protegido", http.StatusFound)
	default:
		s.entryHits++
		if s.challengeEntry {
			fmt.Fprint(w, challengePage)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "plain-session", Path: "/"})
		fmt.Fprint(w, `<html><body>entrada</body></html>`)
	}
}

// solver mimics a FlareSolverr-compatible API. Like Byparr it does not
// implement the sessions commands, which is the path that fills the pool with
// placeholder ids.
type solver struct {
	calls    int
	body     string
	cookies  []map[string]string
	finalURL string
}

func (s *solver) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Cmd string `json:"cmd"`
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&request)
	if request.Cmd != "request.get" {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"detail":"not supported"}`)
		return
	}

	s.calls++
	finalURL := request.URL
	if s.finalURL != "" {
		finalURL = s.finalURL
	}
	response := map[string]any{
		"status": "ok",
		"solution": map[string]any{
			"url":       finalURL,
			"status":    200,
			"cookies":   s.cookies,
			"userAgent": "SolverAgent/1.0",
			"response":  s.body,
		},
	}
	_ = json.NewEncoder(w).Encode(response)
}

func newSession(t *testing.T, target *site, flare *solver) (*Session, *mapCache, string) {
	t.Helper()
	siteServer := httptest.NewServer(target)
	t.Cleanup(siteServer.Close)

	flareAddress := ""
	if flare != nil {
		flareServer := httptest.NewServer(flare)
		t.Cleanup(flareServer.Close)
		flareAddress = flareServer.URL
	}

	cache := newMapCache()
	r := NewRequester(NewFlareSolverr(flareAddress, 5000, 1), cache, 5*time.Second)
	session, err := r.NewSession()
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	return session, cache, siteServer.URL
}

func TestSession_Get(t *testing.T) {
	t.Run("keeps cookies across the requests of a handshake", func(t *testing.T) {
		target := &site{}
		session, _, baseURL := newSession(t, target, nil)

		if _, err := session.Get(context.Background(), baseURL+"/", "", ""); err != nil {
			t.Fatalf("entry page: %v", err)
		}
		if _, err := session.Get(context.Background(), baseURL+"/protegido", baseURL, ""); err != nil {
			t.Fatalf("protected page: %v", err)
		}
		if target.sawCookie != "plain-session" {
			t.Errorf("the protected page saw cookie %q, want %q", target.sawCookie, "plain-session")
		}
	})

	t.Run("leaves flaresolverr alone while the plain client works", func(t *testing.T) {
		flare := &solver{body: `<html><body>do solver</body></html>`}
		session, _, baseURL := newSession(t, &site{}, flare)

		body, err := session.Get(context.Background(), baseURL+"/", "", "")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if !strings.Contains(string(body), "entrada") {
			t.Errorf("body = %q, want the page served by the site itself", body)
		}
		if flare.calls != 0 {
			t.Errorf("called flaresolverr %d times, want 0", flare.calls)
		}
	})

	t.Run("clears a challenge through flaresolverr and adopts its cookies", func(t *testing.T) {
		target := &site{challengeEntry: true}
		flare := &solver{
			body:    `<html><body>entrada liberada</body></html>`,
			cookies: []map[string]string{{"name": "PHPSESSID", "value": "solver-session"}},
		}
		session, _, baseURL := newSession(t, target, flare)

		body, err := session.Get(context.Background(), baseURL+"/", "", "")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if !strings.Contains(string(body), "entrada liberada") {
			t.Errorf("body = %q, want the page the solver returned", body)
		}
		if flare.calls != 1 {
			t.Errorf("called flaresolverr %d times, want 1", flare.calls)
		}

		// the request that follows must go out as the visitor the solver
		// established, otherwise the site hands out a second session
		if _, err := session.Get(context.Background(), baseURL+"/protegido", baseURL, ""); err != nil {
			t.Fatalf("protected page: %v", err)
		}
		if target.sawCookie != "solver-session" {
			t.Errorf("the protected page saw cookie %q, want %q", target.sawCookie, "solver-session")
		}
		if session.userAgent != "SolverAgent/1.0" {
			t.Errorf("session user agent = %q, want the solver's", session.userAgent)
		}
	})

	t.Run("reports a challenge it cannot clear", func(t *testing.T) {
		session, _, baseURL := newSession(t, &site{challengeEntry: true}, nil)

		_, err := session.Get(context.Background(), baseURL+"/", "", "")
		if err != ErrChallenge {
			t.Errorf("error = %v, want ErrChallenge", err)
		}
	})

	t.Run("turns an error status into an error", func(t *testing.T) {
		session, _, baseURL := newSession(t, &site{}, nil)

		_, err := session.Get(context.Background(), baseURL+"/nao-existe", "", "")
		if err == nil {
			t.Fatal("expected an error for a 404")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("error = %v, want it to name the status", err)
		}
	})

	t.Run("stores and serves a body under the given key", func(t *testing.T) {
		target := &site{}
		session, cache, baseURL := newSession(t, target, nil)

		for i := 0; i < 3; i++ {
			if _, err := session.Get(context.Background(), baseURL+"/", "", "chave-estavel"); err != nil {
				t.Fatalf("call %d: %v", i, err)
			}
		}
		if target.entryHits != 1 {
			t.Errorf("hit the site %d times, want 1", target.entryHits)
		}
		if _, ok := cache.entries["shortLivedCache:chave-estavel"]; !ok {
			t.Error("nothing cached under the given key")
		}
	})
}

func TestSession_LastURL(t *testing.T) {
	t.Run("reports where a redirect landed", func(t *testing.T) {
		session, _, baseURL := newSession(t, &site{}, nil)

		if _, err := session.Get(context.Background(), baseURL+"/redireciona", "", ""); err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got, want := session.LastURL(), baseURL+"/protegido"; got != want {
			t.Errorf("LastURL() = %q, want %q", got, want)
		}
	})

	t.Run("reports where flaresolverr landed", func(t *testing.T) {
		// a solver that silently ends up somewhere else is the only signal a
		// caller gets that its handshake was rejected
		flare := &solver{body: `<html><body>página inicial</body></html>`, finalURL: "https://exemplo/"}
		session, _, baseURL := newSession(t, &site{challengeEntry: true}, flare)

		if _, err := session.Get(context.Background(), baseURL+"/", "", ""); err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got, want := session.LastURL(), "https://exemplo/"; got != want {
			t.Errorf("LastURL() = %q, want %q", got, want)
		}
	})
}
