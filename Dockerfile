# Build Tailwind CSS ----------------------------------------------
FROM node:20-alpine AS css-builder

WORKDIR /app

# Copy package files for dependency installation
COPY tailwind/package*.json ./tailwind/

# Install Tailwind dependencies
WORKDIR /app/tailwind
# Install exact dependencies (include dev deps if tailwind is listed there)
RUN npm ci

# Copy Tailwind configuration and source files
COPY tailwind/tailwind.config.js ./
COPY tailwind/postcss.config.js ./
COPY static/css/input.css ../static/css/input.css

# Copy templates for content scanning
COPY templates/ ../templates/
RUN npm run build:css
RUN ls -la ../static/css/output.css


# Stage 2: Build Go Application --------------------------------

FROM golang:1.24-alpine AS go-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy Go modules files
COPY go.mod go.sum ./

# Download Go dependencies
# Use GOTOOLCHAIN=auto to automatically download Go 1.25 if needed
RUN GOTOOLCHAIN=auto go mod download

# Copy entire application source
COPY . .

# Copy compiled CSS from css-builder stage
COPY --from=css-builder /app/static/css/output.css ./static/css/output.css

# Build the Go application
# -ldflags="-s -w" strips debug info for smaller binary
# -trimpath removes file system paths for security
RUN CGO_ENABLED=0 GOOS=linux GOTOOLCHAIN=auto go build \
    -ldflags="-s -w" \
    -trimpath \
    -o github-analyzer \
    ./cmd/server

# Verify binary was created
RUN ls -lh github-analyzer


# Stage 3: Final Runtime Image ---------------------------------------

FROM alpine:latest

# Add ca-certificates for HTTPS requests and wget for healthcheck
RUN apk --no-cache add ca-certificates tzdata wget

# Create non-root user for security
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

# Copy binary from go-builder
COPY --from=go-builder /app/github-analyzer .

# Copy static assets (including compiled CSS)
COPY --from=go-builder /app/static ./static

# Copy templates
COPY --from=go-builder /app/templates ./templates

# Change ownership to non-root user
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose application port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:3000/health || exit 1

# Run the application
CMD ["./github-analyzer"]
