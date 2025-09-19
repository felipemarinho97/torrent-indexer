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

var torrent_dos_filmes = IndexerMeta{
	Label:       "torrent_dos_filmes",
	URL:         "https://torrentdosfilmes.se/",
	SearchURL:   "?s=",
	PagePattern: "category/dublado/page/%s",
}

func createStandardizedTitleTDF(originalTitle, year, releaseTitle string) string {
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

func (i *Indexer) trySearchVariationsTDF(ctx context.Context, baseURL, searchURL, query string) ([]string, error) {
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
		doc.Find(".post").Each(func(j int, s *goquery.Selection) {
			link, exists := s.Find("div.title > a").Attr("href")
			if exists && link != "" {
				variationLinks = append(variationLinks, link)
			}
		})
		
		allLinks = append(allLinks, variationLinks...)
	}
	
	uniqueLinks := removeDuplicatesTDF(allLinks)
	return uniqueLinks, nil
}

func removeDuplicatesTDF(links []string) []string {
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

func (i *Indexer) HandlerTorrentDosFilmesIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := torrent_dos_filmes

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
		links, err = i.trySearchVariationsTDF(ctx, metadata.URL, metadata.SearchURL, q)
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

		doc.Find(".post").Each(func(i int, s *goquery.Selection) {
			link, _ := s.Find("div.title > a").Attr("href")
			if link != "" {
				links = append(links, link)
			}
		})
	}

	if len(links) == 0 {
		
	}

	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsTorrentDosFilmes(ctx, i, link)
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

func getTorrentsTorrentDosFilmes(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	article := doc.Find("article")
	pageTitle := strings.Replace(article.Find(".title > h1").Text(), " - Download", "", -1)
	textContent := article.Find("div.content")
	date := getPublishedDateFromMeta(doc)
	magnets := textContent.Find("a[href^=\"magnet\"]")
	var magnetLinks []string
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var originalTitle string
	article.Find("div.content").Each(func(i int, s *goquery.Selection) {
		htmlContent, _ := s.Html()
		
		titleRegex := regexp.MustCompile(`(?i)t[íi]tulo\s+original:\s*</b>\s*([^<\n\r]+)`)
		if matches := titleRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
			originalTitle = strings.TrimSpace(matches[1])
		}
		
		if originalTitle == "" {
			text := s.Text()
			if strings.Contains(text, "Título Original:") {
				parts := strings.Split(text, "Título Original:")
				if len(parts) > 1 {
					titlePart := strings.TrimSpace(parts[1])
					lines := strings.Split(titlePart, "\n")
					if len(lines) > 0 {
						originalTitle = strings.TrimSpace(lines[0])
					}
				}
			} else if strings.Contains(text, "Titulo Original:") {
				parts := strings.Split(text, "Titulo Original:")
				if len(parts) > 1 {
					titlePart := strings.TrimSpace(parts[1])
					lines := strings.Split(titlePart, "\n")
					if len(lines) > 0 {
						originalTitle = strings.TrimSpace(lines[0])
					}
				}
			}
		}
	})

	if originalTitle == "" {
		originalTitle = pageTitle
	}

	var audio []schema.Audio
	var year string
	var size []string
	article.Find("div.content p").Each(func(i int, s *goquery.Selection) {
		text := s.Text()

		audio = append(audio, findAudioFromText(text)...)
		y := findYearFromText(text, pageTitle)
		if y != "" {
			year = y
		}
		size = append(size, findSizesFromText(text)...)
	})

	imdbLink := ""
	article.Find("div.content a").Each(func(i int, s *goquery.Selection) {
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

			standardizedTitle := createStandardizedTitleTDF(originalTitle, year, originalReleaseTitle)

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

			processedTitle := processTitle(pageTitle, magnetAudio)

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
