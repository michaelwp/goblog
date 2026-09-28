# 1. Build the React bundles (server + client).
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# 2. Compile the Go server with the bundles embedded.
FROM golang:1.26-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend /src/backend/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/blog ./cmd/server

# 3. Ship just the binary.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/blog /blog
EXPOSE 8080
ENTRYPOINT ["/blog"]
