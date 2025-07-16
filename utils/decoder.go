package utils

import (
	"encoding/base64"
	"html"
)

func DecodeAdLink(encodedStr string) (string, error) {
	reversed := reverseString(encodedStr)

	decodedBytes, err := base64.StdEncoding.DecodeString(reversed)
	if err != nil {
		return "", err
	}

	htmlUnescaped := html.UnescapeString(string(decodedBytes))

	return htmlUnescaped, nil
}

// Helper function to reverse a string
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
