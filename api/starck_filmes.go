package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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

var starck_filmes = IndexerMeta{
	URL:       "https://www.starckfilmes.online/",
	SearchURL: "?s=",
}

func (i *Indexer) HandlerStarckFilmesIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		i.metrics.IndexerDuration.WithLabelValues("starck_filmes").Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues("starck_filmes").Inc()
	}()

	ctx := r.Context()
	// supported query params: q, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	// URL encode query param
	q = url.QueryEscape(q)
	url := starck_filmes.URL
	if q != "" {
		url = fmt.Sprintf("%s%s%s", url, starck_filmes.SearchURL, q)
	} else if page != "" {
		url = fmt.Sprintf("%spage/%s", url, page)
	}

	fmt.Println("URL:>", url)
	resp, err := i.requester.GetDocument(ctx, url)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			fmt.Println(err)
		}
		i.metrics.IndexerErrors.WithLabelValues("starck_filmes").Inc()
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

		i.metrics.IndexerErrors.WithLabelValues("starck_filmes").Inc()
		return
	}

	var links []string
	doc.Find(".item").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Find("div.sub-item > a").Attr("href")
		links = append(links, link)
	})

	var itChan = make(chan []schema.IndexedTorrent)
	var errChan = make(chan error)
	indexedTorrents := []schema.IndexedTorrent{}
	for _, link := range links {
		go func(link string) {
			torrents, err := getTorrentStarckFilmes(ctx, i, link)
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
		indexedTorrents = utils.Filter(indexedTorrents, func(it schema.IndexedTorrent) bool {
			return it.Similarity > 0
		})
	}

	// sort by similarity
	slices.SortFunc(indexedTorrents, func(i, j schema.IndexedTorrent) int {
		return int((j.Similarity - i.Similarity) * 1000)
	})

	// send to search index
	go func() {
		_ = i.search.IndexTorrents(indexedTorrents)
	}()

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(Response{
		Results: indexedTorrents,
		Count:   len(indexedTorrents),
	})
	if err != nil {
		fmt.Println(err)
	}
}

func getTorrentStarckFilmes(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	post := doc.Find(".post")
	capa := post.Find(".capa")
	title := capa.Find(".post-description > h2").Text()
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
		// pattern:
		// Nome Original: 28 Weeks Later
		// Lançamento: 2007
		// Duração: 1h 40 min
		// Gênero: Terror, Suspense, Ficção
		// Formato: MKV
		// Tamanho: 2.45 GB 
		// Qualidade de Video: 10
		// Qualidade do Audio: 10
		// Idioma: Português | Inglês
		// Legenda: Português, Inglês, Espanhol
		var text strings.Builder
		s.Find("span").Each(func (i int, span *goquery.Selection) {
			text.WriteString(span.Text())
			text.WriteString(" ")
		})
		fmt.Println(text.String())
		audio = append(audio, findAudioFromText(text.String())...)
		y := findYearFromText(text.String(), title)
		if y != "" {
			year = y
		}
		size = append(size, findSizesFromText(text.String())...)
	})

	// TODO: find any link from imdb
	imdbLink := ""

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

			ixt := schema.IndexedTorrent{
				Title:         appendAudioISO639_2Code(releaseTitle, magnetAudio),
				OriginalTitle: title,
				Details:       link,
				Year:          year,
				IMDB:          imdbLink,
				Audio:         magnetAudio,
				MagnetLink:    magnetLink,
				Date:		   time.Now(),
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
