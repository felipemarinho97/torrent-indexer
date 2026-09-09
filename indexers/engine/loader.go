package engine

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	handler "github.com/felipemarinho97/torrent-indexer/api"
	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/requester"
	meilisearch "github.com/felipemarinho97/torrent-indexer/search"
	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"
)

//go:embed definitions/*.yaml
var BundledDefinitions embed.FS

var validate = validator.New()

// Load reads all bundled definitions first, then overlays any *.yaml files
// found in customDir (if non-empty). A file in customDir whose base name matches
// a bundled file replaces it. Returns a slice of Engines ready for registration.
func Load(
	customDir string,
	indexer *handler.Indexer,
	redis *cache.Redis,
	metrics *monitoring.Metrics,
	req *requester.Requster,
	si *meilisearch.SearchIndexer,
	magnetAPI *magnet.MetadataClient,
) ([]Engine, error) {
	parseAndAdd := func(rawDefs map[string]IndexerDefinition, name string, data []byte, warnOnOverride bool) {
		var def IndexerDefinition
		if err := yaml.Unmarshal(data, &def); err != nil {
			logging.Warn().Err(err).Str("file", name).Msg("Skipping definition: failed to parse YAML")
			return
		}
		if err := validate.Struct(def); err != nil {
			logging.Warn().Err(err).Str("file", name).Msg("Skipping definition: validation failed")
			return
		}
		if warnOnOverride {
			if _, exists := rawDefs[def.ID]; exists {
				logging.Warn().Str("id", def.ID).Str("file", name).Msg("Bundled definition overridden by custom definition")
			}
		}
		rawDefs[def.ID] = def
	}

	rawDefs := make(map[string]IndexerDefinition)

	// 1. Parse bundled definitions.
	entries, err := fs.ReadDir(BundledDefinitions, "definitions")
	if err != nil {
		return nil, fmt.Errorf("reading bundled definitions: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := BundledDefinitions.ReadFile("definitions/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading bundled %s: %w", e.Name(), err)
		}
		parseAndAdd(rawDefs, e.Name(), data, false)
	}

	// 2. Parse and overlay user-provided definitions (logged when overriding by ID).
	if customDir != "" {
		dirEntries, err := os.ReadDir(customDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading extra definitions dir %q: %w", customDir, err)
		}
		for _, e := range dirEntries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(customDir, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", e.Name(), err)
			}
			parseAndAdd(rawDefs, e.Name(), data, true)
		}
	}

	// Build engines, applying env var URL overrides and skipping template definitions.
	var engines []Engine
	for _, def := range rawDefs {
		resolved := def

		envKey := fmt.Sprintf("INDEXER_%s_URL", strings.ToUpper(strings.ReplaceAll(resolved.ID, "-", "_")))
		if v := os.Getenv(envKey); v != "" {
			resolved.URL = v
		}

		if resolved.URL == "" {
			continue
		}

		engines = append(engines, &genericEngine{
			def:       resolved,
			indexer:   indexer,
			redis:     redis,
			metrics:   metrics,
			req:       req,
			si:        si,
			magnetAPI: magnetAPI,
		})
	}
	return engines, nil
}
