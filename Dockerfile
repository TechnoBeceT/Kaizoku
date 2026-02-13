# Stage 1: Build Go binary
FROM golang:1.25-bookworm AS builder

WORKDIR /build
COPY KaizokuBackend/go.mod KaizokuBackend/go.sum ./KaizokuBackend/
RUN cd KaizokuBackend && go mod download

COPY KaizokuBackend/ ./KaizokuBackend/
RUN cd KaizokuBackend && CGO_ENABLED=0 go build -ldflags="-s -w" -o /kaizoku ./cmd/kaizoku

# Stage 2: Build frontend (Nuxt 4 + Bun)
FROM oven/bun:latest AS frontend

WORKDIR /build
COPY KaizokuFrontend/package.json KaizokuFrontend/bun.lock ./
RUN bun install --frozen-lockfile

COPY KaizokuFrontend/ ./
RUN bun run generate

# Stage 3: Runtime with Java 21 for Suwayomi
FROM eclipse-temurin:21-jre-noble

RUN apt-get update && \
    apt-get install -y --no-install-recommends tini && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy Go binary
COPY --from=builder /kaizoku /app/kaizoku

# Copy frontend static build (Nuxt generates to .output/public/)
COPY --from=frontend /build/.output/public/ /app/frontend/

ENV KAIZOKU_DOCKER=true
ENV KAIZOKU_STORAGE_DIR=/series

EXPOSE 9833
EXPOSE 4567

ENTRYPOINT ["tini", "--"]
CMD ["/app/kaizoku"]
