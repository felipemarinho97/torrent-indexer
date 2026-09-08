package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var bludv = IndexerMeta{
	Label:       "bludv",
	URL:         utils.GetIndexerURLFromEnv("INDEXER_BLUDV_URL", "https://bludv-v1.xyz/"),
	SearchURL:   "?s=",
	PagePattern: "page/%s",
}

func (i *Indexer) HandlerBluDVIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := bludv

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	// supported query params: q, season, episode, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	// URL encode query param
	q = url.QueryEscape(q)
	url := metadata.URL
	if page != "" {
		url = fmt.Sprintf(fmt.Sprintf("%s%s", url, metadata.PagePattern), page)
	} else {
		url = fmt.Sprintf("%s%s%s", url, metadata.SearchURL, q)
	}

	logging.InfoWithRequest(r).Str("target_url", url).Msg("Processing indexer request")
	resp, err := i.requester.GetDocument(ctx, url)
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

	doc, err := goquery.NewDocumentFromReader(resp)
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
	doc.Find(".post").Each(func(i int, s *goquery.Selection) {
		// get link from h2.entry-title > a
		link, _ := s.Find("div.title > a").Attr("href")
		links = append(links, link)
	})

	// if no links were indexed, expire the document in cache
	if len(links) == 0 {
		_ = i.requester.ExpireDocument(ctx, url)
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsBluDV(ctx, i, link, url)
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

func getTorrentsBluDV(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	article := doc.Find(".post")
	title := strings.Replace(article.Find(".title > h1").Text(), " - Download", "", -1)
	textContent := article.Find("div.content")
	date := getPublishedDate(doc)
	magnets := textContent.Find("a[href^=\"magnet\"]")
	var magnetLinks []ExtractedMagnet
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		ctxStr := ExtractMagnetContext(s)
		magnetLinks = append(magnetLinks, ExtractedMagnet{Link: magnetLink, Context: ctxStr})
	})

	adwareHosts := map[string]struct{}{
		"www.seuvideo.xyz":   {},
		"www.systemads.org":  {},
		"systemads.org":      {},
		"superadsgo.xyz":     {},
		"www.superadsgo.xyz": {},
		"systemads.xyz":      {},
		"www.systemads.xyz":  {},
	}

	// Process all links and check if hostname matches known ad redirect hosts
	textContent.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		parsedURL, err := url.Parse(href)
		if err != nil {
			logging.Error().Err(err).Str("href", href).Msg("Failed to parse URL")
			return
		}

		host := strings.ToLower(parsedURL.Hostname())
		if _, ok := adwareHosts[host]; !ok {
			return
		}

		magnetLink := parsedURL.Query().Get("id")
		logging.Debug().Str("encoded_id", magnetLink).Str("href", href).Msg("Attempting to decode ad link")
		magnetLinkDecoded, err := utils.DecodeAdLink(magnetLink)
		ctxStr := ExtractMagnetContext(s)
		
		pHtml, _ := s.Parent().Html()
		logging.Debug().Str("ctxStr", ctxStr).Str("href", href).Str("parentHtml", pHtml).Msg("Extracted context for link")

		// Strategy 1: Try decoding
		if err == nil && strings.HasPrefix(magnetLinkDecoded, "magnet:") {
			logging.Debug().Str("decoded", magnetLinkDecoded).Msg("Successfully decoded ad link")
			magnetLinks = append(magnetLinks, ExtractedMagnet{Link: magnetLinkDecoded, Context: ctxStr})
			return
		}

		// Strategy 2: Try resolving via HTTP HEAD to capture redirect location
		logging.Debug().Str("href", href).Msg("Decode failed, attempting to resolve via HEAD request for redirect")
		
		// Make a HEAD request to capture the redirect Location header without following it
		headReq, err := http.NewRequestWithContext(ctx, "HEAD", href, nil)
		if err == nil {
			// Create a custom client that doesn't follow redirects
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse // Don't follow redirects
				},
				Timeout: 10 * time.Second,
			}
			resp, err := client.Do(headReq)
			if err == nil && resp.StatusCode >= 300 && resp.StatusCode < 400 {
				location := resp.Header.Get("Location")
				if strings.HasPrefix(location, "magnet:") {
					logging.Debug().Str("magnet", location).Str("href", href).Msg("Extracted magnet from redirect Location header")
					magnetLinks = append(magnetLinks, ExtractedMagnet{Link: location, Context: ctxStr})
					resp.Body.Close()
					return
				}
				resp.Body.Close()
			}
		}

		// Fallback: Try resolving via FlareSolverr and look for magnet in HTML
		doc, err := getDocument(ctx, i, href, referer)
		if err != nil {
			logging.Error().Err(err).Str("href", href).Msg("Failed to resolve ad link via FlareSolverr")
			return
		}

		// Search the raw HTML for any occurrence of 'receber.php'
		htmlContent, _ := doc.Html()
		lines := strings.Split(htmlContent, "\n")
		var redirectURL string
		for _, line := range lines {
			if strings.Contains(line, "receber.php") {
				start := strings.Index(line, "https://")
				if start != -1 {
					end := strings.Index(line[start:], "\"")
					if end != -1 {
						redirectURL = line[start : start+end]
						break
					}
					// Try single quote if double quote fails
					end = strings.Index(line[start:], "'")
					if end != -1 {
						redirectURL = line[start : start+end]
						break
					}
				}
			}
		}

		if redirectURL != "" {
			u, err := url.Parse(redirectURL)
			if err == nil {
				recId := u.Query().Get("id")
				if recId != "" {
					// Use the improved decoder which handles the reverse + base64 logic
					decoded, err := utils.DecodeAdLink(recId)
					if err == nil && strings.HasPrefix(decoded, "magnet:") {
						logging.Debug().Str("magnet", decoded).Str("from", redirectURL).Msg("Extracted magnet using spider logic")
						magnetLinks = append(magnetLinks, ExtractedMagnet{Link: decoded, Context: ctxStr})
					}
				}
			}
		}

		// Look for magnet links in the resolved page (direct magnets fallback)
		doc.Find("a[href^=\"magnet:\"]").Each(func(_ int, elem *goquery.Selection) {
			if magnet, ok := elem.Attr("href"); ok && strings.HasPrefix(magnet, "magnet:") {
				logging.Debug().Str("magnet", magnet).Str("href", href).Msg("Extracted magnet from resolved ad link")
				magnetLinks = append(magnetLinks, ExtractedMagnet{Link: magnet, Context: ctxStr})
			}
		})
	})

	var audio []schema.Audio
	var year string
	var size []string
	var allText strings.Builder
	article.Find("div.content p").Each(func(i int, s *goquery.Selection) {
		// pattern:
		// Título Traduzido: Fundação
		// Título Original: Foundation
		// IMDb: 7,5
		// Ano de Lançamento: 2023
		// Gênero: Ação | Aventura | Ficção
		// Formato: MKV
		// Qualidade: WEB-DL
		// Áudio: Português | Inglês
		// Idioma: Português | Inglês
		// Legenda: Português
		// Tamanho: –
		// Qualidade de Áudio: 10
		// Qualidade de Vídeo: 10
		// Duração: 59 Min.
		// Servidor: Torrent
		text := s.Text()
		allText.WriteString(text + "\n")

		audio = append(audio, findAudioFromText(text)...)
		y := findYearFromText(text, title)
		if y != "" {
			year = y
		}
		size = append(size, findSizesFromText(text)...)
	})

	// find any link from imdb
	imdbLink := ""
	article.Find("div.content a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		_imdbLink, err := getIMDBLink(link)
		if err == nil {
			imdbLink = _imdbLink
		}
	})

	// // size = utils.StableUniq(size) // Fixed bug: do not deduplicate sizes // Fixed bug: do not deduplicate sizes

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, magnetInfo := range magnetLinks {
		it := it
		go func(it int, magnetInfo ExtractedMagnet) {
			magnetLink := magnetInfo.Link
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
			
			if releaseTitle == "" {
				releaseTitle = title
			}

			var ctxCln string
			if magnetInfo.Context != "" {
				ctxCln = strings.TrimSpace(magnetInfo.Context)
			}

			// if the number of sizes is equal to the number of magnets, then assign the size to each indexed torrent in order
			var mySize string
			if len(size) == len(magnetLinks) {
				mySize = size[it]
			} else if len(size) > 0 {
				if it < len(size) {
					mySize = size[it]
				} else {
					mySize = size[0]
				}
			} else if len(size) > 0 {
				if it < len(size) {
					mySize = size[it]
				} else {
					mySize = size[0]
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
				Context:       ctxCln,
			}
			
			extractExtendedMetadata(allText.String(), &ixt)
			
			chanIndexedTorrent <- ixt
		}(it, magnetInfo)
	}

	for i := 0; i < len(magnetLinks); i++ {
		it := <-chanIndexedTorrent
		indexedTorrents = append(indexedTorrents, it)
	}

	return indexedTorrents, nil
}

func getPublishedDate(document *goquery.Document) time.Time {
	var date time.Time
	//<meta property="article:published_time" content="2019-08-23T13:20:57+00:00">
	datePublished := strings.TrimSpace(document.Find("meta[property=\"article:published_time\"]").AttrOr("content", ""))

	if datePublished != "" {
		date, _ = time.Parse(time.RFC3339, datePublished)
	}

	return date
}
