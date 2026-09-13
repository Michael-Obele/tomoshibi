# Build Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/tomoshibi-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/tomoshibi-worker ./cmd/worker

# Final Stage
FROM alpine:3.20

# Install Chromium, fonts, runtime dependencies, and tini for zombie reaping.
# apk is retried: the alpine CDN (dl-cdn.alpinelinux.org) intermittently
# SERVFAILs under load, and a single failed fetch would otherwise fail the
# whole image build. Each attempt that succeeds exits 0; only after all
# attempts fail does the build fail.
RUN apk update && \
    for i in 1 2 3 4 5; do \
        apk add --no-cache \
            chromium \
            nss \
            freetype \
            harfbuzz \
            ca-certificates \
            tzdata \
            ttf-freefont \
            font-noto-emoji \
            wqy-zenhei \
            tini && exit 0; \
        echo "apk add failed (attempt $i); retrying in 5s..."; \
        sleep 5; \
    done; \
    exit 1

# Set env for Chromedp to find chromium
ENV CHROME_BIN=/usr/bin/chromium-browser
ENV CHROME_PATH=/usr/lib/chromium/

WORKDIR /app
COPY --from=builder /app/tomoshibi-api .
COPY --from=builder /app/tomoshibi-worker .
COPY --from=builder /app/internal/api/docs ./internal/api/docs

# Create a symlink for chromedp compatibility
RUN ln -sf /usr/bin/chromium-browser /usr/bin/google-chrome-stable

EXPOSE 7431

# tini reaps zombie processes that Chrome spawns
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["./tomoshibi-api"]
