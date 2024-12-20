package requester

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"

	"github.com/felipemarinho97/torrent-indexer/utils"
)

type FlareSolverr struct {
	url         string
	maxTimeout  int
	httpClient  *http.Client
	sessionPool chan string
	mu          sync.Mutex
	initiated   bool
}

var (
	ErrListSessions = fmt.Errorf("failed to list sessions")
)

func NewFlareSolverr(url string, timeoutMilli int) *FlareSolverr {
	poolSize := 5
	httpClient := &http.Client{}
	sessionPool := make(chan string, poolSize) // Pool size of 5 sessions

	f := &FlareSolverr{
		url:         url,
		maxTimeout:  timeoutMilli,
		httpClient:  httpClient,
		sessionPool: sessionPool,
	}

	err := f.FillSessionPool()
	if err == nil {
		f.initiated = true
	}

	return f
}

func (f *FlareSolverr) FillSessionPool() error {
	// Check if the pool is already filled
	if len(f.sessionPool) == cap(f.sessionPool) {
		return nil
	}

	// Pre-initialize the pool with existing sessions
	sessions, err := f.ListSessions()
	if err != nil {
		// if fail to list sessions, it may not support the sessions.list command
		// create new dumb sessions to fill the pool
		if err == ErrListSessions {
			for len(f.sessionPool) < cap(f.sessionPool) {
				f.sessionPool <- "dumb-session"
			}
			return nil
		}
		fmt.Println("Failed to list existing FlareSolverr sessions:", err)
		return err
	} else {
		for _, session := range sessions {
			// Add available sessions to the pool
			if len(f.sessionPool) < cap(f.sessionPool) {
				f.sessionPool <- session
			}
		}
		if len(f.sessionPool) > 0 {
			fmt.Printf("Added %d FlareSolverr sessions to the pool\n", len(f.sessionPool))
		}
	}

	// If fewer than poolSize sessions were found, create new ones to fill the pool
	for len(f.sessionPool) < cap(f.sessionPool) {
		f.CreateSession()
	}

	return nil
}

func (f *FlareSolverr) CreateSession() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	body := map[string]string{"cmd": "sessions.create"}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return ""
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1", f.url), bytes.NewBuffer(jsonBody))
	if err != nil {
		return ""
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return ""
	}

	defer resp.Body.Close()

	var sessionResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&sessionResponse)
	if err != nil {
		return ""
	}

	session := sessionResponse["session"].(string)
	// Add session to the pool
	f.sessionPool <- session

	fmt.Println("Created new FlareSolverr session:", session)
	return session
}

func (f *FlareSolverr) ListSessions() ([]string, error) {
	body := map[string]string{"cmd": "sessions.list"}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1", f.url), bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var sessionsResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&sessionsResponse)
	if err != nil {
		return nil, err
	}
	if sessionsResponse["sessions"] == nil {
		return nil, ErrListSessions
	}

	sessions := sessionsResponse["sessions"].([]interface{})
	var sessionIDs []string
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.(string))
	}

	return sessionIDs, nil
}

func (f *FlareSolverr) RetrieveSession() string {
	// Blocking receive from the session pool.
	session := <-f.sessionPool
	return session
}

type Response struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		Url     string `json:"url"`
		Status  int    `json:"status"`
		Cookies []struct {
			Domain   string `json:"domain"`
			Expiry   int    `json:"expiry"`
			HttpOnly bool   `json:"httpOnly"`
			Name     string `json:"name"`
			Path     string `json:"path"`
			SameSite string `json:"sameSite"`
			Secure   bool   `json:"secure"`
			Value    string `json:"value"`
		} `json:"cookies"`
		UserAgent string            `json:"userAgent"`
		Headers   map[string]string `json:"headers"`
		Response  string            `json:"response"`
	} `json:"solution"`
}

func (f *FlareSolverr) Get(_url string) (io.ReadCloser, error) {
	// Check if the FlareSolverr instance was initiated
	if !f.initiated {
		return io.NopCloser(bytes.NewReader([]byte(""))), nil
	}

	// Retrieve session from the pool (blocking if no sessions available)
	session := f.RetrieveSession()

	// Ensure the session is returned to the pool after the request is done
	defer func() {
		f.sessionPool <- session
	}()

	body := map[string]string{
		"cmd":        "request.get",
		"url":        _url,
		"maxTimeout": fmt.Sprintf("%d", f.maxTimeout),
		"session":    session,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1", f.url), bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Parse the response
	var response Response
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	// Check if the response was successful
	if response.Status != "ok" {
		return nil, fmt.Errorf("failed to get response: %s", response.Message)
	}

	// Check if "Under attack" is in the response
	if strings.Contains(response.Solution.Response, "Under attack") {
		return nil, fmt.Errorf("under attack")
	}

	// check if the response is valid HTML
	if !utils.IsValidHTML(response.Solution.Response) {
		fmt.Printf("[FlareSolverr] Invalid HTML response from %s\n", _url)
		response.Solution.Response = ""
	}

	// If the response body is empty but cookies are present, make a new request
	if response.Solution.Response == "" && len(response.Solution.Cookies) > 0 {
		fmt.Printf("[FlareSolverr] Making a new request to %s with cookies\n", _url)
		// Create a new request with cookies
		client := &http.Client{}
		cookieJar, err := cookiejar.New(&cookiejar.Options{})
		if err != nil {
			return nil, err
		}
		for _, cookie := range response.Solution.Cookies {
			cookieJar.SetCookies(&url.URL{Host: cookie.Domain}, []*http.Cookie{
				{
					Name:   cookie.Name,
					Value:  cookie.Value,
					Domain: cookie.Domain,
					Path:   cookie.Path,
				},
			})
		}
		client.Jar = cookieJar

		secondReq, err := http.NewRequest("GET", _url, nil)
		if err != nil {
			return nil, err
		}

		// use the same user returned by the FlareSolverr
		secondReq.Header.Set("User-Agent", response.Solution.UserAgent)

		secondResp, err := client.Do(secondReq)
		if err != nil {
			return nil, err
		}

		respByte := new(bytes.Buffer)
		_, err = respByte.ReadFrom(secondResp.Body)
		if err != nil {
			return nil, err
		}

		// Return the body of the second request
		return io.NopCloser(bytes.NewReader(respByte.Bytes())), nil
	}

	// Return the original response body
	return io.NopCloser(bytes.NewReader([]byte(response.Solution.Response))), nil
}
