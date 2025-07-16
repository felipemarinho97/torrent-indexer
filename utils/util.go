package utils

import (
	"strings"
	"golang.org/x/net/html"
)

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
	r := strings.NewReader(input)
	_, err := html.Parse(r)
	return err == nil
}
