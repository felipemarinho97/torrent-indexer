package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

var comando = IndexerMeta{
	Label:       "comando",
	URL:         "https://comando.la/",
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

	fmt.Println("URL:>", url)
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

	var links []string
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		// get link from h2.entry-title > a
		link, _ := s.Find("h2.entry-title > a").Attr("href")
		links = append(links, link)
	})

	// extract each torrent link
	indexedTorrents := utils.ParallelMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrents(ctx, i, link)
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
		fmt.Println(err)
	}
}

func getTorrents(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	article := doc.Find("article")
	title := strings.Replace(article.Find(".entry-title").Text(), " - Download", "", -1)
	textContent := article.Find("div.entry-content")
	// div itemprop="datePublished"
	datePublished := strings.TrimSpace(article.Find("div[itemprop=\"datePublished\"]").Text())
	// pattern: 10 de setembro de 2021
	date, err := parseLocalizedDate(datePublished)
	if err != nil {
		return nil, err
	}

	magnets := textContent.Find("a[href^=\"magnet\"]")
	var magnetLinks []string
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var audio []schema.Audio
	var year string
	var size []string
	article.Find("div.entry-content > p").Each(func(i int, s *goquery.Selection) {
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

	size = stableUniq(size)

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, magnetLink := range magnetLinks {
		it := it
		go func(it int, magnetLink string) {
			magnet, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				fmt.Println(err)
			}
			releaseTitle := magnet.DisplayName
			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			magnetAudio := getAudioFromTitle(releaseTitle, audio)

			peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
			if err != nil {
				fmt.Println(err)
			}

			title := processTitle(title, magnetAudio)

			// if the number of sizes is equal to the number of magnets, then assign the size to each indexed torrent in order
			var mySize string
			if len(size) == len(magnetLinks) {
				mySize = size[it]
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
		}(it, magnetLink)
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

func stableUniq(s []string) []string {
	var uniq []map[string]interface{}
	m := make(map[string]map[string]interface{})
	for i, v := range s {
		m[v] = map[string]interface{}{
			"v": v,
			"i": i,
		}
	}
	// to order by index
	for _, v := range m {
		uniq = append(uniq, v)
	}

	// sort by index
	for i := 0; i < len(uniq); i++ {
		for j := i + 1; j < len(uniq); j++ {
			if uniq[i]["i"].(int) > uniq[j]["i"].(int) {
				uniq[i], uniq[j] = uniq[j], uniq[i]
			}
		}
	}

	// get only values
	var uniqValues []string
	for _, v := range uniq {
		uniqValues = append(uniqValues, v["v"].(string))
	}

	return uniqValues
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

func getDocument(ctx context.Context, i *Indexer, link string) (*goquery.Document, error) {
	// try to get from redis first
	docCache, err := i.redis.Get(ctx, link)
	if err == nil {
		i.metrics.CacheHits.WithLabelValues("document_body").Inc()
		fmt.Printf("returning from long-lived cache: %s\n", link)
		return goquery.NewDocumentFromReader(io.NopCloser(bytes.NewReader(docCache)))
	}
	defer i.metrics.CacheMisses.WithLabelValues("document_body").Inc()

	resp, err := i.requester.GetDocument(ctx, link)
	if err != nil {
		return nil, err
	}
	defer resp.Close()

	body, err := io.ReadAll(resp)
	if err != nil {
		return nil, err
	}

	// set cache
	err = i.redis.Set(ctx, link, body)
	if err != nil {
		fmt.Println(err)
	}

	doc, err := goquery.NewDocumentFromReader(io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return nil, err
	}

	return doc, nil
}
