# Build: a static binary; SQLite comes from modernc.org/sqlite, so no CGO is needed.
FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/spokebook/spokebook/internal/app/spokebook.Version=${VERSION}" -o /out/spokebook ./cmd/spokebook \
 && mkdir /out/data

# Run: distroless, no shell, runs as nonroot (65532) unless the platform overrides the user.
FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="Spokebook" \
      org.opencontainers.image.source="https://github.com/spokebook/spokebook" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=build /out/spokebook /spokebook
COPY --from=build --chown=65532:65532 /out/data /data
ENV APP_ADDR=:8080 \
    APP_DB_PATH=/data/spokebook.sqlite
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/spokebook"]
