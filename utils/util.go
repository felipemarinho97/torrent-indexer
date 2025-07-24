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

func ParallelMap[T any, R any](iterable []T, mapper func(item T) ([]R, error), errHandler ...func(error)) []R {
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

	for i := 0; i < len(iterable); i++ {
		select {
		case items := <-itChan:
			mappedItems = append(mappedItems, items...)
		case err := <-errChan:
			for _, handler := range errHandler {
				handler(err)
			}
		}
	}
	return mappedItems
}

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

func IsValidHTML(input string) bool {
	r := strings.NewReader(input)
	_, err := html.Parse(r)
	return err == nil
}
