package handler

import (
	"net/http"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/felipemarinho97/torrent-indexer/schema"
)

// ApplyPostProcessors runs all registered post-processors on the given torrents.
// This is used by the generic YAML-driven engine.
func (i *Indexer) ApplyPostProcessors(r *http.Request, torrents []schema.IndexedTorrent) []schema.IndexedTorrent {
	for _, processor := range i.postProcessors {
		torrents = processor(i, r, torrents)
	}
	return torrents
}

// FindAudioFromText is the exported counterpart of findAudioFromText.
func FindAudioFromText(text string) []schema.Audio {
	return findAudioFromText(text)
}

// GetSeparator is the exported counterpart of getSeparator.
func GetSeparator(text string) string {
	return getSeparator(text)
}

// FindSizesFromText is the exported counterpart of findSizesFromText.
func FindSizesFromText(text string) []string {
	return findSizesFromText(text)
}

// GetPublishedDateFromMeta is the exported counterpart of getPublishedDateFromMeta.
func GetPublishedDateFromMeta(doc *goquery.Document) time.Time {
	return getPublishedDateFromMeta(doc)
}

// GetAudioFromTitle is the exported counterpart of getAudioFromTitle.
func GetAudioFromTitle(releaseTitle string, audioFromContent []schema.Audio) []schema.Audio {
	return getAudioFromTitle(releaseTitle, audioFromContent)
}

// ProcessTitle is the exported counterpart of processTitle.
func ProcessTitle(title string, a []schema.Audio) string {
	return processTitle(title, a)
}

// FindYearFromText is the exported counterpart of findYearFromText.
func FindYearFromText(text, title string) string {
	return findYearFromText(text, title)
}

// GetIMDBLink is the exported counterpart of getIMDBLink.
// It accepts either a direct imdb.com URL or an indirect URL containing
// "imdbid-NNNNN" (e.g. opensubtitles.org) and returns a canonical IMDB URL.
func GetIMDBLink(link string) (string, error) {
	return getIMDBLink(link)
}

func ParseComandoDate(datePublished string) (time.Time, error) {
	return parseLocalizedDate(datePublished)
}
