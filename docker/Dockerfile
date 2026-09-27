# Çok aşamalı, tek Dockerfile: BINARY argümanı ile ilgili cmd/ ikilisini derler.
# Kullanım: docker build --build-arg BINARY=orchestrator -t a2a-research-crew/orchestrator -f docker/Dockerfile .

FROM golang:1.26 AS build

ARG BINARY
ENV CGO_ENABLED=0 GOOS=linux

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN test -n "$BINARY" || (echo "BINARY build arg is required" && exit 1)
RUN go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/${BINARY}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]
