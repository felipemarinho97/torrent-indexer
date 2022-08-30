package quotation

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

type RawResponse struct {
	BizSts struct {
		Cd string `json:"cd"`
	} `json:"BizSts"`
	Msg struct {
		DtTm string `json:"dtTm"`
	} `json:"Msg"`
	Trad []struct {
		Scty struct {
			SctyQtn struct {
				OpngPric float64 `json:"opngPric"`
				MinPric  float64 `json:"minPric"`
				MaxPric  float64 `json:"maxPric"`
				AvrgPric float64 `json:"avrgPric"`
				CurPrc   float64 `json:"curPrc"`
				PrcFlcn  float64 `json:"prcFlcn"`
			} `json:"SctyQtn"`
			Mkt struct {
				Nm string `json:"nm"`
			} `json:"mkt"`
			Symb        string `json:"symb"`
			Desc        string `json:"desc"`
			IndxCmpnInd bool   `json:"indxCmpnInd"`
		} `json:"scty"`
		TTLQty int `json:"ttlQty"`
	} `json:"Trad"`
}

type Response struct {
	Symbol                  string  `json:"symbol"`
	Name                    string  `json:"name"`
	Market                  string  `json:"market"`
	OpeningPrice            float64 `json:"openingPrice"`
	MinPrice                float64 `json:"minPrice"`
	MaxPrice                float64 `json:"maxPrice"`
	AveragePrice            float64 `json:"averagePrice"`
	CurrentPrice            float64 `json:"currentPrice"`
	PriceVariation          float64 `json:"priceVariation"`
	IndexComponentIndicator bool    `json:"indexComponentIndicator"`
}

type RawErrorResponse struct {
	BizSts struct {
		Cd   string `json:"cd"`
		Desc string `json:"desc"`
	} `json:"BizSts"`
	Msg struct {
		DtTm string `json:"dtTm"`
	} `json:"Msg"`
}

type Error struct {
	Message string `json:"message"`
}

func HandlerListCompanies(w http.ResponseWriter, r *http.Request) {
	ticker := strings.Split(r.URL.Path, "/")[4]
	log.Info("Getting quotation info for ticker: " + ticker)
	if ticker == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Ticker is required"))
		return
	}

	client := http.Client{}
	res, err := client.Get(fmt.Sprintf("https://cotacao.b3.com.br/mds/api/v1/instrumentQuotation/%s", ticker))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	defer res.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	// add 1min cache header
	w.Header().Set("Cache-Control", "max-age=60, public")

	out, err := ioutil.ReadAll(res.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	var raw RawResponse
	err = json.Unmarshal(out, &raw)
	if err != nil || raw.BizSts.Cd != "OK" {
		var errorResponse RawErrorResponse
		err = json.Unmarshal(out, &errorResponse)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}

		// 404
		w.WriteHeader(http.StatusNotFound)
		formatedError := Error{Message: errorResponse.BizSts.Desc}
		err := json.NewEncoder(w).Encode(formatedError)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		return
	}

	var response []Response
	for _, trad := range raw.Trad {
		response = append(response, Response{
			Symbol:                  trad.Scty.Symb,
			Name:                    trad.Scty.Desc,
			Market:                  trad.Scty.Mkt.Nm,
			OpeningPrice:            trad.Scty.SctyQtn.OpngPric,
			MinPrice:                trad.Scty.SctyQtn.MinPric,
			MaxPrice:                trad.Scty.SctyQtn.MaxPric,
			AveragePrice:            trad.Scty.SctyQtn.AvrgPric,
			CurrentPrice:            trad.Scty.SctyQtn.CurPrc,
			PriceVariation:          trad.Scty.SctyQtn.PrcFlcn,
			IndexComponentIndicator: trad.Scty.IndxCmpnInd,
		})
	}

	err = json.NewEncoder(w).Encode(response[0])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	return
}
