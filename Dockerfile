FROM golang:1.22 as builder

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
LABEL maintainer="felipevm97@gmail.com"

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /go/src/app/app .

CMD ["/root/app"]
