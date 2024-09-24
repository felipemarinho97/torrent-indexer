package main

import (
	"fmt"
	"net/http"
	"os"

	handler "github.com/felipemarinho97/torrent-indexer/api"
	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/requester"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	redis := cache.NewRedis()
	metrics := monitoring.NewMetrics()
	metrics.Register()

	flaresolverr := requester.NewFlareSolverr(os.Getenv("FLARESOLVERR_ADDRESS"), 60000)
	req := requester.NewRequester(flaresolverr, redis)
	indexers := handler.NewIndexers(redis, metrics, req)

	indexerMux := http.NewServeMux()
	metricsMux := http.NewServeMux()

	indexerMux.HandleFunc("/", handler.HandlerIndex)
	indexerMux.HandleFunc("/indexers/comando_torrents", indexers.HandlerComandoIndexer)
	indexerMux.HandleFunc("/indexers/bludv", indexers.HandlerBluDVIndexer)
	indexerMux.HandleFunc("/indexers/manual", indexers.HandlerManualIndexer)

	metricsMux.Handle("/metrics", promhttp.Handler())

	go func() {
		err := http.ListenAndServe(":8081", metricsMux)
		if err != nil {
			panic(err)
		}
	}()
	fmt.Println("Server listening on :7006")
	err := http.ListenAndServe(":7006", indexerMux)
	if err != nil {
		panic(err)
	}
}
