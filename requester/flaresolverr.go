package requester

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type FlareSolverr struct {
	url        string
	maxTimeout int
	httpClient *http.Client
}

func NewFlareSolverr(url string, timeoutMilli int) *FlareSolverr {
	httpClient := &http.Client{}
	return &FlareSolverr{url: url, maxTimeout: timeoutMilli, httpClient: httpClient}
}

func (f *FlareSolverr) CreateSession() string {
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

	return sessionResponse["session"].(string)
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

	sessions := sessionsResponse["sessions"].([]interface{})
	var sessionIDs []string
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.(string))
	}

	return sessionIDs, nil
}

func (f *FlareSolverr) RetrieveSession() string {
	sessions, err := f.ListSessions()
	if err != nil {
		return ""
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found, creating a new one")
		return f.CreateSession()
	}

	return sessions[0]
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

func (f *FlareSolverr) Get(url string) (io.ReadCloser, error) {
	session := f.RetrieveSession()
	body := map[string]string{"cmd": "request.get", "url": url, "maxTimeout": fmt.Sprintf("%d", f.maxTimeout), "session": session}
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

	// parse the response
	var response Response
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, err
	}

	// check if the response was successful
	if response.Status != "ok" {
		return nil, fmt.Errorf("failed to get response: %s", response.Message)
	}

	// check if string "Under attack" is in the response
	if strings.Contains(response.Solution.Response, "Under attack") {
		return nil, fmt.Errorf("under attack")
	}

	// return the response body
	return io.NopCloser(bytes.NewReader([]byte(response.Solution.Response))), nil
}
