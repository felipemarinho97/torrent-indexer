package goscrape

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
)

func getPeersFromCache(ctx context.Context, r *cache.Redis, infoHash string) (int, int, error) {
	// get peers and seeds from redis first
	peersCache, err := r.Get(ctx, infoHash)
	if err == nil {
		var peers map[string]int
		err = json.Unmarshal(peersCache, &peers)
		if err != nil {
			return 0, 0, err
		}
		return peers["leech"], peers["seed"], nil
	}
	return 0, 0, err
}

func setPeersToCache(ctx context.Context, r *cache.Redis, infoHash string, peer, seed int) error {
	peers := map[string]int{
		"leech": peer,
		"seed":  seed,
	}
	peersJSON, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	err = r.SetWithExpiration(ctx, infoHash, peersJSON, 24*time.Hour)
	if err != nil {
		return err
	}
	return nil
}

func GetLeechsAndSeeds(ctx context.Context, r *cache.Redis, infoHash string, trackers []string) (int, int, error) {
	var leech, seed int
	leech, seed, err := getPeersFromCache(ctx, r, infoHash)
	if err != nil {
		fmt.Println("unable to get peers from cache for infohash:", infoHash)
	} else {
		fmt.Println("get from cache> leech:", leech, "seed:", seed)
		return leech, seed, nil
	}

	for _, tracker := range trackers {
		// get peers and seeds from redis first
		scraper, err := New(tracker)
		if err != nil {
			fmt.Println(err)
			continue
		}

		scraper.SetTimeout(500 * time.Millisecond)

		// get peers and seeds from redis first
		res, err := scraper.Scrape([]byte(infoHash))
		if err != nil {
			fmt.Println(err)
			continue
		}

		leech += int(res[0].Leechers)
		seed += int(res[0].Seeders)
		setPeersToCache(ctx, r, infoHash, leech, seed)
		return leech, seed, nil
	}
	return leech, seed, nil
}
