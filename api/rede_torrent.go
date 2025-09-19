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

	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var rede_torrent = IndexerMeta{
	Label:       "rede_torrent",
	URL:         "https://redetorrent.com/",
	SearchURL:   "index.php?s=",
	PagePattern: "%s",
}

func createStandardizedTitleRT(originalTitle, year, releaseTitle string) string {
	if originalTitle == "" {
		return releaseTitle
	}
	
	cleanOriginalTitle := strings.ReplaceAll(originalTitle, " ", ".")
	cleanOriginalTitle = regexp.MustCompile(`[^\w\.\-]`).ReplaceAllString(cleanOriginalTitle, "")
	
	yearRegex := regexp.MustCompile(`(19|20)\d{2}`)
	seasonRegex := regexp.MustCompile(`(?i)s\d{1,2}e?\d{0,2}`)
	
	if yearMatch := yearRegex.FindStringIndex(releaseTitle); yearMatch != nil {
		beforeYear := releaseTitle[:yearMatch[0]]
		fromYear := releaseTitle[yearMatch[0]:]
		
		beforeYear = strings.TrimRight(beforeYear, ". ")
		if beforeYear != "" {
			return cleanOriginalTitle + "." + fromYear
		} else {
			return cleanOriginalTitle + "." + fromYear
		}
	}
	
	if seasonMatch := seasonRegex.FindStringIndex(releaseTitle); seasonMatch != nil {
		beforeSeason := releaseTitle[:seasonMatch[0]]
		fromSeason := releaseTitle[seasonMatch[0]:]
		
		beforeSeason = strings.TrimRight(beforeSeason, ". ")
		if beforeSeason != "" {
			return cleanOriginalTitle + "." + fromSeason
		} else {
			return cleanOriginalTitle + "." + fromSeason
		}
	}
	
	return releaseTitle
}

func (i *Indexer) trySearchVariationsRT(ctx context.Context, baseURL, searchURL, query string) ([]string, error) {
	if query == "" {
		return nil, nil
	}
	
	variations := []string{
		query,
	}
	
	firstWord := strings.Split(query, " ")[0]
	if !strings.HasPrefix(strings.ToLower(query), "the ") {
		variations = append(variations, "The "+firstWord)
	}
	
	if firstWord != query {
		variations = append(variations, firstWord)
	}
	
	var allLinks []string
	
	for _, variation := range variations {
		encodedQuery := url.QueryEscape(variation)
		searchURL := baseURL + searchURL + encodedQuery
		
		resp, err := i.requester.GetDocument(ctx, searchURL)
		if err != nil {
			continue
		}
		
		doc, err := goquery.NewDocumentFromReader(resp)
		resp.Close()
		if err != nil {
			continue
		}
		
		var variationLinks []string
		doc.Find(".capa_lista").Each(func(j int, s *goquery.Selection) {
			link, exists := s.Find("a").Attr("href")
			if exists && link != "" {
				variationLinks = append(variationLinks, link)
			}
		})
		
		allLinks = append(allLinks, variationLinks...)
	}
	
	uniqueLinks := removeDuplicatesRT(allLinks)
	return uniqueLinks, nil
}

func removeDuplicatesRT(links []string) []string {
	keys := make(map[string]bool)
	var result []string
	
	for _, link := range links {
		if !keys[link] && link != "" {
			keys[link] = true
			result = append(result, link)
		}
	}
	return result
}

func (i *Indexer) HandlerRedeTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := rede_torrent

	defer func() {
		i.metrics.IndexerDuration.WithLabelValues(metadata.Label).Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues(metadata.Label).Inc()
	}()

	ctx := r.Context()
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	var links []string
	var err error

	if q != "" {
		links, err = i.trySearchVariationsRT(ctx, metadata.URL, metadata.SearchURL, q)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				fmt.Println(err)
			}
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}
	} else {
		url := metadata.URL
		if page != "" {
			url = fmt.Sprintf(fmt.Sprintf("%s%s", url, metadata.PagePattern), page)
		}

		resp, err := i.requester.GetDocument(ctx, url)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				fmt.Println(err)
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
				fmt.Println(err)
			}
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}

		doc.Find(".capa_lista").Each(func(i int, s *goquery.Selection) {
			link, _ := s.Find("a").Attr("href")
			if link != "" {
				links = append(links, link)
			}
		})
	}

	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsRedeTorrent(ctx, i, link)
	})

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
		fmt.Println(err)
	}
}

func getTorrentsRedeTorrent(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	article := doc.Find(".conteudo")
	titleRe := regexp.MustCompile(`^(.*?)(?: - (.*?))? \((\d{4})\)`)
	titleP := titleRe.FindStringSubmatch(article.Find("h1").Text())
	if len(titleP) < 3 {
		return nil, fmt.Errorf("could not extract title from %s", link)
	}
	title := strings.TrimSpace(titleP[1])
	year := strings.TrimSpace(titleP[3])

	var originalTitle string
	article.Find("div#informacoes > p").Each(func(i int, s *goquery.Selection) {
		htmlContent, err := s.Html()
		if err != nil {
			return
		}

		htmlContent = strings.ReplaceAll(htmlContent, "\n", "")
		htmlContent = strings.ReplaceAll(htmlContent, "\t", "")

		brRe := regexp.MustCompile(`<br\s*\/?>`)
		htmlContent = brRe.ReplaceAllString(htmlContent, "<br>")
		lines := strings.Split(htmlContent, "<br>")

		for _, line := range lines {
			re := regexp.MustCompile(`<[^>]*>`)
			line = re.ReplaceAllString(line, "")
			line = strings.TrimSpace(line)
			
			if strings.Contains(line, "Título Original:") {
				parts := strings.Split(line, "Título Original:")
				if len(parts) > 1 {
					originalTitle = strings.TrimSpace(parts[1])
				}
			}
		}
	})

	if originalTitle == "" {
		originalTitle = title
	}

	textContent := article.Find(".apenas_itemprop")
	date := getPublishedDateFromMeta(doc)
	magnets := textContent.Find("a[href^=\"magnet\"]")
	var magnetLinks []string
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var audio []schema.Audio
	var size []string
	article.Find("div#informacoes > p").Each(func(i int, s *goquery.Selection) {
		htmlContent, err := s.Html()
		if err != nil {
			fmt.Println(err)
			return
		}

		htmlContent = strings.ReplaceAll(htmlContent, "\n", "")
		htmlContent = strings.ReplaceAll(htmlContent, "\t", "")

		brRe := regexp.MustCompile(`<br\s*\/?>`)
		htmlContent = brRe.ReplaceAllString(htmlContent, "<br>")
		lines := strings.Split(htmlContent, "<br>")

		var text strings.Builder
		for _, line := range lines {
			re := regexp.MustCompile(`<[^>]*>`)
			line = re.ReplaceAllString(line, "")

			line = strings.TrimSpace(line)
			text.WriteString(line + "\n")
		}

		audio = append(audio, findAudioFromText(text.String())...)
		y := findYearFromText(text.String(), title)
		if y != "" {
			year = y
		}
		size = append(size, findSizesFromText(text.String())...)
	})

	imdbLink := ""
	article.Find("a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		_imdbLink, err := getIMDBLink(link)
		if err == nil {
			imdbLink = _imdbLink
		}
	})

	size = utils.StableUniq(size)

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	for it, magnetLink := range magnetLinks {
		it := it
		go func(it int, magnetLink string) {
			magnetLink = strings.ReplaceAll(magnetLink, "&#038;", "&")
			magnetLink = strings.ReplaceAll(magnetLink, "&amp;", "&")
			
			magnet, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				fmt.Println(err)
			}

			originalReleaseTitle := strings.TrimSpace(magnet.DisplayName)
			originalReleaseTitle, err = url.QueryUnescape(originalReleaseTitle)
			if err != nil {
				originalReleaseTitle = strings.TrimSpace(magnet.DisplayName)
			}

			standardizedTitle := createStandardizedTitleRT(originalTitle, year, originalReleaseTitle)

			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			for i, tracker := range trackers {
				unescapedTracker := strings.ReplaceAll(tracker, "&#038;", "&")
				unescapedTracker = strings.ReplaceAll(unescapedTracker, "&amp;", "&")
				
				unescapedTracker, err := url.QueryUnescape(unescapedTracker)
				if err != nil {
					fmt.Println(err)
				}
				trackers[i] = strings.TrimSpace(unescapedTracker)
			}

			magnetAudio := getAudioFromTitle(originalReleaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				fmt.Println(err)
			}

			processedTitle := processTitle(title, magnetAudio)

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
				Title:         standardizedTitle,
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
		indexedTorrents = append(indexedTorrents, it)
	}

	return indexedTorrents, nil
}
