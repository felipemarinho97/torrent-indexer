package custom

import (
	"net/http"

	handler "github.com/felipemarinho97/torrent-indexer/api"
	"github.com/felipemarinho97/torrent-indexer/indexers/engine"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

// adapter wraps an existing http.HandlerFunc-based indexer into the Engine interface
// without touching the original implementation file.
type adapter struct {
	id      string
	label   string
	handler http.HandlerFunc
}

func (a *adapter) ID() string                { return a.id }
func (a *adapter) Label() string             { return a.label }
func (a *adapter) Handler() http.HandlerFunc { return a.handler }

func init() {
	funcMappings := map[string]engine.ParseFunc{
		"starck_data_u":     utils.DecodeStarckDataU,
		"bludv_adware_link": utils.DecodeBludvAdwareLink,
		"1337x_date":        utils.Parse1337xDate,
		"comando_date": func(s string) (string, error) {
			t, err := handler.ParseComandoDate(s)
			if err != nil {
				return "", err
			}
			return t.Format("2006-01-02T15:04:05Z"), nil
		},
	}

	for name, fn := range funcMappings {
		engine.RegisterParseFunc(name, fn)
	}
}

// RegisterCustomIndexers registers all hand-coded indexers into the registry using the adapter.
// These are indexers whose logic is too complex to express in YAML.
func RegisterCustomIndexers(reg *engine.Registry, i *handler.Indexer) {
	reg.Register(&adapter{"starck-filmes", "starck_filmes", i.HandlerStarckFilmesIndexer})
	reg.Register(&adapter{"vaca_torrent", "vaca_torrent", i.HandlerVacaTorrentIndexer})
	reg.Register(&adapter{"comando_torrents", "comando", i.HandlerComandoIndexer})
	reg.Register(&adapter{"rede_torrent", "rede_torrent", i.HandlerRedeTorrentIndexer})
	reg.Register(&adapter{"bludv", "bludv", i.HandlerBluDVIndexer})
	reg.Register(&adapter{"torrent-dos-filmes", "torrent_dos_filmes", i.HandlerTorrentDosFilmesIndexer})
	reg.Register(&adapter{"manual", "manual", i.HandlerManualIndexer})
}
