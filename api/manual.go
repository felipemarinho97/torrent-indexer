package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/felipemarinho97/torrent-indexer/magnet"
	"github.com/felipemarinho97/torrent-indexer/schema"
	goscrape "github.com/felipemarinho97/torrent-indexer/scrape"
	"github.com/redis/go-redis/v9"
)

const manualTorrentsRedisKey = "manual:torrents"

var manualTorrentExpiration = 8 * time.Hour

type ManualIndexerRequest struct {
	MagnetLink string `json:"magnetLink"`
}

func (i *Indexer) HandlerManualIndexer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req ManualIndexerRequest
	indexedTorrents := []IndexedTorrent{}

	// fetch from redis
	out, err := i.redis.Get(ctx, manualTorrentsRedisKey)
	if err != nil && !errors.Is(err, redis.Nil) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Println(err)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			fmt.Println(err)
		}
		i.metrics.IndexerErrors.WithLabelValues("manual").Inc()
		return
	} else if errors.Is(err, redis.Nil) {
		out = bytes.NewBufferString("[]").Bytes()
	}

	err = json.Unmarshal([]byte(out), &indexedTorrents)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		if err != nil {
			fmt.Println(err)
		}
		i.metrics.IndexerErrors.WithLabelValues("manual").Inc()
		return
	}

	// check if the request is a POST
	if r.Method == http.MethodPost {
		// decode the request body
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				fmt.Println(err)
			}
			i.metrics.IndexerErrors.WithLabelValues("manual").Inc()
			return
		}

		magnet, err := magnet.ParseMagnetUri(req.MagnetLink)
		if err != nil {
			fmt.Println(err)
		}
		var audio []schema.Audio
		releaseTitle := magnet.DisplayName
		infoHash := magnet.InfoHash.String()
		trackers := magnet.Trackers
		magnetAudio := []schema.Audio{}
		if strings.Contains(strings.ToLower(releaseTitle), "dual") || strings.Contains(strings.ToLower(releaseTitle), "dublado") {
			magnetAudio = append(magnetAudio, audio...)
		} else if len(audio) > 1 {
			// remove portuguese audio, and append to magnetAudio
			for _, a := range audio {
				if a != schema.AudioPortuguese {
					magnetAudio = append(magnetAudio, a)
				}
			}
		} else {
			magnetAudio = append(magnetAudio, audio...)
		}

		peer, seed, err := goscrape.GetLeechsAndSeeds(ctx, i.redis, i.metrics, infoHash, trackers)
		if err != nil {
			fmt.Println(err)
		}

		title := processTitle(releaseTitle, magnetAudio)

		ixt := IndexedTorrent{
			Title:         appendAudioISO639_2Code(releaseTitle, magnetAudio),
			OriginalTitle: title,
			Audio:         magnetAudio,
			MagnetLink:    req.MagnetLink,
			InfoHash:      infoHash,
			Trackers:      trackers,
			LeechCount:    peer,
			SeedCount:     seed,
		}

		// write to redis
		indexedTorrents = append(indexedTorrents, ixt)
		out, err := json.Marshal(indexedTorrents)
		if err != nil {
			fmt.Println(err)
		}

		err = i.redis.SetWithExpiration(ctx, manualTorrentsRedisKey, out, manualTorrentExpiration)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			if err != nil {
				fmt.Println(err)
			}
			i.metrics.IndexerErrors.WithLabelValues("manual").Inc()
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(Response{
		Results: indexedTorrents,
		Count:   len(indexedTorrents),
	})
	if err != nil {
		fmt.Println(err)
	}
}
