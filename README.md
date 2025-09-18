# torrent-indexer

[![](https://dcbadge.limes.pink/api/server/7wqNywmpQW)](https://discord.gg/7wqNywmpQW)

This is a simple torrent indexer that can be used to index torrents from HTML pages. It is written in Golang and uses Redis as a cache.

## Test it

Visit [https://torrent-indexer.darklyn.org/](https://torrent-indexer.darklyn.org/) to test it.

## Supported sites

- [comando-torrents](https://comando.la/)
- [bludv](https://bludvfilmes.tv/)
- [torrent-dos-filmes](https://torrentdosfilmes.se/)
- [starck-filmes](https://www.starckfilmes.online/)
- [comandohds](https://comandohds.org/)
- [rede-torrent](https://redetorrent.com/)

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
- `FLARESOLVERR_ADDRESS`: (optional) The address of the FlareSolverr instance. Default: `N/A`
- `MEILISEARCH_ADDRESS`: (optional) The address of the MeiliSearch instance. Default: `N/A`
- `MEILISEARCH_KEY`: (optional) The API key of the MeiliSearch instance. Default: `N/A`
- `REDIS_HOST`: (optional) The address of the Redis instance. Default: `localhost`
- `SHORT_LIVED_CACHE_EXPIRATION` (optional) The expiration time of the short-lived cache in duration format. Default: `30m`
    - This cache is used to cache homepage or search results.
    - Example: `30m`, `1h`, `1h30m`, `1h30m30s`
- `LONG_LIVED_CACHE_EXPIRATION` (optional) The expiration time of the long-lived cache in duration format. Default: `7d`
    - This cache is used to store the torrent webpages (posts). You can set it to a higher value because the torrent pages are not updated frequently.

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
id: torrent-indexer-robust
name: Brazilian Torrent Indexer (Robust)
description: "Robust indexing of Brazilian Torrent sites via torrent-indexer API"
language: pt-BR
type: public
encoding: UTF-8
links:
  - http://localhost:8080/

caps:
  categorymappings:
    - {id: 2000, cat: Movies, desc: "Movies"}
    - {id: 5000, cat: TV, desc: "TV"}
  modes:
    search: [q]
    tv-search: [q, season, ep]
    movie-search: [q, year]

settings:
  - name: indexer
    type: select
    label: Indexer
    default: starck-filmes
    options:
      bludv: BLUDV
      comando_torrents: Comando Torrents
      torrent-dos-filmes: Torrent dos Filmes
      comandohds: Comando HDs
      starck-filmes: Starck Filmes
      rede_torrent: Rede Torrent
  - name: timeout
    type: text
    label: Timeout (seconds)
    default: "30"
  - name: filter_results
    type: checkbox
    label: Filter similar results
    default: true

search:
  paths:
    - path: "/indexers/{{ .Config.indexer }}"
      method: get
      response:
        type: json
      
  inputs:
    q: "{{ .Keywords }}"
    filter_results: "{{ if .Config.filter_results }}true{{ else }}false{{ end }}"
    timeout: "{{ .Config.timeout }}"

  keywordsfilters:
    - name: tolower
    - name: re_replace
      args: ["(?i)\\bS0?(\\d{1,2})E\\d{2}\\b", "temporada $1"]
    - name: re_replace
      args: ["(?i)\\bS0?(\\d{1,2})\\b$", "temporada $1"]
    - name: re_replace
      args: ["\\s+", " "]
    - name: trim

  rows:
    selector: "$.results"
    count:
      selector: "$.count"

  fields:
    title:
      selector: title
      filters:
        - name: re_replace
          args: ["[\\n\\r]+", " "]
        - name: re_replace
          args: ["\\s{2,}", " "]
        - name: trim

    original_title:
      selector: original_title
      optional: true

    description:
      text: "{{ if .Result.original_title }}{{ .Result.original_title }}{{ else }}{{ .Result.title }}{{ end }}"

    details:
      selector: details
      filters:
        - name: urldecode

    download:
      selector: magnet_link
      filters:
        - name: urldecode

    date:
      selector: date
      filters:
        # Corrige data inválida que vem da API
        - name: re_replace
          args: ["^(0001-01-01.*|null|)$", "now"]
        - name: dateparse
          args: "2006-01-02T15:04:05Z"

    # Tamanho - usa exatamente o que vem da API ou 1 GB se for vazio/0 B
    size:
      selector: size
      optional: true
      filters:
        - name: re_replace
          args: ["^(|0 B|0B)$", "1 GB"]

    seeders:
      selector: seed_count
      filters:
        # Garante valores válidos para seeders
        - name: re_replace
          args: ["^(|null|0)$", "1"]

    leechers:
      selector: leech_count
      filters:
        # Garante valores válidos para leechers - se for 0 vira 1
        - name: re_replace
          args: ["^(|null|0)$", "1"]

    infohash:
      selector: info_hash
      optional: true

    imdbid:
      selector: imdb
      optional: true
      filters:
        - name: re_replace
          args: ["^$", ""]

    year:
      selector: year
      optional: true

    # Detecção robusta de categoria
    category_is_tv_show:
      text: "{{ .Result.title }} {{ .Result.original_title }}"
      filters:
        - name: regexp
          args: "(?i)(temporada|season|S\\d{1,3}E\\d{1,3}|S\\d{1,3}$|\\d+x\\d+|série|series|episódio|episode|EP\\d+|1ª|2ª|3ª|4ª|5ª|6ª|7ª|8ª|9ª|10ª|completa)"

    category:
      text: "{{ if .Result.category_is_tv_show }}5000{{ else }}2000{{ end }}"

    # Detecção de qualidade
    quality:
      text: "{{ .Result.title }}"
      filters:
        - name: regexp
          args: "(?i)(2160p|4K|UHD|1080p|FHD|FullHD|720p|HD|480p|SD|BluRay|BRRip|WEB-?DL|WEBRip|HDTV|DVDRip)"
        - name: toupper
        - name: re_replace
          args: ["^$", "WEB-DL"]

    # Detecção de codec
    codec:
      text: "{{ .Result.title }}"
      optional: true
      filters:
        - name: regexp
          args: "(?i)(x265|HEVC|H\\.?265|x264|H\\.?264|AVC|XviD|DivX)"
        - name: toupper

    # Detecção de idioma/áudio
    language:
      text: "{{ .Result.title }} {{ .Result.original_title }}"
      filters:
        - name: regexp
          args: "(?i)(DUAL|Dublado|Legendado|Nacional|PT-?BR|Português|Inglês)"
        - name: re_replace
          args: ["(?i)dual", "Dual Áudio"]
        - name: re_replace
          args: ["(?i)(dublado|nacional)", "Dublado"]
        - name: re_replace
          args: ["(?i)legendado", "Legendado"]
        - name: re_replace
          args: ["^$", "PT-BR"]

    # Similaridade da API para debugging
    similarity:
      selector: similarity
      optional: true

retry:
  count: 3
  delay: 2

error:
  - selector: "$.error"
    message:
      selector: "$.message"
      
  - selector: "$.results"
    filter:
      selector: "."
      filters:
        - name: regexp
          args: "^\\[\\]$"
    message:
      text: "Nenhum resultado encontrado para este indexer. Tente outro ou ajuste a busca."

  - selector: ":contains(\"timeout\")"
    message:
      text: "Timeout na requisição. Aumente o timeout nas configurações ou tente novamente."
```

# Warning

The instance running at [https://torrent-indexer.darklyn.org/](https://torrent-indexer.darklyn.org/) is my personal instance and it is not guaranteed to be up all the time. Also, for better availability, I recommend deploying your own instance because the Cloudflare protection may block requests from indexed sites if too many requests are made in a short period of time from the same IP.

If I notice that the instance is being used a lot, I may block requests from Jackett to avoid overloading the server without prior notice.
