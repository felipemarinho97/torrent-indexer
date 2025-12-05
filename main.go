package main

import (
	"net/http"
	"os"
	"strconv"
	"time"

	handler "github.com/felipemarinho97/torrent-indexer/api"
	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/public"
	"github.com/felipemarinho97/torrent-indexer/requester"
	meilisearch "github.com/felipemarinho97/torrent-indexer/search"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	str2duration "github.com/xhit/go-str2duration/v2"
)

func main() {
	// Initialize logging first
	logging.InitLogger()

	redis := cache.NewRedis()
	searchIndex := meilisearch.NewSearchIndexer(os.Getenv("MEILISEARCH_ADDRESS"), os.Getenv("MEILISEARCH_KEY"), "torrents")
	var magnetMetadataAPI *magnet.MetadataClient
	if os.Getenv("MAGNET_METADATA_API_ENABLED") == "true" {
		timeout := 10 * time.Second
		if v := os.Getenv("MAGNET_METADATA_API_TIMEOUT_SECONDS"); v != "" {
			if t, err := strconv.Atoi(v); err == nil {
				timeout = time.Duration(t) * time.Second
			}
		}
		magnetMetadataAPI = magnet.NewClient(os.Getenv("MAGNET_METADATA_API_ADDRESS"), timeout, redis)
	}
	metrics := monitoring.NewMetrics()
	metrics.Register()

	timeoutFlaresolverrMilli := 30000
	if v := os.Getenv("FLARESOLVERR_TIMEOUT_SECONDS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			timeoutFlaresolverrMilli = t * 1000
		}
	}

	flaresolverr := requester.NewFlareSolverr(os.Getenv("FLARESOLVERR_ADDRESS"), timeoutFlaresolverrMilli)
	req := requester.NewRequester(flaresolverr, redis)

	// get shot-lived and long-lived cache expiration from env
	shortLivedCacheExpiration, err := str2duration.ParseDuration(os.Getenv("SHORT_LIVED_CACHE_EXPIRATION"))
	if err == nil {
		logging.Info().Dur("expiration", shortLivedCacheExpiration).Msg("Setting short-lived cache expiration")
		req.SetShortLivedCacheExpiration(shortLivedCacheExpiration)
	}
	longLivedCacheExpiration, err := str2duration.ParseDuration(os.Getenv("LONG_LIVED_CACHE_EXPIRATION"))
	if err == nil {
		logging.Info().Dur("expiration", longLivedCacheExpiration).Msg("Setting long-lived cache expiration")
		redis.SetDefaultExpiration(longLivedCacheExpiration)
	} else {
		logging.Error().Err(err).Msg("Failed to parse long-lived cache expiration")
	}

	icfg := handler.IndexersConfig{
		FallbackTitleEnabled: os.Getenv("FALLBACK_TITLE_ENABLED") == "true",
	}

	indexers := handler.NewIndexers(icfg, redis, metrics, req, searchIndex, magnetMetadataAPI)
	search := handler.NewMeilisearchHandler(searchIndex)

	indexerMux := http.NewServeMux()
	metricsMux := http.NewServeMux()

	indexerMux.HandleFunc("/", handler.HandlerIndex)
	indexerMux.HandleFunc("/indexers/bludv", indexers.HandlerBluDVIndexer)
	indexerMux.HandleFunc("/indexers/comando_torrents", indexers.HandlerComandoIndexer)
	indexerMux.HandleFunc("/indexers/filme_torrent", indexers.HandlerFilmeTorrentIndexer)
	indexerMux.HandleFunc("/indexers/rede_torrent", indexers.HandlerRedeTorrentIndexer)
	indexerMux.HandleFunc("/indexers/starck-filmes", indexers.HandlerStarckFilmesIndexer)
	indexerMux.HandleFunc("/indexers/torrent-dos-filmes", indexers.HandlerTorrentDosFilmesIndexer)
	indexerMux.HandleFunc("/indexers/vaca_torrent", indexers.HandlerVacaTorrentIndexer)
	indexerMux.HandleFunc("/indexers/manual", indexers.HandlerManualIndexer)
	indexerMux.HandleFunc("/search", search.SearchTorrentHandler)
	indexerMux.HandleFunc("/search/health", search.HealthHandler)
	indexerMux.HandleFunc("/search/stats", search.StatsHandler)
	indexerMux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.FS(public.UIFiles))))

	loggedIndexerMux := logging.HTTPLoggingMiddleware(indexerMux)

	metricsMux.Handle("/metrics", promhttp.Handler())

	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "8081"
	}

	go func() {
		err := http.ListenAndServe(":"+metricsPort, metricsMux)
		if err != nil {
			panic(err)
		}
	}()

	port := os.Getenv("PORT")
	if port == "" {
		port = "7006"
	}

	logging.Info().Str("port", port).Msg("Server listening")
	err = http.ListenAndServe(":"+port, loggedIndexerMux)
	if err != nil {
		logging.Fatal().Err(err).Msg("Server failed to start")
	}
}
