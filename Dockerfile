FROM golang:1.23-alpine AS build

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/feedler ./cmd/feedler

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=build /out/feedler /usr/local/bin/feedler
COPY web ./web
COPY Feeds.opml ./Feeds.opml

ENV ADDR=:8080 \
    DATA_DIR=/data \
    WEB_DIR=/app/web \
    OPML_PATH=/app/Feeds.opml \
    TZ=Europe/Berlin

VOLUME ["/data"]
EXPOSE 8080

CMD ["feedler"]
