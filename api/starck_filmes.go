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

type Requester struct {
}

func (r *Requester) GetDocument(ctx context.Context, url string) (resp *strings.Reader, err error) {
	return nil, nil
}

func (r *Requester) ExpireDocument(ctx context.Context, url string) error {
	fmt.Println("ExpireDocument chamado para:", url)
	return nil
}

var starck_filmes = IndexerMeta{
	Label:       "starck_filmes",
	URL:         "https://www.starckfilmes.fans/",
	SearchURL:   "?s=",
	PagePattern: "page/%s",
}

func removeDuplicates(links []string) []string {
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

func (i *Indexer) trySearchVariations(ctx context.Context, baseURL, searchURL, query string) ([]string, error) {
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
		doc.Find(".item").Each(func(j int, s *goquery.Selection) {
			link, exists := s.Find("div.sub-item > a").Attr("href")
			if exists && link != "" {
				variationLinks = append(variationLinks, link)
			}
		})
		
		allLinks = append(allLinks, variationLinks...)
	}
	
	uniqueLinks := removeDuplicates(allLinks)
	return uniqueLinks, nil
}

func createStandardizedTitle(originalTitle, year, releaseTitle string) string {
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

func (i *Indexer) HandlerStarckFilmesIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	metadata := starck_filmes

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
		links, err = i.trySearchVariations(ctx, metadata.URL, metadata.SearchURL, q)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}
	} else {
		urlStr := metadata.URL
		if page != "" {
			urlStr = fmt.Sprintf(fmt.Sprintf("%s%s", urlStr, metadata.PagePattern), page)
		} else {
			urlStr = fmt.Sprintf(fmt.Sprintf("%s%s", urlStr, metadata.PagePattern), "1")
		}

		resp, err := i.requester.GetDocument(ctx, urlStr)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}
		defer resp.Close()

		doc, err := goquery.NewDocumentFromReader(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			i.metrics.IndexerErrors.WithLabelValues(metadata.Label).Inc()
			return
		}

		doc.Find(".item").Each(func(i int, s *goquery.Selection) {
			link, _ := s.Find("div.sub-item > a").Attr("href")
			if link != "" {
				links = append(links, link)
			}
		})
	}

	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentStarckFilmes(ctx, i, link)
	})

	postProcessedTorrents := indexedTorrents
	for _, processor := range i.postProcessors {
		postProcessedTorrents = processor(i, r, postProcessedTorrents)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{
		Results: postProcessedTorrents,
		Count:   len(postProcessedTorrents),
	})
}

func getTorrentStarckFilmes(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent

	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	date := getPublishedDateFromRawString(link)

	post := doc.Find(".post")
	capa := post.Find(".capa")

	pageTitle := capa.Find(".post-description > h2").Text()

	var originalTitle string
	capa.Find(".post-description p").Each(func(i int, s *goquery.Selection) {
		spans := s.Find("span")
		spans.Each(func(j int, span *goquery.Selection) {
			if strings.Contains(span.Text(), "Nome Original:") {
				originalSpan := span.Next()
				if originalSpan != nil && strings.TrimSpace(originalSpan.Text()) != "" {
					originalTitle = strings.TrimSpace(originalSpan.Text())
				}
			}
		})
	})

	if originalTitle == "" {
		originalTitle = pageTitle
	}

	post_buttons := post.Find(".post-buttons")
	magnets := post_buttons.Find("a[href^=\"magnet\"]")
	var magnetLinks []string
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var audio []schema.Audio
	var year string
	var size []string
	capa.Find(".post-description p").Each(func(i int, s *goquery.Selection) {
		var text strings.Builder
		s.Find("span").Each(func(i int, span *goquery.Selection) {
			text.WriteString(span.Text())
			text.WriteString(" ")
		})
		audio = append(audio, findAudioFromText(text.String())...)
		y := findYearFromText(text.String(), pageTitle)
		if y != "" {
			year = y
		}
		size = append(size, findSizesFromText(text.String())...)
	})

	imdbLink := ""
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

			standardizedTitle := createStandardizedTitle(originalTitle, year, originalReleaseTitle)

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
				OriginalTitle: pageTitle,
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
