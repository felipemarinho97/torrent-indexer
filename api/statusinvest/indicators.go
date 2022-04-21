package statusinvest

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Response struct {
	Success bool                                `json:"success"`
	Data    map[string][]map[string]interface{} `json:"data"`
}

type ParsedResponse map[string]interface{}

func HandlerIndicators(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Method not allowed"))
		return
	}
	time := r.URL.Query().Get("time")
	if time == "" {
		time = "7"
	}
	ticker := r.URL.Query().Get("ticker")
	if ticker == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Ticker is required"))
		return
	}

	data := url.Values{
		"codes[]":    strings.Split(ticker, ","),
		"time":       {time},
		"byQuarter":  {"false"},
		"futureData": {"false"},
	}

	resp, err := http.PostForm("https://statusinvest.com.br/acao/indicatorhistoricallist", data)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	out, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	var indicators Response
	err = json.Unmarshal(out, &indicators)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	parsedResponse := ParsedResponse{}

	d := indicators.Data[ticker]

	for _, v := range d {
		v := v
		parsedResponse[v["key"].(string)] = v
	}

	out, err = json.Marshal(parsedResponse)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write(out)
}
