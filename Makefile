
build:
	docker build -t torrent-indexer .

lint:
	$(shell go env GOPATH)/bin/golangci-lint run -v --timeout 5m

run:
	go run main.go

test:
	go test -v ./...