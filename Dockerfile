FROM golang:1.22-bookworm AS build

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/feedler .

FROM debian:bookworm-slim

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=build /out/feedler /app/feedler
COPY Feeds.opml /app/Feeds.opml

ENV ADDR=:8080
ENV DB_PATH=/app/data/feedler.db
ENV OPML_PATH=/app/Feeds.opml
ENV TZ=Europe/Berlin

VOLUME ["/app/data"]
EXPOSE 8080

ENTRYPOINT ["/app/feedler"]
