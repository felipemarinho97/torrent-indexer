package utils

import "regexp"

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

func IsValidHTML(input string) bool {
	// Check for <!DOCTYPE> declaration (case-insensitive)
	doctypeRegex := regexp.MustCompile(`(?i)<!DOCTYPE\s+html>`)
	if !doctypeRegex.MatchString(input) {
		return false
	}

	// Check for <html> and </html> tags (case-insensitive)
	htmlTagRegex := regexp.MustCompile(`(?i)<html[\s\S]*?>[\s\S]*?</html>`)
	if !htmlTagRegex.MatchString(input) {
		return false
	}

	// Check for <body> and </body> tags (case-insensitive)
	bodyTagRegex := regexp.MustCompile(`(?i)<body[\s\S]*?>[\s\S]*?</body>`)
	if !bodyTagRegex.MatchString(input) {
		return false
	}

	return true
}
