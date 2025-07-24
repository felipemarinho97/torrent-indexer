package utils

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"
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

// ParallelMap applies a function to each item in the iterable concurrently
// and returns a slice of results. It can handle errors by passing an error handler function.
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

	for range iterable {
		select {
		case items := <-itChan:
			mappedItems = append(mappedItems, items...)
		case err := <-errChan:
			for _, handler := range errHandler {
				handler(err)
			}
			if len(errHandler) == 0 {
				fmt.Println(err)
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

func IsValidHTML(input string) bool {
	r := strings.NewReader(input)
	_, err := html.Parse(r)
	return err == nil
}
