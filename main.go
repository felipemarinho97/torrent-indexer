package main

import (
	"net/http"

	handler "github.com/felipemarinho97/vercel-lambdas/api"
	"github.com/felipemarinho97/vercel-lambdas/api/indexers"
	"github.com/felipemarinho97/vercel-lambdas/api/statusinvest"
)

func main() {
	http.HandleFunc("/", handler.HandlerIndex)
	http.HandleFunc("/statusinvest/companies", statusinvest.HandlerListCompanies)
	http.HandleFunc("/indexers/comando_torrents", indexers.HandlerComandoIndexer)

	err := http.ListenAndServe(":7006", nil)
	if err != nil {
		panic(err)
	}
}
