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

	str2duration "github.com/xhit/go-str2duration/v2"
)

func main() {
	redis := cache.NewRedis()
	metrics := monitoring.NewMetrics()
	metrics.Register()

	flaresolverr := requester.NewFlareSolverr(os.Getenv("FLARESOLVERR_ADDRESS"), 60000)
	req := requester.NewRequester(flaresolverr, redis)

	// get shot-lived and long-lived cache expiration from env
	shortLivedCacheExpiration, err := str2duration.ParseDuration(os.Getenv("SHORT_LIVED_CACHE_EXPIRATION"))
	if err == nil {
		fmt.Printf("Setting short-lived cache expiration to %s\n", shortLivedCacheExpiration)
		req.SetShortLivedCacheExpiration(shortLivedCacheExpiration)
	}
	longLivedCacheExpiration, err := str2duration.ParseDuration(os.Getenv("LONG_LIVED_CACHE_EXPIRATION"))
	if err == nil {
		fmt.Printf("Setting long-lived cache expiration to %s\n", longLivedCacheExpiration)
		redis.SetDefaultExpiration(longLivedCacheExpiration)
	} else {
		fmt.Println(err)
	}

	indexers := handler.NewIndexers(redis, metrics, req)

	indexerMux := http.NewServeMux()
	metricsMux := http.NewServeMux()

	indexerMux.HandleFunc("/", handler.HandlerIndex)
	indexerMux.HandleFunc("/indexers/comando_torrents", indexers.HandlerComandoIndexer)
	indexerMux.HandleFunc("/indexers/torrent-dos-filmes", indexers.HandlerTorrentDosFilmesIndexer)
	indexerMux.HandleFunc("/indexers/bludv", indexers.HandlerBluDVIndexer)
	indexerMux.HandleFunc("/indexers/manual", indexers.HandlerManualIndexer)

	metricsMux.Handle("/metrics", promhttp.Handler())

	go func() {
		err := http.ListenAndServe(":8081", metricsMux)
		if err != nil {
			panic(err)
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "7006"
	}

	fmt.Printf("Server listening on :%s\n", port)
	err = http.ListenAndServe(":"+port, indexerMux)
	if err != nil {
		panic(err)
	}
}
