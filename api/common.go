package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/felipemarinho97/torrent-indexer/logging"
	"github.com/felipemarinho97/torrent-indexer/schema"
)

// getDocument retrieves a document from the cache or makes a request to get it.
// It first checks the Redis cache for the document body.
func getDocument(ctx context.Context, i *Indexer, link, referer string) (*goquery.Document, error) {
	// try to get from redis first
	docCache, err := i.redis.Get(ctx, link)
	if err == nil {
		i.metrics.CacheHits.WithLabelValues("document_body").Inc()
		logging.Debug().Str("url", link).Msg("Returning document from long-lived cache")
		return goquery.NewDocumentFromReader(io.NopCloser(bytes.NewReader(docCache)))
	}
	defer i.metrics.CacheMisses.WithLabelValues("document_body").Inc()

	resp, err := i.requester.GetDocument(ctx, link, referer)
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
		logging.Error().Err(err).Str("url", link).Msg("Failed to set document body in redis cache")
	}

	doc, err := goquery.NewDocumentFromReader(io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return nil, err
	}

	return doc, nil
}

func getPublishedDateFromMeta(document *goquery.Document) time.Time {
	var date time.Time
	//<meta property="article:published_time" content="2019-08-23T13:20:57+00:00">
	datePublished := strings.TrimSpace(document.Find("meta[property=\"article:published_time\"]").AttrOr("content", ""))
	if datePublished == "" {
		// <meta property="og:updated_time" content="2025-09-30T13:08:58-03:00">
		datePublished = strings.TrimSpace(document.Find("meta[property=\"og:updated_time\"]").AttrOr("content", ""))
	}

	if datePublished == "" {
		// type="application/ld+json" with "datePublished" attribute on json
		scriptTags := document.Find("script[type=\"application/ld+json\"]")
		scriptTags.EachWithBreak(func(i int, s *goquery.Selection) bool {
			scriptContent := s.Text()
			mapData := make(map[string]interface{})
			err := json.Unmarshal([]byte(scriptContent), &mapData)
			if err != nil {
				return true // continue
			}
			if dateVal, ok := mapData["datePublished"].(string); ok {
				datePublished = dateVal
				return false // break
			}
			return true // continue
		})
	}

	if datePublished != "" {
		date, _ = time.Parse(time.RFC3339, datePublished)
	}

	return date
}

type datePattern struct {
	regex  *regexp.Regexp
	layout string
}

var datePatterns = []datePattern{
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}`), "2006-01-02"},
	{regexp.MustCompile(`\d{2}-\d{2}-\d{4}`), "02-01-2006"},
	{regexp.MustCompile(`\d{2}/\d{2}/\d{4}`), "02/01/2006"},
	// Release Date: 4, October
	{regexp.MustCompile(`\d{1,2},? [A-Za-z]+`), "2, January"},
	// Release Date: October 4, 2020
	{regexp.MustCompile(`[A-Za-z]+ \d{1,2},? \d{4}`), "January 2, 2006"},
}

// getPublishedDateFromRawString extracts the date from a raw string using predefined patterns.
func getPublishedDateFromRawString(dateStr string) time.Time {
	for _, p := range datePatterns {
		match := p.regex.FindString(dateStr)

		if match != "" {
			date, err := time.Parse(p.layout, match)
			if err == nil {
				return date.UTC()
			}
		}
	}

	return time.Time{}
}

// getSeparator returns the separator used in the string.
// It checks for common separators like "|", ",", "/", and " e "
func getSeparator(s string) string {
	if strings.Contains(s, "|") {
		return "|"
	} else if strings.Contains(s, ",") {
		return ","
	} else if strings.Contains(s, "/") {
		return "/"
	} else if strings.Contains(s, " e ") {
		return " e "
	}
	return " "
}

// findAudioFromText extracts audio languages from a given text.
// It looks for patterns like "Áudio: Português, Inglês" or "Idioma: Português, Inglês"
func findAudioFromText(text string) []schema.Audio {
	var audio []schema.Audio
	re := regexp.MustCompile(`(.udio|Idioma|Languages):.?(.*)`)
	audioMatch := re.FindStringSubmatch(text)
	if len(audioMatch) > 0 {
		sep := getSeparator(audioMatch[2])
		langs_raw := strings.Split(audioMatch[2], sep)
		for _, lang := range langs_raw {
			lang = strings.TrimSpace(lang)
			a := schema.GetAudioFromString(lang)
			if a != nil {
				audio = append(audio, *a)
			} else if strings.TrimSpace(lang) != "" {
				logging.Warn().
					Str("language", lang).
					Msg("Unknown language detected")
				logging.Debug().Str("text", text).Msg("Unknown language detected from this text")
			}
		}
	}
	return audio
}

// findYearFromText extracts the year from a given text.
// It looks for patterns like "Lançamento: 2001" in the title.
func findYearFromText(text string, title string) (year string) {
	re := regexp.MustCompile(`(?:Lançamento|Year): (.*)`)
	yearMatch := re.FindStringSubmatch(text)
	if len(yearMatch) > 0 {
		lancamentoText := strings.TrimSpace(yearMatch[1])
		// Extract 4-digit year from the lançamento field
		yearRe := regexp.MustCompile(`\b(\d{4})\b`)
		if yearDigits := yearRe.FindStringSubmatch(lancamentoText); len(yearDigits) > 0 {
			year = yearDigits[1]
		}
	}

	if year == "" {
		re = regexp.MustCompile(`\((\d{4})\)`)
		yearMatch := re.FindStringSubmatch(title)
		if len(yearMatch) > 0 {
			year = yearMatch[1]
		}
	}

	return year
}

// findSizesFromText extracts sizes from a given text.
// It looks for patterns like "Tamanho: 1.26 GB" or "Tamanho: 700 MB".
func findSizesFromText(text string) []string {
	var sizes []string
	// everything that ends with GB or MB, using ',' or '.' as decimal separator
	re := regexp.MustCompile(`(\d+[\.,]?\d+) ?(GB|MB)`)
	sizesMatch := re.FindAllStringSubmatch(text, -1)
	if len(sizesMatch) > 0 {
		for _, size := range sizesMatch {
			sizes = append(sizes, size[0])
		}
	}
	return sizes
}

var imdbLinkRE = regexp.MustCompile(`https://www.imdb.com(/[a-z]{2})?/title/(tt\d+)/?`)
var subtitlesHintIMDBLinkRE = regexp.MustCompile(`imdbid-((tt)?\d+)`)

// getIMDBLink extracts the IMDB link from a given link.
// It looks for patterns like "https://www.imdb.com/title/tt1234567/".
// Returns an error if no valid IMDB link is found.
func getIMDBLink(link string) (string, error) {
	var imdbLink string

	matches := imdbLinkRE.FindStringSubmatch(link)
	if len(matches) > 0 {
		imdbLink = matches[0]
	} else if matches := subtitlesHintIMDBLinkRE.FindStringSubmatch(link); len(matches) > 0 {
		id := strings.TrimPrefix(matches[1], "tt")
		imdbLink = fmt.Sprintf("https://www.imdb.com/title/tt%s/", id)
	} else {
		return "", fmt.Errorf("no imdb link found")
	}
	return imdbLink, nil
}

// appendAudioISO639_2Code appends the audio languages to the title in ISO 639-2 code format.
// It formats the title to include the audio languages in parentheses.
// Example: "Movie Title (eng, por)"
func appendAudioISO639_2Code(title string, a []schema.Audio) string {
	if len(a) > 0 {
		audio := []string{}
		for _, lang := range a {
			audio = append(audio, lang.String())
		}
		audio = slices.Compact(audio)
		title = fmt.Sprintf("%s (%s)", title, strings.Join(audio, ", "))
	}
	return title
}

// getAudioFromTitle extracts audio languages from the release title.
// It checks for common patterns like "nacional", "dual", or "dublado"
func getAudioFromTitle(releaseTitle string, audioFromContent []schema.Audio) []schema.Audio {
	magnetAudio := []schema.Audio{}
	isNacional := strings.Contains(strings.ToLower(releaseTitle), "nacional")
	if isNacional {
		magnetAudio = append(magnetAudio, schema.AudioPortuguese)
	}

	if strings.Contains(strings.ToLower(releaseTitle), "dual") || strings.Contains(strings.ToLower(releaseTitle), "dublado") {
		magnetAudio = append(magnetAudio, audioFromContent...)
		// if Portuguese audio is not in the audio slice, append it
		if !slices.Contains(magnetAudio, schema.AudioPortuguese) {
			magnetAudio = append(magnetAudio, schema.AudioPortuguese)
		}
	} else if len(audioFromContent) > 1 {
		// remove portuguese audio, and append to magnetAudio
		for _, a := range audioFromContent {
			if a != schema.AudioPortuguese {
				magnetAudio = append(magnetAudio, a)
			}
		}
	} else {
		magnetAudio = append(magnetAudio, audioFromContent...)
	}

	// order and uniq the audio slice
	slices.SortFunc(magnetAudio, func(a, b schema.Audio) int {
		return strings.Compare(a.String(), b.String())
	})
	magnetAudio = slices.CompactFunc(magnetAudio, func(a, b schema.Audio) bool {
		return a.String() == b.String()
	})

	return magnetAudio
}
