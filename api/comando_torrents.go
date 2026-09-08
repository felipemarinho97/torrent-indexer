package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var comando = IndexerMeta{
	Label:       "comando",
	URL:         utils.GetIndexerURLFromEnv("INDEXER_COMANDO_URL", "https://comando.la/"),
	SearchURL:   "?s=",
	PagePattern: "page/%s",
}

var replacer = strings.NewReplacer(
	"janeiro", "01",
	"fevereiro", "02",
	"março", "03",
	"abril", "04",
	"maio", "05",
	"junho", "06",
	"julho", "07",
	"agosto", "08",
	"setembro", "09",
	"outubro", "10",
	"novembro", "11",
	"dezembro", "12",
)

func (i *Indexer) HandlerComandoIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := comando

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
	if q != "" {
		url = fmt.Sprintf("%s%s%s", url, metadata.SearchURL, q)
	} else if page != "" {
		url = fmt.Sprintf(fmt.Sprintf("%s%s", url, metadata.PagePattern), page)
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
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		// get link from h2.entry-title > a
		link, _ := s.Find("h2.entry-title > a").Attr("href")
		links = append(links, link)
	})

	// if no links were indexed, expire the document in cache
	if len(links) == 0 {
		_ = i.requester.ExpireDocument(ctx, url)
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrents(ctx, i, link, url)
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

func getTorrents(ctx context.Context, i *Indexer, link, referer string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link, referer)
	if err != nil {
		return nil, err
	}

	article := doc.Find("article")
	title := strings.Replace(article.Find(".entry-title").Text(), " - Download", "", -1)
	textContent := article.Find("div.entry-content")
	date := getPublishedDateFromMeta(doc)

	if date.IsZero() {
		// div itemprop="datePublished"
		datePublished := strings.TrimSpace(article.Find("div[itemprop=\"datePublished\"]").Text())
		// pattern: 10 de setembro de 2021
		date, err = parseLocalizedDate(datePublished)
		if err != nil {
			return nil, err
		}
	}

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

		// Fallback: Try resolving via FlareSolverr and look for magnet in HTML
		doc, err := getDocument(ctx, i, href, referer)
		if err != nil {
			logging.Error().Err(err).Str("href", href).Msg("Failed to resolve ad link via FlareSolverr")
			return
		}

		// Smart Extraction: Look for ANY URL with a suspicious ID parameter
		htmlContent, _ := doc.Html()

		smartRe := regexp.MustCompile(`https?://[^"'\s?]+\.(php|xyz|info|top)[^"'\s]*[?&](id|u|link)=([A-Za-z0-9+/=%]{30,})`)
		smartMatches := smartRe.FindAllStringSubmatch(htmlContent, -1)

		ctxStr := ExtractMagnetContext(s)

		for _, match := range smartMatches {
			if len(match) < 4 {
				continue
			}
			recId := match[3]

			decoded, err := utils.DecodeAdLink(recId)
			if err == nil && strings.HasPrefix(decoded, "magnet:") {
				magnetLinks = append(magnetLinks, ExtractedMagnet{Link: decoded, Context: ctxStr})
			}
		}

		// Look for magnet links in the resolved page (direct magnets fallback)
		doc.Find("a[href^=\"magnet:\"]").Each(func(_ int, elem *goquery.Selection) {
			if magnetLink, ok := elem.Attr("href"); ok && strings.HasPrefix(magnetLink, "magnet:") {
				magnetLinks = append(magnetLinks, ExtractedMagnet{Link: magnetLink, Context: ctxStr})
			}
		})
	})

	var audio []schema.Audio
	var year string
	var size []string
	var allText strings.Builder
	article.Find("div.entry-content > p").Each(func(i int, s *goquery.Selection) {
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
	article.Find("a").Each(func(i int, s *goquery.Selection) {
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

func parseLocalizedDate(datePublished string) (time.Time, error) {
	re := regexp.MustCompile(`(\d{1,2}) de (\w+) de (\d{4})`)
	matches := re.FindStringSubmatch(datePublished)
	if len(matches) > 0 {
		day := matches[1]
		// append 0 to single digit day
		if len(day) == 1 {
			day = fmt.Sprintf("0%s", day)
		}
		month := matches[2]
		year := matches[3]
		datePublished = fmt.Sprintf("%s-%s-%s", year, replacer.Replace(month), day)
		date, err := time.Parse("2006-01-02", datePublished)
		if err != nil {
			return time.Time{}, err
		}
		return date, nil
	}
	return time.Time{}, nil
}

func processTitle(title string, a []schema.Audio) string {
	// remove ' - Donwload' from title
	title = strings.Replace(title, " – Download", "", -1)

	// remove 'comando.la' from title
	title = strings.Replace(title, "comando.la", "", -1)

	// add audio ISO 639-2 code to title between ()
	title = appendAudioISO639_2Code(title, a)

	return title
}
