FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /shieldcaptcha ./cmd/server/

FROM node:20-alpine AS sdk-builder
WORKDIR /sdk
COPY sdk/package.json sdk/package-lock.json* ./
RUN npm ci --ignore-scripts 2>/dev/null || npm install
COPY sdk/ .
RUN npm run build

FROM alpine:3.20
RUN apk --no-cache add ca-certificates wget
WORKDIR /app
COPY --from=builder /shieldcaptcha .
COPY --from=sdk-builder /sdk/dist ./web/sdk/
COPY web/ ./web/
COPY internal/storage/migrations ./migrations/

EXPOSE 8080
ENV CAPTCHA_PORT=8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD wget --spider -q http://localhost:8080/health || exit 1
ENTRYPOINT ["./shieldcaptcha"]
