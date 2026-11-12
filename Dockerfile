FROM node:22-alpine AS frontend
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web ./web
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-build
ARG TARGETOS TARGETARCH
ARG GOPROXY
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/ ./cmd/...

FROM alpine:3.22 AS backend
WORKDIR /app
COPY --from=go-build /out/ /usr/local/bin/
USER 65532:65532
CMD ["game"]

FROM backend AS local
COPY --from=frontend /src/dist ./dist
ENV WEB_DIR=/app/dist
