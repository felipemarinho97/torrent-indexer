package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
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
	
	// Configure searchable attributes asynchronously
	go func() {
		for i := 0; i < 10; i++ {
			if searchIndex.IsHealthy() {
				err := searchIndex.UpdateSearchableAttributes([]string{"imdb", "title", "original_title", "context", "files.path"})
				if err != nil {
					logging.Error().Err(err).Msg("Failed to update searchable attributes in Meilisearch")
				} else {
					logging.Info().Msg("Successfully updated searchable attributes in Meilisearch")
				}

				err = searchIndex.UpdateSynonyms(map[string][]string{
					"the": {"o", "a", "os", "as"},
					"o":   {"the"},
					"a":   {"the"},
					"os":  {"the"},
					"as":  {"the"},
					"do":  {"of", "from"},
					"da":  {"of", "from"},
					"de":  {"of"},
					"of":  {"do", "da", "de"},
				})
				if err != nil {
					logging.Error().Err(err).Msg("Failed to update synonyms in Meilisearch")
				} else {
					logging.Info().Msg("Successfully updated synonyms in Meilisearch")
				}
				break
			}
			time.Sleep(5 * time.Second)
		}
	}()

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
	} else {
		logging.Error().Err(err).Msg("Failed to parse long-lived cache expiration")
	}

	disabledIndexers := make(map[string]bool)
	if disabledEnv := os.Getenv("TORRENT_INDEXER_DISABLED_INDEXERS"); disabledEnv != "" {
		for _, name := range strings.Split(disabledEnv, ",") {
			disabledIndexers[strings.TrimSpace(name)] = true
		}
	}

	icfg := handler.IndexersConfig{
		FallbackTitleEnabled: os.Getenv("FALLBACK_TITLE_ENABLED") == "true",
		DisabledIndexers:     disabledIndexers,
	}

	indexers := handler.NewIndexers(icfg, redis, metrics, req, searchIndex, magnetMetadataAPI)
	search := handler.NewMeilisearchHandler(searchIndex)

	indexerMux := http.NewServeMux()
	metricsMux := http.NewServeMux()

	indexerMux.HandleFunc("/", indexers.HandlerIndex)
	indexerMux.HandleFunc("/indexers/all", handler.WithMultiQuery(indexers.HandlerMultiIndexers))
	indexerMux.HandleFunc("/indexers/search", handler.WithMultiQuery(indexers.HandlerMultiIndexers))
	indexerMux.HandleFunc("/indexers/manual", handler.WithMultiQuery(indexers.HandlerManualIndexer))

	// Register all configured indexers dynamically
	for name, h := range indexers.GetAllHandlersMap() {
		indexerMux.HandleFunc("/indexers/"+name, handler.WithMultiQuery(h))
	}
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
