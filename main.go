package main

import (
	"fmt"
	"maps"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"

	handler "github.com/felipemarinho97/torrent-indexer/api"
	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/indexers/custom"
	"github.com/felipemarinho97/torrent-indexer/indexers/engine"
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

	flaresolverrPoolSize := 5
	if v := os.Getenv("FLARESOLVERR_POOL_SIZE"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			flaresolverrPoolSize = t
		}
	}

	flaresolverr := requester.NewFlareSolverr(os.Getenv("FLARESOLVERR_ADDRESS"), timeoutFlaresolverrMilli, flaresolverrPoolSize)

	timeoutRequester := 5000 * time.Millisecond
	if v := os.Getenv("REQUEST_TIMEOUT_MILLISECONDS"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			timeoutRequester = time.Duration(t) * time.Millisecond
		}
	}
	req := requester.NewRequester(flaresolverr, redis, timeoutRequester)

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
	}

	icfg := handler.IndexersConfig{
		FallbackTitleEnabled: os.Getenv("FALLBACK_TITLE_ENABLED") == "true",
	}

	indexers := handler.NewIndexers(icfg, redis, metrics, req, searchIndex, magnetMetadataAPI)
	search := handler.NewMeilisearchHandler(searchIndex)

	indexerMux := http.NewServeMux()
	metricsMux := http.NewServeMux()

	// build the indexer registry
	reg := engine.NewRegistry()
	custom.RegisterCustomIndexers(reg, indexers)

	customDefsDir := os.Getenv("INDEXER_DEFINITIONS_DIR")
	engineInstances, err := engine.Load(customDefsDir, indexers, redis, metrics, req, searchIndex, magnetMetadataAPI)
	if err != nil {
		logging.Warn().Err(err).Str("custom_defs_dir", customDefsDir).Msg("Could not load YAML indexer definitions")
	} else {
		for _, e := range engineInstances {
			reg.Register(e)
			logging.Info().Str("id", e.ID()).Msg("Registered YAML indexer")
		}
	}

	// mount all registered engines under /indexers/<id>.
	engines := reg.All()
	for id, e := range engines {
		id, e := id, e // capture loop vars
		indexerMux.HandleFunc(fmt.Sprintf("/indexers/%s", id), e.Handler())
	}

	indexerMux.HandleFunc("/", handler.HandlerIndex(slices.Collect(maps.Keys(engines))))

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
