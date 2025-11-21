package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/felipemarinho97/torrent-indexer/logging"
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
		// skip if title is empty
		if it.Title == "" {
			continue
		}
		// reprocess audio tags in case any middleware has changed the title
		torrents[i].Audio = getAudioFromTitle(it.Title, it.Audio)

		// for each video file, get audio ISO639-2 code and append to title
		for _, file := range it.Files {
			// check if file is a video
			if !utils.IsVideoFile(file.Path) {
				continue
			}
			torrents[i].Audio = getAudioFromTitle(file.Path, torrents[i].Audio)
		}

		torrents[i].Title = appendAudioISO639_2Code(torrents[i].Title, torrents[i].Audio)
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
		logging.Debug().Str("info_hash", m.InfoHash).Str("size", it.Size).Msg("Retrieved metadata from MagnetMetadataAPI")

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

		// If "date" is zero, use the date from metadata if available
		if it.Date.IsZero() {
			it.Date = m.CreatedAt
		}

		return []schema.IndexedTorrent{it}, nil
	})
}

func FallbackPostTitle(i *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	emptyTitles := 0

	for idx := range torrents {
		if torrents[idx].Title == "" {
			if i.config.FallbackTitleEnabled {
				torrents[idx].Title = fmt.Sprintf("[UNSAFE] %s", torrents[idx].OriginalTitle)
			} else {
				emptyTitles++
			}
		}
	}

	if emptyTitles > 0 && !i.magnetMetadataAPI.IsEnabled() {
		logging.WarnWithRequest(r).
			Int("empty_titles", emptyTitles).
			Msg("Some torrents have empty titles. Consider setting up MAGNET_METADATA_API (recommended) or set FALLBACK_TITLE_ENABLED=true.")
	}

	return torrents
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

// ApplyLimit limits the number of results based on the "limit" query parameter
func ApplyLimit(_ *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		return torrents
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return torrents
	}

	if len(torrents) > limit {
		return torrents[:limit]
	}

	return torrents
}

// ApplySorting sorts the results based on "sortBy" and "sortDirection" query parameters
func ApplySorting(_ *Indexer, r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	sortBy := r.URL.Query().Get("sortBy")
	if sortBy == "" {
		return torrents
	}

	sortDirection := r.URL.Query().Get("sortDirection")
	ascending := sortDirection == "asc"

	slices.SortFunc(torrents, func(i, j schema.IndexedTorrent) int {
		var cmp int
		switch sortBy {
		case "title":
			cmp = strings.Compare(strings.ToLower(i.Title), strings.ToLower(j.Title))
		case "original_title":
			cmp = strings.Compare(strings.ToLower(i.OriginalTitle), strings.ToLower(j.OriginalTitle))
		case "year":
			cmp = strings.Compare(i.Year, j.Year)
		case "date":
			if i.Date.Before(j.Date) {
				cmp = -1
			} else if i.Date.After(j.Date) {
				cmp = 1
			}
		case "seed_count", "seeders":
			cmp = i.SeedCount - j.SeedCount
		case "leech_count", "leechers":
			cmp = i.LeechCount - j.LeechCount
		case "size":
			// Parse size strings to bytes for accurate comparison
			iBytes := utils.ParseSize(i.Size)
			jBytes := utils.ParseSize(j.Size)
			if iBytes < jBytes {
				cmp = -1
			} else if iBytes > jBytes {
				cmp = 1
			}
		case "similarity":
			if i.Similarity < j.Similarity {
				cmp = -1
			} else if i.Similarity > j.Similarity {
				cmp = 1
			}
		default:
			return 0
		}

		if ascending {
			return cmp
		}
		return -cmp
	})

	return torrents
}
