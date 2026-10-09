FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download
COPY apps/backend/ ./
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ghostscript tesseract-ocr tesseract-ocr-eng && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /server /app/server
COPY apps/backend/migrations /app/apps/backend/migrations
ENV PROJECT_ROOT=/app
CMD ["/app/server"]

FROM runtime AS production
RUN apt-get update && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 app \
    && useradd --uid 10001 --gid app --no-create-home app \
    && mkdir -p /app/data/runtime/assets \
    && chown app:app /app/data/runtime/assets
USER 10001:10001

# Keep the existing development image as the default build target.
FROM runtime AS development
