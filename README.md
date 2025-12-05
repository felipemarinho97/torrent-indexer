# torrent-indexer

[![](https://dcbadge.limes.pink/api/server/7wqNywmpQW)](https://discord.gg/7wqNywmpQW)
[![Sponsor](https://img.shields.io/badge/Sponsor-❤-ff69b4?style=for-the-badge&logo=github)](https://github.com/sponsors/felipemarinho97)

This is a simple torrent indexer that can be used to index torrents from HTML pages. It is written in Golang and uses Redis as a cache.

## Test it

Visit [https://torrent-indexer.darklyn.org/](https://torrent-indexer.darklyn.org/) to test it.

## Supported sites

- [comando-torrents](https://comando.la/)
- [bludv](https://bludvfilmes.tv/)
- [torrent-dos-filmes](https://torrentdosfilmes.se/)
- [starck-filmes](https://www.starckfilmes.fans/)
- [rede-torrent](https://redetorrent.com/)
- [vaca-torrent](https://vacatorrentmov.com/)
- [filme-torrent](https://www.filmetorrent.org)

## Deploy

If you have Docker + docker-compose installed, you can deploy it using the following command:

```bash
curl -s https://raw.githubusercontent.com/felipemarinho97/torrent-indexer/main/docker-compose.yml > docker-compose.yml
docker-compose up -d
```

The server will be available at [http://localhost:8080/](http://localhost:8080/).

## Configuration

You can configure the server using the following environment variables:
  
- `PORT`: (optional) The port that the server will listen to. Default: `7006`
- `METRICS_PORT`: (optional) The port that the metrics server will listen to. Default: `8081`
- `LOG_LEVEL`: (optional) The log level. Default: `1` (info). 
  - Possible values: `0` (debug), `1` (info), `2` (warn), `3` (error)
- `LOG_FORMAT`: (optional) The log format. Can be "json" or default to console logger.
- `FLARESOLVERR_ADDRESS`: (optional) The address of the FlareSolverr instance. Default: `N/A`
- `MEILISEARCH_ADDRESS`: (optional) The address of the MeiliSearch instance. Default: `N/A`
- `MEILISEARCH_KEY`: (optional) The API key of the MeiliSearch instance. Default: `N/A`
- `REDIS_HOST`: (optional) The address of the Redis instance. Default: `localhost`
- `REDIS_PASSWORD`: (optional) The password of the Redis instance. Default: `N/A`
- `SHORT_LIVED_CACHE_EXPIRATION` (optional) The expiration time of the short-lived cache in duration format. Default: `30m`
    - This cache is used to cache homepage or search results.
    - Example: `30m`, `1h`, `1h30m`, `1h30m30s`
- `LONG_LIVED_CACHE_EXPIRATION` (optional) The expiration time of the long-lived cache in duration format. Default: `7d`
    - This cache is used to store the torrent webpages (posts). You can set it to a higher value because the torrent pages are not updated frequently.

### Extra Configuration
- `FALLBACK_TITLE_ENABLED`: (optional) Enable the fallback title post-processor that sets the title to "[UNSAFE] {original_title}" (Page title) if the title is empty. Default: `false`
    - This is useful for sites that do not have a title for some torrents, but can lead to misleading titles.
- `MAGNET_METADATA_API_ENABLED`: (optional) Enable the magnet metadata API. (deploy instrucitons [here](https://github.com/felipemarinho97/magnet-metadata-api)) Default: `false`
- `MAGNET_METADATA_API_ADDRESS`: (optional) The address of your magnet metadata API. Default: `N/A`
- `MAGNET_METADATA_API_TIMEOUT_SECONDS`: (optional) The timeout for the magnet metadata API requests in seconds. Default: `10`
- `INDEXER_<NAME>_URL`: (optional) Set a custom URL for the indexer. Where the "NAME" will be always uppercase indexer key with underscores. ex: `INDEXER_DODO_FILMES_URL=https://my-proxied-dodo-url.org`
## Integrating with Jackett

You can integrate this indexer with Jackett by adding a new Torznab custom indexer. Here is an example of how to do it for the `bludv` indexer:

```yaml
---
id: bludv_indexer
name: BluDV Indexer
description: "BluDV - Custom indexer on from torrent-indexer"
language: pt-BR
type: public
encoding: UTF-8
links:
  - http://localhost:8080/

caps:
  categorymappings:
    - { id: Movie, cat: Movies, desc: "Movies" }
    - { id: TV, cat: TV, desc: "TV" }

  modes:
    search: [q]
    tv-search: [q, season, ep]
    movie-search: [q]
  allowrawsearch: true

settings: []

search:
  paths:
    - path: "indexers/bludv?filter_results=true&q={{ .Keywords }}"
      response:
        type: json

  keywordsfilters:
    - name: tolower

  rows:
    selector: $.results
    count:
      selector: $.count

  fields:
    _id:
      selector: title
    download:
      selector: magnet_link
    title:
      selector: title
    description:
      selector: original_title
    details:
      selector: details
    infohash:
      selector: info_hash
    date:
      selector: date
    size:
      selector: size
    seeders:
      selector: seed_count
    leechers:
      selector: leech_count
    imdb:
      selector: imdb
    category_is_tv_show:
      selector: title
      filters:
        - name: regexp
          args: "\\b(S\\d+(?:E\\d+)?)\\b"
    category:
      text: "{{ if .Result.category_is_tv_show }}TV{{ else }}Movie{{ end }}"
# json engine n/a
```

## Integrating with Prowlarr

You can integrate this indexer with Prowlarr by adding a custom definition. See [Adding a custom YML definition](https://wiki.servarr.com/prowlarr/indexers#adding-a-custom-yml-definition).

```yaml
---
id: torrent-indexer
name: Torrent Indexer
description: "Indexing Brazilian Torrent websites into structured data. github.com/felipemarinho97/torrent-indexer"
language: pt-BR
type: public
encoding: UTF-8
links:
  - http://localhost:8080/

caps:
  categories:
    Movies: Movies
    TV: TV

  modes:
    search: [q]
    tv-search: [q, season]
    movie-search: [q]

settings:
  - name: indexer
    type: select
    label: Indexer
    default: bludv
    options:
      search: Torrent-Indexer Cache
      bludv: BLUDV
      comando_torrents: Comando Torrents
      torrent-dos-filmes: Torrent dos Filmes
      starck-filmes: Starck Filmes
      rede_torrent: Rede Torrent
      filme_torrent: Filme Torrent
      vaca_torrent: Vaca Torrent

search:
  paths:
    - path: "{{ if eq .Config.indexer \"search\" }}/search{{ else }}/indexers/{{ .Config.indexer }}{{ end }}"
      response:
        type: json
  inputs:
    filter_results: "true"
    q: "{{ .Keywords }}"
  keywordsfilters:
    - name: tolower
    - name: re_replace
      args: ["(?i)(S0)(\\d{1,2})$", "temporada $2"]
    - name: re_replace
      args: ["(?i)(S)(\\d{1,3})$", "temporada $2"]

  rows:
    selector: $.results
    count:
      selector: $.count

  fields:
    download:
      selector: magnet_link
    title:
      selector: title
    description:
      selector: original_title
    details:
      selector: details
    infohash:
      selector: info_hash
    date:
      selector: date
    size:
      selector: size
    seeders:
      selector: seed_count
    leechers:
      selector: leech_count
    imdb:
      selector: imdb
    category_is_tv_show:
      selector: title
      filters:
        - name: regexp
          args: "\\b(S\\d+(?:E\\d+)?)\\b"
    category:
      text: "{{ if .Result.category_is_tv_show }}TV{{ else }}Movies{{ end }}"
```


## Mirrors

- N/A

### Community Mirrors

If you're running a public instance of torrent-indexer and would like to share it with the community, please open a **Merge Request** adding your instance to this list!

**Requirements for community mirrors:**
- Must be publicly accessible
- Should have reasonable uptime
- Include geographic location information
- Provide contact information for maintenance issues

# Warning

The instance running at [https://torrent-indexer.darklyn.org/](https://torrent-indexer.darklyn.org/) is my personal instance and it is not guaranteed to be up all the time. Also, for better availability, I recommend deploying your own instance because the Cloudflare protection may block requests from indexed sites if too many requests are made in a short period of time from the same IP.

If I notice that the instance is being used a lot, I may block requests from Jackett/Prowlarr to avoid overloading the server. I can also redirect your requests to a fake instance without prior notice. Consider yourself warned.

## If you cannot self-host

**If you really need to use my [instance](https://torrent-indexer.darklyn.org) continuously without the risk of being IP blocked, please subscribe to my [GitHub Sponsors](https://github.com/sponsors/felipemarinho97) (any tier) and let me know about your support ❤️ via [Discord](https://discord.gg/7wqNywmpQW).**
