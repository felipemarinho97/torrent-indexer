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

var rede_torrent = IndexerMeta{
	URL:         "https://redetorrent.com/",
	SearchURL:   "index.php?s=",
	PagePattern: "%s",
}

func (i *Indexer) HandlerRedeTorrentIndexer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		i.metrics.IndexerDuration.WithLabelValues("rede_torrent").Observe(time.Since(start).Seconds())
		i.metrics.IndexerRequests.WithLabelValues("rede_torrent").Inc()
	}()

	ctx := r.Context()
	// supported query params: q, season, episode, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	// URL encode query param
	q = url.QueryEscape(q)
	url := rede_torrent.URL
	if q != "" {
		url = fmt.Sprintf("%s%s%s", url, rede_torrent.SearchURL, q)
	} else if page != "" {
		url = fmt.Sprintf(fmt.Sprintf("%s%s", url, rede_torrent.PagePattern), page)
	}

	fmt.Println("URL:>", url)
	resp, err := i.requester.GetDocument(ctx, url)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			fmt.Println(err)
		}
		i.metrics.IndexerErrors.WithLabelValues("rede_torrent").Inc()
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

		i.metrics.IndexerErrors.WithLabelValues("rede_torrent").Inc()
		return
	}

	var links []string
	doc.Find(".capa_lista").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Find("a").Attr("href")
		links = append(links, link)
	})

	var itChan = make(chan []schema.IndexedTorrent)
	var errChan = make(chan error)
	indexedTorrents := []schema.IndexedTorrent{}
	for _, link := range links {
		go func(link string) {
			torrents, err := getTorrentsRedeTorrent(ctx, i, link)
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

func getTorrentsRedeTorrent(ctx context.Context, i *Indexer, link string) ([]schema.IndexedTorrent, error) {
	var indexedTorrents []schema.IndexedTorrent
	doc, err := getDocument(ctx, i, link)
	if err != nil {
		return nil, err
	}

	article := doc.Find(".conteudo")
	// title pattern: "Something - optional balbla (dddd) some shit" - extract "Something" and "dddd"
	titleRe := regexp.MustCompile(`^(.*?)(?: - (.*?))? \((\d{4})\)`)
	titleP := titleRe.FindStringSubmatch(article.Find("h1").Text())
	if len(titleP) < 3 {
		return nil, fmt.Errorf("could not extract title from %s", link)
	}
	title := strings.TrimSpace(titleP[1])
	year := strings.TrimSpace(titleP[3])

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
		// pattern:
		// Filme Bicho de Sete Cabeças Torrent
		// Título Original: Bicho de Sete Cabeças
		// Lançamento: 2001
		// Gêneros: Drama / Nacional
		// Idioma: Português
		// Qualidade: 720p / BluRay
		// Duração: 1h 14 Minutos
		// Formato: Mp4
		// Vídeo: 10 e Áudio: 10
		// Legendas: Português
		// Nota do Imdb: 7.7
		// Tamanho: 1.26 GB

		// we need to manualy parse because the text is not well formatted
		htmlContent, err := s.Html()
		if err != nil {
			fmt.Println(err)
			return
		}

		// remove any \n and \t characters
		htmlContent = strings.ReplaceAll(htmlContent, "\n", "")
		htmlContent = strings.ReplaceAll(htmlContent, "\t", "")

		// split by <br> tags and render each line
		brRe := regexp.MustCompile(`<br\s*\/?>`)
		htmlContent = brRe.ReplaceAllString(htmlContent, "<br>")
		lines := strings.Split(htmlContent, "<br>")

		var text strings.Builder
		for _, line := range lines {
			// remove any HTML tags
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
			magnetAudio := []schema.Audio{}
			if strings.Contains(strings.ToLower(releaseTitle), "dual") || strings.Contains(strings.ToLower(releaseTitle), "dublado") {
				magnetAudio = append(magnetAudio, audio...)
				// if Portuguese audio is not in the audio slice, append it
				if !slices.Contains(magnetAudio, schema.AudioPortuguese) {
					magnetAudio = append(magnetAudio, schema.AudioPortuguese)
				}
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
