FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY scraper.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o scraper scraper.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata chromium chromium-chromedriver
WORKDIR /root/
COPY --from=builder /app/scraper .
COPY config.json .
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/
CMD ["./scraper"]