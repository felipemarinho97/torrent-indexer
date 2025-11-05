package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var vacaTorrent = IndexerMeta{
	Label:       "vaca_torrent",
	URL:         "https://vacatorrentmov.com/",
	SearchURL:   "wp-admin/admin-ajax.php",
	PagePattern: "page/%s",
}

func (i *Indexer) HandlerVacaTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := vacaTorrent

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	// supported query params: q, season, episode, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	if page == "" {
		page = "1"
	}

	var doc *goquery.Document
	var err error
	var targetURL string

	if q != "" {
		// Perform POST request to WordPress AJAX endpoint
		targetURL = fmt.Sprintf("%s%s", metadata.URL, metadata.SearchURL)
		doc, err = postSearchVacaTorrent(ctx, i, targetURL, q, page)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				logging.ErrorWithRequest(r).Err(err).Msg("Failed to encode error response")
			}
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}
	} else {
		// For home page or pagination
		targetURL = metadata.URL
		if page != "" && page != "1" {
			targetURL = fmt.Sprintf(fmt.Sprintf("%s%s", targetURL, metadata.PagePattern), page)
		}

		logging.InfoWithRequest(r).Str("target_url", targetURL).Msg("Processing indexer request")
		resp, err := i.requester.GetDocument(ctx, targetURL)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				logging.ErrorWithRequest(r).Err(err).Msg("Failed to encode error response")
			}
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}
		defer resp.Close()

		doc, err = goquery.NewDocumentFromReader(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				logging.ErrorWithRequest(r).Err(err).Msg("Failed to encode error response")
			}
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}
	}

	var links []string

	// For home page: find article links in .grid-home
	selector := ".i-tem_ht"
	doc.Find(selector).Each(func(_ int, s *goquery.Selection) {
		link, exists := s.Find("a").Attr("href")
		if exists {
			links = append(links, link)
		}
	})

	// if no links were indexed, expire the document in cache
	if len(links) == 0 {
		_ = i.requester.ExpireDocument(ctx, targetURL)
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsVacaTorrent(ctx, i, link, targetURL)
	})

	// Apply post-processors
	postProcessedTorrents := indexedTorrents
	for _, processor := range i.postProcessors {
		postProcessedTorrents = processor(i, r, postProcessedTorrents)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(Response{
		Results: postProcessedTorrents,
		Count:   len(postProcessedTorrents),
	})
	if err != nil {
		logging.Error().Err(err).Msg("Failed to encode response")
	}
}

// VacaTorrentAjaxResponse represents the JSON response from the WordPress AJAX endpoint
type VacaTorrentAjaxResponse struct {
	HTML        string `json:"html"`
	Total       int    `json:"total"`
	Pages       int    `json:"pages"`
	CountTodos  int    `json:"count_todos"`
	CountFilme  int    `json:"count_filme"`
	CountSerie  int    `json:"count_serie"`
	CountHQs    int    `json:"count_hqs"`
	CountMangas int    `json:"count_mangas"`
}

func postSearchVacaTorrent(ctx context.Context, i *Indexer, targetURL, query, page string) (*goquery.Document, error) {
	// Create multipart form data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add form fields
	_ = writer.WriteField("action", "filtrar_busca")
	_ = writer.WriteField("s", query)
	_ = writer.WriteField("tipo", "todos")
	_ = writer.WriteField("paged", page)

	err := writer.Close()
	if err != nil {
		return nil, err
	}

	// Create POST request
	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, body)
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Origin", "https://vacatorrentmov.com")
	req.Header.Set("Referer", fmt.Sprintf("https://vacatorrentmov.com/?s=%s&lang=en-US", query))

	// Execute request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var ajaxResp VacaTorrentAjaxResponse
	err = json.Unmarshal(bodyBytes, &ajaxResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Unescape HTML entities from JSON
	unescapedHTML := html.UnescapeString(ajaxResp.HTML)

	// Parse HTML from JSON using goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(unescapedHTML))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc, nil
}

func getTorrentsVacaTorrent(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	// Extract title from .custom-main-title or h1
	title := strings.TrimSpace(doc.Find(".custom-main-title").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	// Remove release date from title if present
	title = strings.TrimSpace(strings.Split(title, "(")[0])

	// Extract metadata from the list items
	var year string
	var imdbLink string
	var audio []schema.Audio
	var size []string
	var season string
	var date time.Time

	doc.Find(".col-left ul li, .content p").Each(func(_ int, s *goquery.Selection) {
		text := s.Text()
		html, _ := s.Html()

		// Extract Year
		if year == "" {
			year = findYearFromText(text, title)
		}

		// Extract link
		if imdbLink == "" {
			s.Find("a").Each(func(_ int, link *goquery.Selection) {
				href, exists := link.Attr("href")
				if exists && strings.Contains(href, "imdb.com") {
					_imdbLink, err := getIMDBLink(href)
					if err == nil {
						imdbLink = _imdbLink
					}
				}
			})
		}

		// Extract Audio/Languages
		if len(audio) == 0 {
			audio = append(audio, findAudioFromText(text)...)
		}

		// Extract Season
		if strings.Contains(text, "Season:") || strings.Contains(text, "Temporada:") {
			seasonMatch := regexp.MustCompile(`(\d+)`).FindStringSubmatch(text)
			if len(seasonMatch) > 0 {
				season = seasonMatch[1]
			}
		}

		// Extract Release Date
		if date.IsZero() {
			date = getPublishedDateFromRawString(text)
		}

		// Extract sizes from text
		size = append(size, findSizesFromText(html)...)
	})

	if date.Year() == 0 {
		yearInt, _ := strconv.Atoi(year)
		date = time.Date(yearInt, date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	}

	// Extract magnet links
	var magnetLinks []string
	doc.Find("a[href^=\"magnet\"]").Each(func(_ int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	size = utils.StableUniq(size)

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, magnetLink := range magnetLinks {
		it := it
		go func(it int, magnetLink string) {
			magnet, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				logging.Error().Err(err).Str("magnet_link", magnetLink).Msg("Failed to parse magnet URI")
				chanIndexedTorrent <- schema.IndexedTorrent{}
				return
			}
			releaseTitle := magnet.DisplayName
			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			magnetAudio := getAudioFromTitle(releaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				logging.Error().Err(err).Str("info_hash", infoHash).Msg("Failed to get leechers and seeders")
			}

			processedTitle := processVacaTorrentTitle(title, magnetAudio, season)

			// if the number of sizes is equal to the number of magnets, then assign the size to each indexed torrent in order
			var mySize string
			if len(size) == len(magnetLinks) {
				mySize = size[it]
			}
			if mySize == "" {
				go func() {
					_, _ = i.magnetMetadataAPI.FetchMetadata(ctx, magnetLink)
				}()
			}

			ixt := schema.IndexedTorrent{
				Title:         releaseTitle,
				OriginalTitle: processedTitle,
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
		}(it, magnetLink)
	}

	for i := 0; i < len(magnetLinks); i++ {
		it := <-chanIndexedTorrent
		// Skip empty torrents (failed to parse)
		if it.InfoHash != "" {
			indexedTorrents = append(indexedTorrents, it)
		}
	}

	return indexedTorrents, nil
}

func processVacaTorrentTitle(title string, audio []schema.Audio, season string) string {
	// Remove common suffixes
	title = strings.Replace(title, " – Download", "", -1)
	title = strings.Replace(title, " - Download", "", -1)
	title = strings.TrimSpace(title)

	// Add season if present
	if season != "" {
		title = fmt.Sprintf("%s S%s - %sª Temporada", title, fmt.Sprintf("%02s", season), season)
	}

	// Add audio ISO 639-2 code to title between ()
	title = appendAudioISO639_2Code(title, audio)

	return title
}
