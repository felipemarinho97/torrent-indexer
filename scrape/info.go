package goscrape

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/felipemarinho97/torrent-indexer/cache"
	"github.com/felipemarinho97/torrent-indexer/monitoring"
	"github.com/felipemarinho97/torrent-indexer/utils"
)

type peers struct {
	Seeders  int `json:"seed"`
	Leechers int `json:"leech"`
}

func getPeersFromCache(ctx context.Context, r *cache.Redis, infoHash string) (int, int, error) {
	// get peers and seeds from redis first
	peersCache, err := r.Get(ctx, infoHash)
	if err == nil {
		var peers peers
		err = json.Unmarshal(peersCache, &peers)
		if err != nil {
			return 0, 0, err
		}
		return peers.Leechers, peers.Seeders, nil
	}
	return 0, 0, err
}

func setPeersToCache(ctx context.Context, r *cache.Redis, infoHash string, peer, seed int) error {
	peers := peers{
		Seeders:  seed,
		Leechers: peer,
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

var additionalTrackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://p4p.arenabg.com:1337/announce",
	"udp://retracker.hotplug.ru:2710/announce",
	"http://tracker.bt4g.com:2095/announce",
	"http://bt.okmp3.ru:2710/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"http://tracker.mywaifu.best:6969/announce",
	"udp://ttk2.nbaonlineservice.com:6969/announce",
	"http://tracker.privateseedbox.xyz:2710/announce",
	"udp://evan.im:6969/announce",
	"https://tracker.yemekyedim.com:443/announce",
	"udp://retracker.lanta.me:2710/announce",
	"udp://martin-gebhardt.eu:25/announce",
	"http://tracker.beeimg.com:6969/announce",
	"udp://udp.tracker.projectk.org:23333/announce",
	"http://tracker.renfei.net:8080/announce",
	"https://tracker.expli.top:443/announce",
	"https://tr.nyacat.pw:443/announce",
	"udp://tracker.ducks.party:1984/announce",
	"udp://extracker.dahrkael.net:6969/announce",
	"http://ipv4.rer.lol:2710/announce",
	"udp://tracker.plx.im:6969/announce",
	"udp://tracker.tvunderground.org.ru:3218/announce",
	"http://tracker.tricitytorrents.com:2710/announce",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.dler.com:6969/announce",
	"https://tracker.moeblog.cn:443/announce",
	"udp://d40969.acod.regrucolo.ru:6969/announce",
	"https://tracker.jdx3.org:443/announce",
	"http://ipv6.rer.lol:6969/announce",
	"udp://bandito.byterunner.io:6969/announce",
	"udp://tracker.gigantino.net:6969/announce",
	"http://tracker.netmap.top:6969/announce",
	"udp://tracker.yume-hatsuyuki.moe:6969/announce",
	"https://tracker.aburaya.live:443/announce",
	"udp://tracker.srv00.com:6969/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://1c.premierzal.ru:6969/announce",
	"udp://tracker.fnix.net:6969/announce",
	"udp://tracker.kmzs123.cn:17272/announce",
	"https://tracker.home.kmzs123.cn:4443/announce",
	"udp://tracker-udp.gbitt.info:80/announce",
	"udp://tracker.torrust-demo.com:6969/announce",
	"udp://tracker.hifimarket.in:2710/announce",
	"udp://retracker01-msk-virt.corbina.net:80/announce",
	"https://tracker.ghostchu-services.top:443/announce",
	"udp://open.dstud.io:6969/announce",
	"udp://tracker.therarbg.to:6969/announce",
	"udp://tracker.bitcoinindia.space:6969/announce",
	"udp://www.torrent.eu.org:451/announce",
	"udp://tracker.hifitechindia.com:6969/announce",
	"udp://tracker.gmi.gd:6969/announce",
	"udp://tracker.skillindia.site:6969/announce",
	"http://tracker.ipv6tracker.ru:80/announce",
	"udp://tracker.tryhackx.org:6969/announce",
	"http://torrent.hificode.in:6969/announce",
	"http://open.trackerlist.xyz:80/announce",
	"http://taciturn-shadow.spb.ru:6969/announce",
	"http://0123456789nonexistent.com:80/announce",
	"http://shubt.net:2710/announce",
	"udp://tracker.valete.tf:9999/announce",
	"https://tracker.zhuqiy.top:443/announce",
	"https://tracker.leechshield.link:443/announce",
	"http://tracker.tritan.gg:8080/announce",
	"udp://t.overflow.biz:6969/announce",
	"udp://open.tracker.cl:1337/announce",
	"udp://explodie.org:6969/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://bt.ktrackers.com:6666/announce",
	"udp://wepzone.net:6969/announce",
	"udp://tracker2.dler.org:80/announce",
	"udp://tracker.theoks.net:6969/announce",
	"udp://tracker.ololosh.space:6969/announce",
	"udp://tracker.filemail.com:6969/announce",
	"udp://tracker.dump.cl:6969/announce",
	"udp://tracker.dler.org:6969/announce",
	"udp://tracker.bittor.pw:1337/announce",
}

func GetLeechsAndSeeds(ctx context.Context, r *cache.Redis, m *monitoring.Metrics, infoHash string, trackers []string) (int, int, error) {
	leech, seed, err := getPeersFromCache(ctx, r, infoHash)
	if err != nil {
		m.CacheHits.WithLabelValues("peers").Inc()
		fmt.Println("unable to get peers from cache for infohash:", infoHash)
	} else {
		m.CacheMisses.WithLabelValues("peers").Inc()
		fmt.Println("hash:", infoHash, "get from cache -> leech:", leech, "seed:", seed)
		return leech, seed, nil
	}

	var peerChan = make(chan peers)
	var errChan = make(chan error)

	allTrackers := make([]string, 0, len(trackers)+len(additionalTrackers))
	allTrackers = append(allTrackers, trackers...)
	allTrackers = append(allTrackers, additionalTrackers...)
	allTrackers = utils.StableUniq(allTrackers)

	for _, tracker := range allTrackers {
		go func(tracker string) {
			// get peers and seeds from redis first
			scraper, err := New(tracker)
			if err != nil {
				errChan <- err
				return
			}

			scraper.SetTimeout(500 * time.Millisecond)

			// get peers and seeds from redis first
			res, err := scraper.Scrape([]byte(infoHash))
			if err != nil {
				errChan <- err
				return
			}

			peerChan <- peers{
				Seeders:  int(res[0].Seeders),
				Leechers: int(res[0].Leechers),
			}
		}(tracker)
	}

	var peer peers
	for i := 0; i < len(allTrackers); i++ {
		select {
		case <-errChan:
			// discard error
		case peer = <-peerChan:
			err = setPeersToCache(ctx, r, infoHash, peer.Leechers, peer.Seeders)
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("hash:", infoHash, "get from tracker -> leech:", peer.Leechers, "seed:", peer.Seeders)
			}
			return peer.Leechers, peer.Seeders, nil
		}
	}

	return 0, 0, fmt.Errorf("unable to get peers from trackers for infohash: %s", infoHash)
}
