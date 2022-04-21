package statusinvest

import (
	"io/ioutil"
	"net/http"
)

func HandlerListCompanies(w http.ResponseWriter, r *http.Request) {
	client := http.Client{}
	req, err := http.NewRequest("GET", "https://statusinvest.com.br/acao/companiesnavigation?page=1&size=500", nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_14_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.149 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := client.Do(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Write(body)
}
