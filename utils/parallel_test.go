package utils_test

import (
	"errors"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/felipemarinho97/torrent-indexer/utils"
)

// waitForGoroutines waits until the number of goroutines drops to at most
// want, returning the last observed count.
func waitForGoroutines(want int) int {
	deadline := time.Now().Add(2 * time.Second)
	n := runtime.NumGoroutine()
	for n > want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestParallelFlatMapDoesNotLeakGoroutinesOnError(t *testing.T) {
	before := runtime.NumGoroutine()

	input := []int{1, 2, 3, 4, 5, 6}
	var handled int
	got := utils.ParallelFlatMap(input, func(i int) ([]int, error) {
		if i%2 == 0 {
			return nil, errors.New("even")
		}
		return []int{i * 10}, nil
	}, func(error) { handled++ })

	sort.Ints(got)
	want := []int{10, 30, 50}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if handled != 3 {
		t.Errorf("error handler called %d times, want 3", handled)
	}

	if after := waitForGoroutines(before); after > before {
		t.Errorf("goroutines leaked: %d before, %d after", before, after)
	}
}
