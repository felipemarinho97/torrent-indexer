package utils

import (
	"encoding/base64"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func DecodeAdLink(encodedStr string) (string, error) {
	if encodedStr == "" {
		return "", fmt.Errorf("empty string")
	}
	reversed := reverseString(encodedStr)

	decodedBytes, err := base64.StdEncoding.DecodeString(reversed)
	if err != nil {
		return "", err
	}

	htmlUnescaped := html.UnescapeString(string(decodedBytes))

	return htmlUnescaped, nil
}

func Base64Decode(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty string")
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decodedBytes), nil
}

// Helper function to reverse a string
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// unshuffleStringByStep reconstructs an original string from a shuffled version.
// The input was created by picking characters using a stepping cursor (advancing
// by step positions through the original, skipping already-used positions).
// This function inverts the process: it walks the same cursor sequence and places
// each shuffled[i] back into original[index].
func unshuffleStringByStep(shuffled string, step int) (string, error) {
	runes := []rune(shuffled)
	length := len(runes)
	if length == 0 {
		return "", fmt.Errorf("empty string")
	}
	if step <= 0 {
		return "", fmt.Errorf("step must be greater than 0")
	}

	original := make([]rune, length)
	used := make([]bool, length)
	index := 0

	for i := 0; i < length; i++ {
		for used[index] {
			index = (index + 1) % length
		}
		used[index] = true
		original[i] = runes[index]
		index = (index + step) % length
	}

	return string(original), nil
}

// DecodeStarckDataU decodes the obfuscated magnet link from the data-u attribute. This is indexer-specific.
func DecodeStarckDataU(dataU string) (string, error) {
	if dataU == "" {
		return "", fmt.Errorf("empty data-u value")
	}

	unshuffled, err := unshuffleStringByStep(dataU, 3)
	if err != nil {
		return "", fmt.Errorf("unshuffle failed: %w", err)
	}

	if !IsMagnetLink(unshuffled) {
		return "", fmt.Errorf("decoded string is not a valid magnet link: %s", unshuffled)
	}

	return unshuffled, nil
}

func IsMagnetLink(link string) bool {
	return len(link) > 8 && link[:8] == "magnet:?"
}

// Parse1337xDate parses the 1337x date format "May. 11th '20" and returns
// an RFC3339 string that extractDateField can parse.
func Parse1337xDate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	// Strip ordinal suffixes: 1st 2nd 3rd 4th → 1 2 3 4
	ordinal := regexp.MustCompile(`(\d+)(?:st|nd|rd|th)`)
	raw = ordinal.ReplaceAllString(raw, "$1")

	// Remove dots from month abbreviations: "May." → "May"
	raw = strings.ReplaceAll(raw, ".", "")

	// Expand two-digit years: "'20" → "2020", "'95" → "1995"
	twoDigitYear := regexp.MustCompile(`'(\d{2})`)
	raw = twoDigitYear.ReplaceAllStringFunc(raw, func(s string) string {
		yr, _ := strconv.Atoi(strings.TrimPrefix(s, "'"))
		if yr <= 30 {
			return fmt.Sprintf("20%02d", yr)
		}
		return fmt.Sprintf("19%02d", yr)
	})

	// Parse: "Jan 2 2006"
	t, err := time.Parse("Jan 2 2006", strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("1337x: cannot parse date %q: %w", raw, err)
	}
	return t.Format("2006-01-02"), nil
}

// DecodeBludvAdwareLink helper function to be exported to yaml engine
func DecodeBludvAdwareLink(raw string) (string, error) {
	parsedURL, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	id := parsedURL.Query().Get("id")
	decoded, err := DecodeAdLink(id)
	if err != nil {
		return "", err
	}
	if !IsMagnetLink(decoded) {
		return "", fmt.Errorf("not a magnet link: %s", decoded)
	}
	return decoded, nil
}
