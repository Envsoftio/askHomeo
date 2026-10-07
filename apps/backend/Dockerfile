FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY apps/backend/go.mod apps/backend/go.sum ./
RUN go mod download
COPY apps/backend/ ./
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ghostscript tesseract-ocr tesseract-ocr-eng && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /server /app/server
COPY apps/backend/migrations /app/apps/backend/migrations
ENV PROJECT_ROOT=/app
CMD ["/app/server"]
