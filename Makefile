
build:
	docker build -t torrent-indexer .

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run -v --timeout 5m

run:
	go run main.go
