package engine_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
	"gopkg.in/yaml.v3"

	"github.com/felipemarinho97/torrent-indexer/indexers/engine"
)

// TestBundledDefinitions parses and validates every bundled YAML definition
// to catch structural errors without requiring network access.
func TestBundledDefinitions(t *testing.T) {
	validate := validator.New()

	entries, err := fs.ReadDir(engine.BundledDefinitions, "definitions")
	if err != nil {
		t.Fatalf("reading bundled definitions: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("no bundled definitions found")
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		t.Run(e.Name(), func(t *testing.T) {
			data, err := engine.BundledDefinitions.ReadFile("definitions/" + e.Name())
			if err != nil {
				t.Fatalf("reading %s: %v", e.Name(), err)
			}

			var def engine.IndexerDefinition
			if err := yaml.Unmarshal(data, &def); err != nil {
				t.Fatalf("parsing %s: %v", e.Name(), err)
			}

			// Skip template definitions (no URL = not active).
			if def.URL == "" {
				t.Skipf("skipping template definition %s (no url)", e.Name())
			}

			if err := validate.Struct(def); err != nil {
				t.Errorf("validation failed: %v", err)
			}
		})
	}
}
