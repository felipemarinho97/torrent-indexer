package utils

import (
	"fmt"
	"regexp"
	"strings"

	"os"

	"github.com/felipemarinho97/torrent-indexer/logging"
)

// Filter filters a slice based on a predicate function.
func Filter[A any](arr []A, f func(A) bool) []A {
	var res []A
	res = make([]A, 0)
	for _, v := range arr {
		if f(v) {
			res = append(res, v)
		}
	}
	return res
}

// ParallelFlatMap applies a function to each item in the iterable concurrently
// and returns a slice of results. It can handle errors by passing an error handler function.
func ParallelFlatMap[T any, R any](iterable []T, mapper func(item T) ([]R, error), errHandler ...func(error)) []R {
	var itChan = make(chan []R)
	var errChan = make(chan error)
	mappedItems := []R{}
	for _, link := range iterable {
		go func(link T) {
			items, err := mapper(link)
			if err != nil {
				errChan <- err
			}
			itChan <- items
		}(link)
	}

	for range iterable {
		select {
		case items := <-itChan:
			mappedItems = append(mappedItems, items...)
		case err := <-errChan:
			for _, handler := range errHandler {
				handler(err)
			}
			if len(errHandler) == 0 {
				logging.Error().Err(err).Msg("Error in ParallelFlatMap")
			}
		}
	}
	return mappedItems
}

// StableUniq removes duplicates from a slice while maintaining the order of elements.
func StableUniq(s []string) []string {
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

var (
	doctypeRegex = regexp.MustCompile(`(?i)<!DOCTYPE\s+html>`)
	htmlTagRegex = regexp.MustCompile(`(?i)<html[\s\S]*?>[\s\S]*?</html>`)
	bodyTagRegex = regexp.MustCompile(`(?i)<body[\s\S]*?>[\s\S]*?</body>`)
)

func IsValidHTML(input string) bool {
	// Check for <!DOCTYPE>, <html>, or <body> tags
	if !doctypeRegex.MatchString(input) && !htmlTagRegex.MatchString(input) && !bodyTagRegex.MatchString(input) {
		return false
	}

	return true
}

// FormatBytes formats a byte size into a human-readable string.
// It converts bytes to KB, MB, or GB as appropriate.
func FormatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	} else if bytes < 1024*1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	} else {
		return fmt.Sprintf("%.2f TB", float64(bytes)/(1024*1024*1024*1024))
	}
}

var sizeRegex = regexp.MustCompile(`(?i)^(\d+(?:[.,]\d+)?)\s*(B|KB|MB|GB|TB)$`)

// ParseSize parses a human-readable size string (e.g., "1.5 GB", "500 MB") to bytes.
// Returns the size in bytes, or 0 if the string cannot be parsed.
func ParseSize(sizeStr string) int64 {
	matches := sizeRegex.FindStringSubmatch(sizeStr)
	if len(matches) != 3 {
		return 0
	}

	// Parse the numeric value, handling both comma and dot as decimal separator
	var value float64
	numStr := matches[1]
	numStr = regexp.MustCompile(`[,]`).ReplaceAllString(numStr, ".")
	_, err := fmt.Sscanf(numStr, "%f", &value)
	if err != nil {
		return 0
	}

	unit := matches[2]

	// Convert to bytes based on unit
	var multiplier int64
	switch unit {
	case "B":
		multiplier = 1
	case "KB", "Kb", "kb":
		multiplier = 1024
	case "MB", "Mb", "mb":
		multiplier = 1024 * 1024
	case "GB", "Gb", "gb":
		multiplier = 1024 * 1024 * 1024
	case "TB", "Tb", "tb":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0
	}

	return int64(value * float64(multiplier))
}

func IsVideoFile(filename string) bool {
	lowerFilename := strings.ToLower(filename)
	videoExtensions := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".mpeg", ".mpg", ".m4v", ".3gp", ".ts"}
	for _, ext := range videoExtensions {
		if strings.HasSuffix(lowerFilename, ext) {

			return true
		}
	}
	return false
}

// GetEnvOrDefault returns the value of the environment variable named by the key,
// or the default value if the environment variable is not set.
func GetEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
