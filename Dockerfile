FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-build
ARG TARGETOS TARGETARCH
ARG GOPROXY=https://proxy.golang.org|direct
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY api ./api
COPY cmd ./cmd
COPY config ./config
COPY db ./db
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/ ./cmd/...

FROM alpine:3.22 AS backend
WORKDIR /app
COPY --from=go-build /out/ /usr/local/bin/
USER 65532:65532
CMD ["game"]

FROM backend AS northflank
ENV SCORES_LISTEN=127.0.0.1:8081
EXPOSE 8080
CMD ["orbital-stack"]
