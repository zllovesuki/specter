# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:22.14.0-alpine AS ui-builder
RUN apk --no-cache add make
WORKDIR /app
COPY . .

RUN make ui

FROM --platform=$BUILDPLATFORM golang:1.24.3-alpine AS app-builder
RUN apk --no-cache add ca-certificates git
WORKDIR /app
COPY . .
COPY --from=ui-builder /app/tun/client/ui/build /app/tun/client/ui/build

RUN --mount=type=cache,target=/root/go/pkg/mod \
    go mod download

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOAMD64=$([[ $TARGETARCH == "amd64" ]] && echo ${TARGETVARIANT:-v1} || echo "v1") GOARM=7 \
    go build -tags 'osusergo netgo urfave_cli_no_docs no_mocks' \
    -ldflags "-s -w -extldflags -static -X go.miragespace.co/specter/spec.BuildVersion=`git rev-parse --short HEAD` -X go.miragespace.co/specter/spec.BuildTime=`date +%s`" \
    -o bin/specter .

FROM scratch
WORKDIR /app
COPY --from=app-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=app-builder /app/bin/specter /app/specter

ENTRYPOINT ["/app/specter"]