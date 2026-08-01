FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /rgx-server ./cmd/server

FROM alpine:3.20

# tzdata is required at runtime: internal/cron calls time.LoadLocation("Europe/Berlin"),
# which needs the IANA zoneinfo database — Alpine doesn't ship it by default.
RUN apk add --no-cache tzdata

WORKDIR /app
COPY --from=builder /rgx-server /app/rgx-server

ENV TZ=Europe/Berlin \
    PORT=8080 \
    DB_PATH=/app/data/players.db \
    IMAGE_CACHE_DIR=/app/cache/images

EXPOSE 8080

ENTRYPOINT ["/app/rgx-server"]
