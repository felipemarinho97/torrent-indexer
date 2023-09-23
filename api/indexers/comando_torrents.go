package indexers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	URL         = "https://comando.la/"
	queryFilter = "?s="
)

type Audio string

const (
	AudioPortuguese = "Português"
	AudioEnglish    = "Inglês"
	AudioSpanish    = "Espanhol"
)

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

type IndexedTorrent struct {
	Title         string    `json:"title"`
	OriginalTitle string    `json:"original_title"`
	Details       string    `json:"details"`
	Year          string    `json:"year"`
	Audio         []Audio   `json:"audio"`
	MagnetLink    string    `json:"magnet_link"`
	Date          time.Time `json:"date"`
	InfoHash      string    `json:"info_hash"`
}

func HandlerComandoIndexer(w http.ResponseWriter, r *http.Request) {
	// supported query params: q, season, episode
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "param q is required"})
		return
	}

	// URL encode query param
	q = url.QueryEscape(q)
	url := URL + queryFilter + q

	fmt.Println("URL:>", url)
	resp, err := http.Get(url)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	var links []string
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		// get link from h2.entry-title > a
		link, _ := s.Find("h2.entry-title > a").Attr("href")
		links = append(links, link)
	})

	var itChan = make(chan []IndexedTorrent)
	var errChan = make(chan error)
	var indexedTorrents []IndexedTorrent
	for _, link := range links {
		go func(link string) {
			torrents, err := getTorrents(link)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(indexedTorrents)
}

func getTorrents(link string) ([]IndexedTorrent, error) {
	var indexedTorrents []IndexedTorrent
	resp, err := http.Get(link)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	article := doc.Find("article")
	title := strings.Replace(article.Find(".entry-title").Text(), " - Download", "", -1)
	textContent := article.Find("div.entry-content")
	// div itemprop="datePublished"
	datePublished := strings.TrimSpace(article.Find("div[itemprop=\"datePublished\"]").Text())
	// pattern: 10 de setembro de 2021
	re := regexp.MustCompile(`(\d{2}) de (\w+) de (\d{4})`)
	matches := re.FindStringSubmatch(datePublished)
	var date time.Time
	if len(matches) > 0 {
		day := matches[1]
		month := matches[2]
		year := matches[3]
		datePublished = fmt.Sprintf("%s-%s-%s", year, replacer.Replace(month), day)
		date, err = time.Parse("2006-01-02", datePublished)
		if err != nil {
			return nil, err
		}
	}
	magnets := textContent.Find("a[href^=\"magnet\"]")
	var magnetLinks []string
	magnets.Each(func(i int, s *goquery.Selection) {
		magnetLink, _ := s.Attr("href")
		magnetLinks = append(magnetLinks, magnetLink)
	})

	var audio []Audio
	var year string
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
		// Legenda: Português
		// Tamanho: –
		// Qualidade de Áudio: 10
		// Qualidade de Vídeo: 10
		// Duração: 59 Min.
		// Servidor: Torrent
		text := s.Text()

		re := regexp.MustCompile(`Áudio: (.*)`)
		audioMatch := re.FindStringSubmatch(text)
		if len(audioMatch) > 0 {
			langs_raw := strings.Split(audioMatch[1], "|")
			for _, lang := range langs_raw {
				lang = strings.TrimSpace(lang)
				if lang == "Português" {
					audio = append(audio, AudioPortuguese)
				} else if lang == "Inglês" {
					audio = append(audio, AudioEnglish)
				} else if lang == "Espanhol" {
					audio = append(audio, AudioSpanish)
				}
			}
		}

		re = regexp.MustCompile(`Lançamento: (.*)`)
		yearMatch := re.FindStringSubmatch(text)
		if len(yearMatch) > 0 {
			year = yearMatch[1]
		}

		// if year is empty, try to get it from title
		if year == "" {
			re = regexp.MustCompile(`\((\d{4})\)`)
			yearMatch := re.FindStringSubmatch(title)
			if len(yearMatch) > 0 {
				year = yearMatch[1]
			}
		}
	})

	// for each magnet link, create a new indexed torrent
	for _, magnetLink := range magnetLinks {
		releaseTitle := extractReleaseName(magnetLink)
		magnetAudio := []Audio{}
		if strings.Contains(strings.ToLower(releaseTitle), "dual") {
			magnetAudio = append(magnetAudio, AudioPortuguese)
			magnetAudio = append(magnetAudio, audio...)
		} else {
			// filter portuguese audio from list
			for _, lang := range audio {
				if lang != AudioPortuguese {
					magnetAudio = append(magnetAudio, lang)
				}
			}
		}

		// remove duplicates
		magnetAudio = removeDuplicates(magnetAudio)
		// decode url encoded title
		releaseTitle, _ = url.QueryUnescape(releaseTitle)

		infoHash := extractInfoHash(magnetLink)

		indexedTorrents = append(indexedTorrents, IndexedTorrent{
			Title:         releaseTitle,
			OriginalTitle: title,
			Details:       link,
			Year:          year,
			Audio:         magnetAudio,
			MagnetLink:    magnetLink,
			Date:          date,
			InfoHash:      infoHash,
		})
	}

	return indexedTorrents, nil
}

func extractReleaseName(magnetLink string) string {
	re := regexp.MustCompile(`dn=(.*?)&`)
	matches := re.FindStringSubmatch(magnetLink)
	if len(matches) > 0 {
		return matches[1]
	}
	return ""
}

func extractInfoHash(magnetLink string) string {
	re := regexp.MustCompile(`btih:(.*?)&`)
	matches := re.FindStringSubmatch(magnetLink)
	if len(matches) > 0 {
		return matches[1]
	}
	return ""
}

func removeDuplicates(elements []Audio) []Audio {
	encountered := map[Audio]bool{}
	result := []Audio{}

	for _, element := range elements {
		if !encountered[element] {
			encountered[element] = true
			result = append(result, element)
		}
	}

	return result
}
