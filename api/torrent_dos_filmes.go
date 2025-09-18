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

// Função para criar título padronizado substituindo apenas o início
func createStandardizedTitleTDF(originalTitle, year, releaseTitle string) string {
	// Se não tiver originalTitle, retorna o magnet original
	if originalTitle == "" {
		return releaseTitle
	}
	
	// Limpa o título original (remove caracteres especiais, substitui espaços por pontos)
	cleanOriginalTitle := strings.ReplaceAll(originalTitle, " ", ".")
	cleanOriginalTitle = regexp.MustCompile(`[^\w\.\-]`).ReplaceAllString(cleanOriginalTitle, "")
	
	// Regex para encontrar ano (4 dígitos)
	yearRegex := regexp.MustCompile(`(19|20)\d{2}`)
	
	// Regex para encontrar temporada (SxxExx ou SxxE padrão)
	seasonRegex := regexp.MustCompile(`(?i)s\d{1,2}e?\d{0,2}`)
	
	// Procura por ano primeiro
	if yearMatch := yearRegex.FindStringIndex(releaseTitle); yearMatch != nil {
		// Encontrou ano - substitui tudo antes do ano
		beforeYear := releaseTitle[:yearMatch[0]]
		fromYear := releaseTitle[yearMatch[0]:]
		
		// Remove pontos/espaços extras no final do início
		beforeYear = strings.TrimRight(beforeYear, ". ")
		if beforeYear != "" {
			return cleanOriginalTitle + "." + fromYear
		} else {
			return cleanOriginalTitle + "." + fromYear
		}
	}
	
	// Se não encontrou ano, procura por temporada
	if seasonMatch := seasonRegex.FindStringIndex(releaseTitle); seasonMatch != nil {
		// Encontrou temporada - substitui tudo antes da temporada
		beforeSeason := releaseTitle[:seasonMatch[0]]
		fromSeason := releaseTitle[seasonMatch[0]:]
		
		// Remove pontos/espaços extras no final do início
		beforeSeason = strings.TrimRight(beforeSeason, ". ")
		if beforeSeason != "" {
			return cleanOriginalTitle + "." + fromSeason
		} else {
			return cleanOriginalTitle + "." + fromSeason
		}
	}
	
	// Se não encontrou nem ano nem temporada, retorna o magnet original
	return releaseTitle
}

// Função para fazer busca com variações do termo - TorrentDosFilmes
func (i *Indexer) trySearchVariationsTDF(ctx context.Context, baseURL, searchURL, query string) ([]string, error) {
	if query == "" {
		return nil, nil
	}
	
	// Cria variações do termo de busca
	variations := []string{
		query, // "Amateur 2025"
	}
	
	// Adiciona variação com "The" se não começar com "The"
	firstWord := strings.Split(query, " ")[0]
	if !strings.HasPrefix(strings.ToLower(query), "the ") {
		variations = append(variations, "The "+firstWord)
	}
	
	// Adiciona apenas o primeiro termo (sem ano)
	if firstWord != query {
		variations = append(variations, firstWord)
	}
	
	fmt.Printf("[TorrentDosFilmes] Tentando variações de busca: %v\n", variations)
	
	var allLinks []string
	
	for _, variation := range variations {
		fmt.Printf("[TorrentDosFilmes] Buscando por: %s\n", variation)
		
		encodedQuery := url.QueryEscape(variation)
		searchURL := baseURL + searchURL + encodedQuery
		
		fmt.Printf("[TorrentDosFilmes] URL de busca: %s\n", searchURL)
		
		// Faz a requisição
		resp, err := i.requester.GetDocument(ctx, searchURL)
		if err != nil {
			fmt.Printf("[TorrentDosFilmes] Erro na busca por '%s': %v\n", variation, err)
			continue
		}
		
		doc, err := goquery.NewDocumentFromReader(resp)
		resp.Close()
		if err != nil {
			fmt.Printf("[TorrentDosFilmes] Erro ao parsear HTML para '%s': %v\n", variation, err)
			continue
		}
		
		// Extrai os links desta variação - seletor específico do TorrentDosFilmes
		var variationLinks []string
		doc.Find(".post").Each(func(j int, s *goquery.Selection) {
			link, exists := s.Find("div.title > a").Attr("href")
			if exists && link != "" {
				variationLinks = append(variationLinks, link)
			}
		})
		
		fmt.Printf("[TorrentDosFilmes] Encontrados %d resultados para '%s'\n", len(variationLinks), variation)
		allLinks = append(allLinks, variationLinks...)
		
		// Se encontrou resultados na primeira variação, pode parar (opcional)
		// if len(variationLinks) > 0 {
		//     break
		// }
	}
	
	// Remove duplicatas
	uniqueLinks := removeDuplicatesTDF(allLinks)
	fmt.Printf("[TorrentDosFilmes] Total único de links encontrados: %d\n", len(uniqueLinks))
	
	return uniqueLinks, nil
}

// Função para remover duplicatas - TorrentDosFilmes
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
	// supported query params: q, season, episode, page, filter_results
	q := r.URL.Query().Get("q")
	page := r.URL.Query().Get("page")

	var links []string
	var err error

	if q != "" {
		// Usa a nova função de busca com variações
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
		// Para paginação ou busca sem termo, usa a lógica original
		url := metadata.URL
		if page != "" {
			url = fmt.Sprintf(fmt.Sprintf("%s%s", url, metadata.PagePattern), page)
		}

		fmt.Println("[TorrentDosFilmes] URL de paginação:>", url)
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

	// if no links were indexed, expire the document in cache
	if len(links) == 0 {
		if q != "" {
			fmt.Printf("[TorrentDosFilmes] Nenhum resultado encontrado para: %s\n", q)
		}
	}

	// extract each torrent link
	indexedTorrents := utils.ParallelFlatMap(links, func(link string) ([]schema.IndexedTorrent, error) {
		return getTorrentsTorrentDosFilmes(ctx, i, link)
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

	// Procura pelo título original no HTML
	var originalTitle string
	article.Find("div.content").Each(func(i int, s *goquery.Selection) {
		// Busca no HTML bruto por padrões como "Titulo Original:" ou "Título Original:"
		htmlContent, _ := s.Html()
		
		// Regex para capturar título original (com ou sem acento)
		titleRegex := regexp.MustCompile(`(?i)t[íi]tulo\s+original:\s*</b>\s*([^<\n\r]+)`)
		if matches := titleRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
			originalTitle = strings.TrimSpace(matches[1])
			fmt.Printf("[TorrentDosFilmes] Título Original encontrado: '%s'\n", originalTitle)
		}
		
		// Fallback: busca no texto simples
		if originalTitle == "" {
			text := s.Text()
			if strings.Contains(text, "Título Original:") {
				parts := strings.Split(text, "Título Original:")
				if len(parts) > 1 {
					// Pega tudo até a próxima quebra de linha ou tag
					titlePart := strings.TrimSpace(parts[1])
					lines := strings.Split(titlePart, "\n")
					if len(lines) > 0 {
						originalTitle = strings.TrimSpace(lines[0])
						fmt.Printf("[TorrentDosFilmes] Título Original (fallback) encontrado: '%s'\n", originalTitle)
					}
				}
			} else if strings.Contains(text, "Titulo Original:") {
				// Versão sem acento
				parts := strings.Split(text, "Titulo Original:")
				if len(parts) > 1 {
					titlePart := strings.TrimSpace(parts[1])
					lines := strings.Split(titlePart, "\n")
					if len(lines) > 0 {
						originalTitle = strings.TrimSpace(lines[0])
						fmt.Printf("[TorrentDosFilmes] Titulo Original (sem acento) encontrado: '%s'\n", originalTitle)
					}
				}
			}
		}
	})

	// Se não encontrou o título original, usa o título da página
	if originalTitle == "" {
		originalTitle = pageTitle
	}

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
		y := findYearFromText(text, pageTitle)
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

	size = utils.StableUniq(size)

	var chanIndexedTorrent = make(chan schema.IndexedTorrent)

	// for each magnet link, create a new indexed torrent
	for it, magnetLink := range magnetLinks {
		it := it
		go func(it int, magnetLink string) {
			magnet, err := magnet.ParseMagnetUri(magnetLink)
			if err != nil {
				fmt.Println(err)
			}

			originalReleaseTitle := strings.TrimSpace(magnet.DisplayName)
			originalReleaseTitle, err = url.QueryUnescape(originalReleaseTitle)
			if err != nil {
				originalReleaseTitle = strings.TrimSpace(magnet.DisplayName)
			}

			// Cria o título padronizado usando a nova função
			standardizedTitle := createStandardizedTitleTDF(originalTitle, year, originalReleaseTitle)

			infoHash := magnet.InfoHash.String()
			trackers := magnet.Trackers
			for i, tracker := range trackers {
				unescapedTracker, err := url.QueryUnescape(tracker)
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
				Title:         standardizedTitle, // Usa o título padronizado
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
