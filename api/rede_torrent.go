package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/requester"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var rede_torrent = IndexerMeta{
	Label: "rede_torrent",
	URL:   utils.GetIndexerURLFromEnv("INDEXER_REDE_TORRENT_URL", "https://redestorrents.com/"),
	// the redesign dropped the "?s=" endpoint; search goes through
	// searchRedeTorrent, which needs a session handshake SearchURL can't express
	PagePattern: "pagina/%s/",
}

func (i *Indexer) HandlerRedeTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := rede_torrent

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	// supported query params: q, season, episode, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	var doc *goquery.Document
	var err error
	var targetURL string

	if q != "" {
		// the redesign dropped "?s=", which no longer filters: it answers every
		// query with the homepage listing. searchRedeTorrent drives the
		// handshake the real search form needs.
		targetURL = metadata.URL
		logging.InfoWithRequest(r).Str("target_url", targetURL).Str("query", q).Msg("Processing indexer request (session search)")
		doc, err = searchRedeTorrent(ctx, i.requester, metadata.URL, q)
	} else {
		listURL := metadata.URL
		if page != "" {
			listURL = fmt.Sprintf(fmt.Sprintf("%s%s", listURL, metadata.PagePattern), page)
		}
		targetURL = listURL
		logging.InfoWithRequest(r).Str("target_url", targetURL).Msg("Processing indexer request")
		var resp io.ReadCloser
		resp, err = i.requester.GetDocument(ctx, listURL)
		if err == nil {
			defer resp.Close()
			doc, err = goquery.NewDocumentFromReader(resp)
		}
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			logging.ErrorWithRequest(r).Err(err).Msg("Failed to encode error response")
		}
		i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
		return
	}

	var links []string
	// both the listing and the search results are now a grid of
	// <a class="cover-link"><article class="custom-card">…
	doc.Find("a.cover-link").Each(func(i int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if exists {
			links = append(links, link)
		}
	})

	// if no links were indexed, expire the document in cache so it is fetched
	// again next time. Only on the listing path: a search caches under its own
	// query-keyed entry, while its targetURL is the homepage — expiring that
	// would throw away the listing the other path just cached.
	if len(links) == 0 && q == "" {
		_ = i.requester.ExpireDocument(ctx, targetURL)
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsRedeTorrent(ctx, i, link, targetURL)
	})

	// Apply post-processors
	postProcessedTorrents := indexedTorrents
	for _, processor := range i.postProcessors {
		postProcessedTorrents = processor(i, r, postProcessedTorrents)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(Response{
		Results:      postProcessedTorrents,
		Count:        len(postProcessedTorrents),
		IndexedCount: len(indexedTorrents),
	})
	if err != nil {
		logging.Error().Err(err).Msg("Failed to encode response")
	}
}

func getTorrentsRedeTorrent(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	// the details page no longer has a parseable <h1> or #informacoes block, so
	// title, year and imdb come from the schema.org Movie block the new theme
	// emits for SEO, with the breadcrumb and <h1> as fallbacks.
	title, year, imdbLink := extractMovieJSONLD(doc)
	if title == "" {
		title = strings.TrimSpace(doc.Find(".breadcrumb-item.active [itemprop=\"name\"]").First().Text())
	}
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	if title == "" {
		return nil, fmt.Errorf("could not extract title from %s", link)
	}

	textContent := doc.Text()
	date := getPublishedDateFromMeta(doc)

	magnetLinks, sizeByMagnet := extractMagnetsRedeTorrent(doc)

	audio := extractAudioRedeTorrent(doc)
	if len(audio) == 0 {
		audio = findAudioFromText(textContent)
	}
	if year == "" {
		year = findYearFromText(textContent, title)
	}

	// find any link from imdb, fallback if JSON-LD didn't have "sameAs"
	if imdbLink == "" {
		doc.Find("a").EachWithBreak(func(i int, s *goquery.Selection) bool {
			href, _ := s.Attr("href")
			_imdbLink, err := getIMDBLink(href)
			if err == nil {
				imdbLink = _imdbLink
				return false
			}
			return true
		})
	}

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for _, magnetLink := range magnetLinks {
		go func(magnetLink string) {
			magnet, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				logging.Error().Err(err).Str("magnet_link", magnetLink).Msg("Failed to parse magnet URI")
			}
			releaseTitle := magnet.DisplayName
			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			magnetAudio := getAudioFromTitle(releaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				logging.Error().Err(err).Str("info_hash", infoHash).Msg("Failed to get leechers and seeders")
			}

			title := processTitle(title, magnetAudio)

			mySize := sizeByMagnet[magnetLink]
			// if the row didn't print a size, the magnet's "xl" param (exact
			// byte length) is the next best source.
			if mySize == "" && magnet.Params != nil {
				if xl := magnet.Params.Get("xl"); xl != "" {
					mySize = formatBytesToSize(xl)
				}
			}
			if mySize == "" {
				go func() {
					_, _ = i.magnetMetadataAPI.FetchMetadata(ctx, magnetLink)
				}()
			}

			ixt := schema.IndexedTorrent{
				Title:         releaseTitle,
				OriginalTitle: title,
				Details:       link,
				Year:          year,
				IMDB:          imdbLink,
				Audio:         magnetAudio,
				MagnetLink:    magnetLink,
				Date:          date,
				InfoHash:      infoHash,
				Trackers:      trackers,
				LeechCount:    peer,
				SeedCount:     seed,
				Size:          mySize,
			}
			chanIndexedTorrent <- ixt
		}(magnetLink)
	}

	for i := 0; i < len(magnetLinks); i++ {
		it := <-chanIndexedTorrent
		indexedTorrents = append(indexedTorrents, it)
	}

	return indexedTorrents, nil
}

// extractAudioRedeTorrent reads the languages off the "Idioma / Áudio" spec
// card the redesigned details page prints. The card holds the value without the
// "Idioma:" label findAudioFromText keys on, so the label is put back before
// parsing — that keeps the language table and the separator handling in the one
// place that already owns them.
func extractAudioRedeTorrent(doc *goquery.Document) []schema.Audio {
	var audio []schema.Audio
	doc.Find(".spec-text").EachWithBreak(func(i int, s *goquery.Selection) bool {
		// "Qualidade de Áudio" is a neighbouring card holding a score, so the
		// label has to match on "Idioma" rather than on "Áudio"
		if !strings.Contains(s.Find("small").Text(), "Idioma") {
			return true
		}
		audio = findAudioFromText("Idioma: " + strings.TrimSpace(s.Find("strong").Text()))
		return false
	})
	return audio
}

// extractMagnetsRedeTorrent collects the magnet links of a details page in
// document order, together with the size the page prints for each one.
//
// Sizes have to be read per magnet. Every download option sits in its own
// ".download-row", whose ".download-name" ends with that release's size
// ("1ª OPÇÃO DOWNLOAD TORRENT DUBLADO 720P / 796 MB"), while the
// page repeats one of those sizes further down in a "Tamanho do Arquivo" spec
// card. Collecting sizes from the whole document and pairing them with the
// magnets by index therefore lines the wrong size up against the wrong release.
func extractMagnetsRedeTorrent(doc *goquery.Document) ([]string, map[string]string) {
	var magnetLinks []string
	sizeByMagnet := make(map[string]string)

	doc.Find(".download-row").Each(func(i int, s *goquery.Selection) {
		magnetLink, exists := s.Find("a[href^=\"magnet\"]").First().Attr("href")
		if !exists {
			return
		}
		magnetLinks = append(magnetLinks, magnetLink)
		if sizes := findSizesFromText(s.Find(".download-name").Text()); len(sizes) > 0 {
			sizeByMagnet[magnetLink] = sizes[len(sizes)-1]
		}
	})

	// pick up any magnet that isn't inside a ".download-row"
	doc.Find("a[href^=\"magnet\"]").Each(func(i int, s *goquery.Selection) {
		if magnetLink, exists := s.Attr("href"); exists {
			magnetLinks = append(magnetLinks, magnetLink)
		}
	})
	magnetLinks = utils.StableUniq(magnetLinks)

	// some pages print no size on the download row, only the page-wide spec
	// card. That value can be attributed to a release only when the page offers
	// a single one.
	if len(magnetLinks) == 1 && sizeByMagnet[magnetLinks[0]] == "" {
		doc.Find(".spec-text").EachWithBreak(func(i int, s *goquery.Selection) bool {
			if !strings.Contains(s.Find("small").Text(), "Tamanho") {
				return true
			}
			if sizes := findSizesFromText(s.Find("strong").Text()); len(sizes) > 0 {
				sizeByMagnet[magnetLinks[0]] = sizes[0]
			}
			return false
		})
	}

	return magnetLinks, sizeByMagnet
}

// searchRedeTorrent performs the two-step handshake the redestorrents.com
// search form requires (confirmed 2026-09, after the site's redesign):
//  1. GET the homepage to obtain a PHPSESSID cookie and scrape the hidden
//     anti-bot "token" field from the search form.
//  2. GET index.php?busca=<query>&token=<token> reusing that same cookie.
//
// Both halves are checked: with either one missing, or with a token minted for
// a different session, the endpoint 302-redirects to "/". That redirect still
// renders a full page of latest releases, indistinguishable from a page of
// results, so it is detected rather than trusted.
func searchRedeTorrent(ctx context.Context, req *requester.Requster, baseURL, query string) (*goquery.Document, error) {
	session, err := req.NewSession()
	if err != nil {
		return nil, err
	}

	// the token makes every search URL single-use, so the token-free URL is what
	// the cache is keyed on
	searchURL := fmt.Sprintf("%sindex.php?busca=%s", baseURL, url.QueryEscape(query))
	if body, ok := session.Cached(ctx, searchURL); ok {
		return goquery.NewDocumentFromReader(bytes.NewReader(body))
	}

	homeBody, err := session.Get(ctx, baseURL, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to load homepage for session/token: %w", err)
	}
	homeDoc, err := goquery.NewDocumentFromReader(bytes.NewReader(homeBody))
	if err != nil {
		return nil, err
	}
	token, exists := homeDoc.Find("input[name=\"token\"]").First().Attr("value")
	if !exists || token == "" {
		return nil, fmt.Errorf("could not find search token on homepage")
	}

	searchBody, err := session.Get(ctx, fmt.Sprintf("%s&token=%s&hp_bot_check=", searchURL, token), baseURL, "")
	if err != nil {
		return nil, fmt.Errorf("failed to perform search request: %w", err)
	}
	if !strings.Contains(session.LastURL(), "busca=") {
		return nil, fmt.Errorf("search handshake rejected, redirected to %s", session.LastURL())
	}
	session.Store(ctx, searchURL, searchBody)

	return goquery.NewDocumentFromReader(bytes.NewReader(searchBody))
}

// extractMovieJSONLD reads the schema.org "Movie" JSON-LD block that the
// redestorrents.com theme emits for SEO and returns title, year and IMDB link.
// Returns empty strings for any field it can't find.
func extractMovieJSONLD(doc *goquery.Document) (title, year, imdbLink string) {
	doc.Find("script[type=\"application/ld+json\"]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		var data struct {
			Type          string `json:"@type"`
			Name          string `json:"name"`
			SameAs        string `json:"sameAs"`
			CopyrightYear string `json:"copyrightYear"`
		}
		if err := json.Unmarshal([]byte(s.Text()), &data); err != nil {
			return true // continue to next script tag
		}
		if data.Type != "Movie" {
			return true
		}
		title = strings.TrimSpace(data.Name)
		year = strings.TrimSpace(data.CopyrightYear)
		if l, err := getIMDBLink(data.SameAs); err == nil {
			imdbLink = l
		}
		return false // found it, stop
	})
	return
}

// formatBytesToSize converts a byte-length string (as found in a magnet's
// "xl" param) into a human readable "X.XX GB"/"X.XX MB" string.
func formatBytesToSize(byteStr string) string {
	bytes, err := strconv.ParseFloat(byteStr, 64)
	if err != nil || bytes <= 0 {
		return ""
	}
	const unit = 1024.0
	switch {
	case bytes >= unit*unit*unit:
		return fmt.Sprintf("%.2f GB", bytes/(unit*unit*unit))
	case bytes >= unit*unit:
		return fmt.Sprintf("%.2f MB", bytes/(unit*unit))
	default:
		return fmt.Sprintf("%.2f KB", bytes/unit)
	}
}
