package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
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

var filme_torrent = IndexerMeta{
	Label:       "filme_torrent",
	URL:         "https://limonfilmes.org/",
	SearchURL:   "?s=",
	PagePattern: "page/%s",
}

func (i *Indexer) HandlerFilmeTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := filme_torrent

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	// supported query params: q, page
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	// URL encode query param
	q = url.QueryEscape(q)
	targetURL := metadata.URL
	if q != "" {
		targetURL = fmt.Sprintf("%s%s%s", targetURL, metadata.SearchURL, q)
	} else if page != "" {
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
		link, _ := s.Find("div.title > a").Attr("href")
		if link != "" {
			links = append(links, link)
		}
	})

	logging.Debug().Int("links_found", len(links)).Str("url", targetURL).Msg("Links indexed")

	// if no links were indexed, expire the document in cache
	if len(links) == 0 {
		_ = i.requester.ExpireDocument(ctx, targetURL)
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsFilmeTorrent(ctx, i, link, targetURL)
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

func getTorrentsFilmeTorrent(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	article := doc.Find("article")

	// Extract title - removing common suffixes
	titleRaw := article.Find(".entry-title, h1.entry-title").First().Text()
	title := strings.TrimSpace(titleRaw)
	title = strings.TrimSuffix(title, " Torrent Dual Áudio")
	title = strings.TrimSuffix(title, " Torrent Dublado")
	title = strings.TrimSuffix(title, " Torrent Legendado")
	title = strings.TrimSuffix(title, " Torrent")

	// Get published date
	date := getPublishedDateFromMeta(doc)

	// Extract all magnet links from the modal or download section
	textContent := article.Find("div.content, div.entry-content, div.modal-downloads")

	// Find magnet links - they might be base64 encoded in the href
	var magnetLinks []string
	textContent.Find("a.customButton, a[href*='encurta'], a[href^='magnet']").Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists {
			return
		}

		// Check if it's a direct magnet link
		if strings.HasPrefix(href, "magnet:") {
			magnetLinks = append(magnetLinks, href)
			return
		}

		// Check if it's an vialink shortened link
		if strings.Contains(href, "protlink") {
			// get the protlink value
			u, err := url.Parse(href)
			if err != nil {
				logging.Debug().Err(err).Str("href", href).Msg("Failed to parse URL")
				return
			}
			protlink := u.Query().Get("protlink")
			if protlink == "" {
				return
			}
			encurtaLink := fmt.Sprintf("https://vialink.sbs/encurtador/?prot=%s", protlink)
			shortenedMagnet, err := resolveVialinkShortenedLink(ctx, i, encurtaLink)
			if err != nil {
				logging.Debug().Err(err).Str("href", href).Msg("Failed to resolve vialink shortened link")
				return
			}
			if strings.HasPrefix(shortenedMagnet, "magnet:") {
				magnetLinks = append(magnetLinks, shortenedMagnet)
			}
			return
		}

		// Check if it's an encoded link with token parameter
		if strings.Contains(href, "token=") {
			// Extract the token parameter
			u, err := url.Parse(href)
			if err != nil {
				logging.Debug().Err(err).Str("href", href).Msg("Failed to parse URL")
				return
			}

			token := u.Query().Get("token")
			if token != "" {
				// Decode the base64 token
				decodedMagnet, err := utils.Base64Decode(token)
				if err != nil {
					logging.Debug().Err(err).Str("token", token).Msg("Failed to decode base64 token")
					return
				}

				if strings.HasPrefix(decodedMagnet, "magnet:") {
					magnetLinks = append(magnetLinks, decodedMagnet)
				}
			}
		}
	})

	var audio []schema.Audio
	var year string
	var size []string

	// Extract metadata from the entry-meta and content sections
	article.Find("div.entry-meta, div.content p, div.entry-content p").Each(func(i int, s *goquery.Selection) {
		text := s.Text()

		// Extract audio languages
		audio = append(audio, findAudioFromText(text)...)

		// Extract year
		y := findYearFromText(text, title)
		if y != "" {
			year = y
		}

		// Extract sizes
		size = append(size, findSizesFromText(text)...)
	})

	// Find IMDB link
	imdbLink := ""
	article.Find("div.content a, div.entry-content a, .modal-content a").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		_imdbLink, err := getIMDBLink(href)
		if err == nil {
			imdbLink = _imdbLink
		}
	})

	size = utils.StableUniq(size)

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, magnetLink := range magnetLinks {
		it := it
		go func(it int, magnetLink string) {
			mag, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				logging.Error().Err(err).Str("magnet_link", magnetLink).Msg("Failed to parse magnet URI")
				chanIndexedTorrent <- schema.IndexedTorrent{}
				return
			}
			releaseTitle := mag.DisplayName
			infoHash := mag.InfoHash.String()
			trackers := mag.Trackers
			magnetAudio := getAudioFromTitle(releaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				logging.Error().Err(err).Str("info_hash", infoHash).Msg("Failed to get leechers and seeders")
			}

			processedTitle := processTitle(title, magnetAudio)

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
		if it.InfoHash != "" {
			indexedTorrents = append(indexedTorrents, it)
		}
	}

	return indexedTorrents, nil
}

func resolveVialinkShortenedLink(ctx context.Context, i *Indexer, shortenedURL string) (string, error) {
	resp, err := i.requester.GetDocument(ctx, shortenedURL)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	bodyBytes, err := io.ReadAll(resp)
	if err != nil {
		return "", err
	}
	bodyString := string(bodyBytes)

	// Look for the magnet link in the response body
	startIndex := strings.Index(bodyString, "magnet:")
	if startIndex == -1 {
		return "", fmt.Errorf("magnet link not found in shortened link response")
	}

	// Find the end of the magnet link
	endIndex := strings.IndexAny(bodyString[startIndex:], "\"'<> \n")
	if endIndex == -1 {
		endIndex = len(bodyString)
	} else {
		endIndex += startIndex
	}

	magnetLink := bodyString[startIndex:endIndex]

	// decode any HTML entities
	magnetLink = html.UnescapeString(magnetLink)
	return magnetLink, nil
}
