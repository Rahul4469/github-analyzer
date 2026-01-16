# Build Tailwind CSS ----------------------------------------------
FROM node:20-alpine AS css-builder

WORKDIR /app

COPY tailwind/package*.json ./tailwind/

WORKDIR /app/tailwind
RUN npm ci

COPY tailwind/tailwind.config.js ./
COPY tailwind/postcss.config.js ./
COPY static/css/input.css ../static/css/input.css

COPY templates/ ../templates/
RUN npm run build:css
RUN ls -la ../static/css/output.css


# Build Go Application --------------------------------

FROM golang:1.24-alpine AS go-builder

WORKDIR /app
RUN apk add --no-cache git gcc musl-dev
COPY go.mod go.sum ./
RUN GOTOOLCHAIN=auto go mod download# Copy entire application source
COPY . .
COPY --from=css-builder /app/static/css/output.css ./static/css/output.css
RUN CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto go build \
    -ldflags="-s -w" \
    -trimpath \
    -o github-analyzer \
    ./cmd/server
RUN ls -lh github-analyzer


# Final Runtime Image ---------------------------------------

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata wget
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app
COPY --from=go-builder /app/github-analyzer .
COPY --from=go-builder /app/static ./static
COPY --from=go-builder /app/templates ./templates
RUN chown -R appuser:appuser /app
USER appuser
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1

CMD ["./github-analyzer"]
