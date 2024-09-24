package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hbollon/go-edlib"

	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

var bludv = IndexerMeta{
	URL:       "https://bludvfilmes.tv/",
	SearchURL: "?s=",
}

func (i *Indexer) HandlerBluDVIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		i.metrics.IndexerDuration.WithLabelValues("bludv").Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues("bludv").Inc()
	}()

	ctx := r.Context()
	// supported query params: q, season, episode, filter_results
	q := r.URL.Query().Get("q")

	// URL encode query param
	q = url.QueryEscape(q)
	url := bludv.URL
	if q != "" {
		url = fmt.Sprintf("%s%s%s", url, bludv.SearchURL, q)
	}

	fmt.Println("URL:>", url)
	resp, err := i.requester.GetDocument(ctx, url)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			fmt.Println(err)
		}
		i.metrics.IndexerErrors.WithLabelValues("bludv").Inc()
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

		i.metrics.IndexerErrors.WithLabelValues("bludv").Inc()
		return
	}

	var links []string
	doc.Find(".post").Each(func(i int, s *goquery.Selection) {
		// get link from h2.entry-title > a
		link, _ := s.Find("div.title > a").Attr("href")
		links = append(links, link)
	})

	var itChan = make(chan []IndexedTorrent)
	var errChan = make(chan error)
	indexedTorrents := []IndexedTorrent{}
	for _, link := range links {
		go func(link string) {
			torrents, err := getTorrentsBluDV(ctx, i, link)
			if err != nil {
				fmt.Println(err)
				errChan <- err
			}
			itChan <- torrents
		}(link)
	}

	for i := 0; i < len(links); i++ {
		select {
		case torrents := <-itChan:
			indexedTorrents = append(indexedTorrents, torrents...)
		case err := <-errChan:
			fmt.Println(err)
		}
	}

	for i, it := range indexedTorrents {
		jLower := strings.ReplaceAll(strings.ToLower(fmt.Sprintf("%s %s", it.Title, it.OriginalTitle)), ".", " ")
		qLower := strings.ToLower(q)
		splitLength := 2
		indexedTorrents[i].Similarity = edlib.JaccardSimilarity(jLower, qLower, splitLength)
	}

	// remove the ones with zero similarity
	if len(indexedTorrents) > 20 && r.URL.Query().Get("filter_results") != "" && r.URL.Query().Get("q") != "" {
		indexedTorrents = utils.Filter(indexedTorrents, func(it IndexedTorrent) bool {
			return it.Similarity > 0
		})
	}

	// sort by similarity
	slices.SortFunc(indexedTorrents, func(i, j IndexedTorrent) int {
		return int((j.Similarity - i.Similarity) * 1000)
	})

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(Response{
		Results: indexedTorrents,
		Count:   len(indexedTorrents),
	})
	if err != nil {
		fmt.Println(err)
	}
}

func getTorrentsBluDV(ctx context.Context, i *Indexer, link string) ([]IndexedTorrent, error) {
	var indexedTorrents []IndexedTorrent
	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	article := doc.Find(".post")
	title := strings.Replace(article.Find(".title > h1").Text(), " - Download", "", -1)
	textContent := article.Find("div.content")
	date := getPublishedDate(doc)
	magnets := textContent.Find("a[href^=\"magnet\"]")
	var magnetLinks []string
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var audio []schema.Audio
	var year string
	var size []string
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
		re := regexp.MustCompile(`https://www.imdb.com/title/(tt\d+)`)
		matches := re.FindStringSubmatch(link)
		if len(matches) > 0 {
			imdbLink = matches[0]
		}
	})

	size = stableUniq(size)

	var chanIndexedTorrent = make(chan IndexedTorrent)

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
			magnetAudio := []schema.Audio{}
			if strings.Contains(strings.ToLower(releaseTitle), "dual") || strings.Contains(strings.ToLower(releaseTitle), "dublado") {
				magnetAudio = append(magnetAudio, audio...)
			} else if len(audio) > 1 {
				// remove portuguese audio, and append to magnetAudio
				for _, a := range audio {
					if a != schema.AudioPortuguese {
						magnetAudio = append(magnetAudio, a)
					}
				}
			} else {
				magnetAudio = append(magnetAudio, audio...)
			}

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

			ixt := IndexedTorrent{
				Title:         appendAudioISO639_2Code(releaseTitle, magnetAudio),
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

func getPublishedDate(document *goquery.Document) time.Time {
	var date time.Time
	//<meta property="article:published_time" content="2019-08-23T13:20:57+00:00">
	datePublished := strings.TrimSpace(document.Find("meta[property=\"article:published_time\"]").AttrOr("content", ""))

	if datePublished != "" {
		date, _ = time.Parse(time.RFC3339, datePublished)
	}

	return date
}
