package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/felipemarinho97/torrent-indexer/schema"
	"github.com/felipemarinho97/torrent-indexer/utils"
	"github.com/hbollon/go-edlib"
)

// CleanupTitleWebsites removes unwanted characters from the title
func CleanupTitleWebsites(_ *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	for i := range torrents {
		torrents[i].Title = utils.RemoveKnownWebsites(torrents[i].Title)
	}
	return torrents
}

func AppendAudioTags(_ *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	for i, it := range torrents {
		torrents[i].Title = appendAudioISO639_2Code(torrents[i].Title, it.Audio)
	}

	return torrents
}

// SendToSearchIndexer sends the indexed torrents to the search indexer
func SendToSearchIndexer(i *Indexer, _ *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	go func() {
		_ = i.search.IndexTorrents(torrents)
	}()
	return torrents
}

// FullfilMissingMetadata fills in missing metadata for indexed torrents
func FullfilMissingMetadata(i *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	if !i.magnetMetadataAPI.IsEnabled() {
		return torrents
	}

	return utils.ParallelFlatMap(torrents, func(it schema.IndexedTorrent) ([]schema.IndexedTorrent, error) {
		if it.Size != "" && it.Title != "" && it.OriginalTitle != "" {
			return []schema.IndexedTorrent{it}, nil
		}
		m, err := i.magnetMetadataAPI.FetchMetadata(r.Context(), it.MagnetLink)
		if err != nil {
			return []schema.IndexedTorrent{it}, nil
		}

		// convert size in bytes to a human-readable format
		it.Size = utils.FormatBytes(m.Size)

		// Use name from metadata if available as it is more accurate
		if m.Name != "" {
			it.Title = m.Name
		}
		fmt.Printf("hash: %s get -> size: %s\n", m.InfoHash, it.Size)

		// If files are present, add them to the indexed torrent
		if len(m.Files) > 0 {
			it.Files = make([]schema.File, len(m.Files))
			for i, file := range m.Files {
				it.Files[i] = schema.File{
					Path: file.Path,
					Size: utils.FormatBytes(file.Size),
				}
			}
		}

		return []schema.IndexedTorrent{it}, nil
	})
}

func AddSimilarityCheck(i *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	q := r.URL.Query().Get("q")

	for i, it := range torrents {
		jLower := strings.ReplaceAll(strings.ToLower(fmt.Sprintf("%s %s", it.Title, it.OriginalTitle)), ".", " ")
		qLower := strings.ToLower(q)
		splitLength := 2
		torrents[i].Similarity = edlib.JaccardSimilarity(jLower, qLower, splitLength)
	}

	// remove the ones with zero similarity
	if len(torrents) > 20 && r.URL.Query().Get("filter_results") != "" && r.URL.Query().Get("q") != "" {
		torrents = utils.Filter(torrents, func(it schema.IndexedTorrent) bool {
			return it.Similarity > 0
		})
	}

	// sort by similarity
	slices.SortFunc(torrents, func(i, j schema.IndexedTorrent) int {
		return int((j.Similarity - i.Similarity) * 1000)
	})

	return torrents
}
